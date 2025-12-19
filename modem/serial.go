package modem

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tarm/serial"
	"github.com/xlab/at/pdu"
	"github.com/xlab/at/sms"
)

const (
	bufferSize  = 256
	errorSleep  = 100 * time.Millisecond
	readTimeout = 100 * time.Millisecond
	atTimeout   = 1 * time.Second
)

// SerialService 封装了单个串口的读取、写入和监控。
type SerialService struct {
	name      string
	port      *serial.Port
	broadcast func(string)
	sync.Mutex
}

// NewSerialService 尝试连接并初始化串口服务。
func NewSerialService(name string, baudRate int, broadcast func(string)) (*SerialService, error) {
	port, err := serial.OpenPort(&serial.Config{
		Name: name, Baud: baudRate, ReadTimeout: readTimeout,
	})
	if err != nil {
		return nil, err
	}

	s := &SerialService{name: name, port: port, broadcast: broadcast}
	if err := s.check(); err != nil {
		port.Close()
		return nil, err
	}
	return s, nil
}

// check 发送基本的 AT 命令以验证连接。
func (s *SerialService) check() error {
	resp, err := s.SendATCommand("AT")
	if err != nil {
		return err
	}
	if !strings.Contains(resp, "OK") {
		return fmt.Errorf("command AT failed: %s", resp)
	}
	return nil
}

// Start 开始串口服务读取循环。
func (s *SerialService) Start() {
	s.SendATCommand("ATE0")      // 关闭回显
	s.SendATCommand("AT+CMGF=0") // 短信格式
	go s.readLoop()
}

// readLoop 持续读取串口输出并广播它。
func (s *SerialService) readLoop() {
	buf := make([]byte, bufferSize)
	for {
		s.Lock()
		n, err := s.port.Read(buf)
		s.Unlock()

		if n > 0 && s.broadcast != nil {
			s.broadcast(fmt.Sprintf("[%s] %s", s.name, string(buf[:n])))
		}

		if err != nil {
			time.Sleep(errorSleep)
		}
	}
}

// SendATCommand 发送 AT 命令并读取响应。
func (s *SerialService) SendATCommand(command string) (string, error) {
	return s.sendRawCommand(command, "\r\n", atTimeout)
}

// sendRawCommand 发送原始命令并读取响应。
func (s *SerialService) sendRawCommand(command, suffix string, timeout time.Duration) (string, error) {
	s.Lock()
	defer s.Unlock()

	_ = s.port.Flush()
	if _, err := s.port.Write([]byte(command + suffix)); err != nil {
		return "", err
	}

	var resp strings.Builder
	buf := make([]byte, bufferSize)

	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return "", errors.New("command timeout")
		}

		n, err := s.port.Read(buf)
		if n > 0 {
			resp.Write(buf[:n])
			str := resp.String()
			if strings.Contains(str, "OK") || strings.Contains(str, "ERROR") || strings.Contains(str, ">") {
				return strings.TrimSpace(str), nil
			}
		}
		if err != nil {
			if err == io.EOF {
				time.Sleep(errorSleep)
				continue
			}
			if resp.Len() > 0 {
				return strings.TrimSpace(resp.String()), nil
			}
			return "", err
		}
	}
}

// GetModemInfo 获取有关当前端口的基本信息。
func (s *SerialService) GetModemInfo() (*ModemInfo, error) {
	info := &ModemInfo{Port: s.name, Connected: true}
	cmds := map[*string]string{
		&info.Manufacturer: "AT+CGMI",
		&info.Model:        "AT+CGMM",
		&info.IMEI:         "AT+CGSN",
		&info.IMSI:         "AT+CIMI",
	}

	for ptr, cmd := range cmds {
		if resp, err := s.SendATCommand(cmd); err == nil {
			*ptr = extractValue(resp)
		}
	}

	info.Operator, _ = s.getOperator()
	info.PhoneNumber, _ = s.GetPhoneNumber()
	return info, nil
}

// getOperator 查询当前运营商名称。
func (s *SerialService) getOperator() (string, error) {
	resp, err := s.SendATCommand("AT+COPS?")
	if err != nil {
		return "", err
	}

	if m := regexp.MustCompile(`"([^"]+)"`).FindStringSubmatch(resp); len(m) > 1 {
		return m[1], nil
	}
	return "", errors.New("not found")
}

// GetPhoneNumber 查询电话号码。
func (s *SerialService) GetPhoneNumber() (string, error) {
	resp, err := s.SendATCommand("AT+CNUM")
	if err != nil {
		return "", err
	}

	if m := regexp.MustCompile(`\+CNUM:.*,"([^"]+)"`).FindStringSubmatch(resp); len(m) > 1 {
		return DecodeUCS2Hex(m[1]), nil
	}
	return "", errors.New("not found")
}

// GetSignalStrength 查询信号强度。
func (s *SerialService) GetSignalStrength() (*SignalStrength, error) {
	resp, err := s.SendATCommand("AT+CSQ")
	if err != nil {
		return nil, err
	}

	var rssi, qual int = -1, -1
	if _, err := fmt.Sscanf(extractValue(resp), "+CSQ: %d,%d", &rssi, &qual); err != nil {
		return nil, err
	}

	dbm := "unknown"
	if rssi >= 0 && rssi <= 31 {
		dbm = fmt.Sprintf("%d dBm", -113+rssi*2)
	}

	return &SignalStrength{
		RSSI:    rssi,
		Quality: qual,
		DBM:     dbm,
	}, nil
}

// ListSMS 获取短信列表。
func (s *SerialService) ListSMS() ([]SMS, error) {
	resp, err := s.SendATCommand("AT+CMGL=4")
	if err != nil {
		return nil, err
	}

	var parts []struct {
		SMS
		ref, total, seq int
	}

	// 按 +CMGL: 分割以处理多条消息，跳过第一个空部分
	chunks := strings.Split(resp, "+CMGL: ")
	for _, chunk := range chunks[1:] {
		lines := strings.SplitN(chunk, "\n", 2)
		if len(lines) < 2 {
			continue
		}

		// 解析元数据: index,stat,,length
		fields := strings.Split(strings.TrimSpace(lines[0]), ",")
		if len(fields) < 2 {
			continue
		}

		idx, _ := strconv.Atoi(strings.TrimSpace(fields[0]))
		stat, _ := strconv.Atoi(strings.TrimSpace(fields[1]))
		pduHex := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(lines[1]), "OK"))

		var sender, timestamp, message string
		ref, total, seq := 0, 1, 1
		if raw, decErr := hex.DecodeString(pduHex); decErr == nil {
			var msg sms.Message
			if _, rdErr := msg.ReadFrom(raw); rdErr == nil {
				sender = string(msg.Address)
				timestamp = time.Time(msg.ServiceCenterTime).Format("2006/01/02 15:04:05")
				message = msg.Text
				if msg.UserDataHeader.TotalNumber > 0 && msg.UserDataHeader.Sequence > 0 {
					total = msg.UserDataHeader.TotalNumber
					seq = msg.UserDataHeader.Sequence
					ref = msg.UserDataHeader.Tag
				}
			} else {
				message = "PDU Decode Error: " + rdErr.Error() + " Raw: " + pduHex
			}
		} else {
			message = "PDU Hex Decode Error: " + decErr.Error() + " Raw: " + pduHex
		}

		parts = append(parts, struct {
			SMS
			ref, total, seq int
		}{
			SMS: SMS{
				Index:   idx,
				Status:  getPDUStatus(stat),
				Number:  sender,
				Time:    timestamp,
				Message: message,
			},
			ref: ref, total: total, seq: seq,
		})
	}

	// 合并长短信
	var result []SMS
	merged := make(map[string][]struct {
		seq int
		msg string
	})

	for _, p := range parts {
		if p.total <= 1 {
			result = append(result, p.SMS)
			continue
		}
		key := fmt.Sprintf("%s_%d", p.Number, p.ref)
		merged[key] = append(merged[key], struct {
			seq int
			msg string
		}{p.seq, p.Message})
	}

	for key, fragments := range merged {
		sort.Slice(fragments, func(i, j int) bool { return fragments[i].seq < fragments[j].seq })
		fullMsg := ""
		for _, f := range fragments {
			fullMsg += f.msg
		}

		// 从部分中查找原始元数据（效率低但简单）
		for _, p := range parts {
			if fmt.Sprintf("%s_%d", p.Number, p.ref) == key && p.seq == 1 {
				p.SMS.Message = fullMsg
				result = append(result, p.SMS)
				break
			}
		}
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Index < result[j].Index })
	return result, nil
}

// SendSMS 发送短信。
func (s *SerialService) SendSMS(number, message string) error {
	msg := sms.Message{
		Type:    sms.MessageTypes.Submit,
		Address: sms.PhoneNumber(number),
		Text:    message,
	}
	if pdu.Is7BitEncodable(message) {
		msg.Encoding = sms.Encodings.Gsm7Bit
	} else {
		msg.Encoding = sms.Encodings.UCS2
	}

	length, octets, err := msg.PDU()
	if err != nil {
		return err
	}

	_, err = s.SendATCommand(fmt.Sprintf("AT+CMGS=%d", length))
	if err != nil {
		return err
	}

	pduHex := strings.ToUpper(hex.EncodeToString(octets))
	_, err = s.sendRawCommand(pduHex, "\x1A", 60*time.Second)
	return err
}

// DeleteSMS 删除指定索引的短信。
func (s *SerialService) DeleteSMS(index int) error {
	_, err := s.SendATCommand(fmt.Sprintf("AT+CMGD=%d", index))
	return err
}

// 辅助函数

func extractValue(response string) string {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != "OK" && !strings.HasPrefix(line, "AT") {
			return line
		}
	}
	return ""
}

func DecodeUCS2Hex(s string) string {
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return s
	}

	decoded, err := pdu.DecodeUcs2(b, false)
	if err != nil {
		return s
	}
	return decoded
}

func getPDUStatus(stat int) string {
	switch stat {
	case 0:
		return "REC UNREAD"
	case 1:
		return "REC READ"
	case 2:
		return "STO UNSENT"
	case 3:
		return "STO SENT"
	default:
		return "UNKNOWN"
	}
}
