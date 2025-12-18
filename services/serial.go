package services

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/tarm/serial"

	"modem-manager/models"
)

const (
	// AT Commands
	cmdEchoOff      = "ATE0"
	cmdTextMode     = "AT+CMGF=1"
	cmdCheck        = "AT"
	cmdListSMS      = "AT+CMGL=\"ALL\""
	cmdSendSMS      = "AT+CMGS=\"%s\""
	cmdSignal       = "AT+CSQ"
	cmdManufacturer = "AT+CGMI"
	cmdModel        = "AT+CGMM"
	cmdIMEI         = "AT+CGSN"
	cmdIMSI         = "AT+CIMI"
	cmdOperator     = "AT+COPS?"
	cmdNumber       = "AT+CNUM"
	
	// Timeouts and Delays
	readTimeout     = 100 * time.Millisecond
	errorSleep      = 100 * time.Millisecond
	bufferSize      = 128
)

// SerialService encapsulates reading, writing, and monitoring of a single serial port.
type SerialService struct {
	name string
	port *serial.Port
	sync.Mutex
}

// NewSerialService attempts to connect and initialize the serial service.
func NewSerialService(name string, baudRate int) (*SerialService, error) {
	port, err := serial.OpenPort(&serial.Config{
		Name: name, Baud: baudRate, ReadTimeout: readTimeout,
	})
	if err != nil {
		return nil, err
	}

	s := &SerialService{name: name, port: port}
	if err := s.check(); err != nil {
		port.Close()
		return nil, err
	}
	return s, nil
}

// check sends a basic AT command to verify the connection.
func (s *SerialService) check() error {
	resp, err := s.SendATCommand(cmdCheck)
	if err != nil {
		return err
	}
	if !strings.Contains(resp, "OK") {
		return fmt.Errorf("command AT failed: %s", resp)
	}
	return nil
}

// Start begins the serial service read loop.
func (s *SerialService) Start() {
	s.SendATCommand(cmdEchoOff)  // Turn off echo
	s.SendATCommand(cmdTextMode) // Set text mode
	go s.readLoop()
}

// readLoop continuously reads serial output and broadcasts it.
func (s *SerialService) readLoop() {
	buf := make([]byte, bufferSize)
	for {
		s.Lock()
		n, err := s.port.Read(buf)
		s.Unlock()
		
		if n > 0 {
			GetEventListener().Broadcast(fmt.Sprintf("[%s] %s", s.name, string(buf[:n])))
		}
		
		if err != nil {
			time.Sleep(errorSleep)
		}
	}
}

// SendATCommand sends an AT command and reads the response.
func (s *SerialService) SendATCommand(command string) (string, error) {
	return s.sendRawCommand(command, "\r\n")
}

// sendRawCommand sends a raw command and reads the response.
func (s *SerialService) sendRawCommand(command, suffix string) (string, error) {
	s.Lock()
	defer s.Unlock()

	_ = s.port.Flush()
	if _, err := s.port.Write([]byte(command + suffix)); err != nil {
		return "", err
	}

	var resp strings.Builder
	buf := make([]byte, bufferSize)
	
	for {
		n, err := s.port.Read(buf)
		if n > 0 {
			resp.Write(buf[:n])
			str := resp.String()
			if strings.Contains(str, "OK") || strings.Contains(str, "ERROR") || strings.Contains(str, ">") {
				return str, nil
			}
		}
		if err != nil {
			if resp.Len() > 0 { return resp.String(), nil }
			return "", err
		}
	}
}

// GetModemInfo retrieves basic information about the current port.
func (s *SerialService) GetModemInfo() (*models.ModemInfo, error) {
	info := &models.ModemInfo{Port: s.name, Connected: true}
	cmds := map[*string]string{
		&info.Manufacturer: cmdManufacturer,
		&info.Model:        cmdModel,
		&info.IMEI:         cmdIMEI,
		&info.IMSI:         cmdIMSI,
	}
	
	for ptr, cmd := range cmds {
		if resp, err := s.SendATCommand(cmd); err == nil {
			*ptr = extractValue(resp)
		}
	}
	
	if resp, err := s.SendATCommand(cmdOperator); err == nil {
		info.Operator = extractOperator(resp)
	}
	
	info.PhoneNumber, _ = s.GetPhoneNumber()
	return info, nil
}

// GetPhoneNumber queries the phone number.
func (s *SerialService) GetPhoneNumber() (string, error) {
	resp, err := s.SendATCommand(cmdNumber)
	if err != nil {
		return "", err
	}
	if m := regexp.MustCompile(`\+CNUM:.*,"([^"]+)"`).FindStringSubmatch(resp); len(m) > 1 {
		return m[1], nil
	}
	return "", errors.New("not found")
}

// GetSignalStrength queries the signal strength.
func (s *SerialService) GetSignalStrength() (*models.SignalStrength, error) {
	resp, err := s.SendATCommand(cmdSignal)
	if err != nil {
		return nil, err
	}
	
	var rssi, qual int
	if _, err := fmt.Sscanf(extractValue(resp), "+CSQ: %d,%d", &rssi, &qual); err != nil {
		return nil, err
	}
	
	return &models.SignalStrength{
		RSSI:    rssi,
		Quality: qual,
		DBM:     fmt.Sprintf("%d dBm", -113+rssi*2),
	}, nil
}

// ListSMS retrieves the list of SMS messages.
func (s *SerialService) ListSMS() ([]models.SMS, error) {
	resp, err := s.SendATCommand(cmdListSMS)
	if err != nil {
		return nil, err
	}

	var parts []struct { models.SMS; ref, total, seq int }
	
	// Split by +CMGL: to handle multiple messages
	chunks := strings.Split(resp, "+CMGL: ")
	for _, chunk := range chunks[1:] { // Skip first empty part
		lines := strings.SplitN(chunk, "\n", 2)
		if len(lines) < 2 { continue }
		
		meta, content := lines[0], strings.TrimSpace(strings.TrimSuffix(lines[1], "OK"))
		// Parse meta: index,"status","oa",,"scts"
		fields := strings.Split(meta, ",")
		if len(fields) < 5 { continue }
		
		idx, _ := strconv.Atoi(strings.TrimSpace(fields[0]))
		txt, ref, tot, seq := decodeHexSMS(content)
		
		parts = append(parts, struct{ models.SMS; ref, total, seq int }{
			SMS: models.SMS{
				Index:   idx,
				Status:  strings.Trim(fields[1], `"`),
				Number:  strings.Trim(fields[2], `"`),
				Time:    strings.Trim(fields[4], `"`),
				Message: txt,
			},
			ref: ref, total: tot, seq: seq,
		})
	}

	// Merge long SMS
	merged := make(map[string][]struct{ seq int; msg string })
	var result []models.SMS
	
	for _, p := range parts {
		if p.total <= 1 {
			result = append(result, p.SMS)
			continue
		}
		key := fmt.Sprintf("%s_%d", p.Number, p.ref)
		merged[key] = append(merged[key], struct{ seq int; msg string }{p.seq, p.Message})
	}
	
	for key, fragments := range merged {
		sort.Slice(fragments, func(i, j int) bool { return fragments[i].seq < fragments[j].seq })
		fullMsg := ""
		for _, f := range fragments { fullMsg += f.msg }
		
		// Find original metadata from parts (inefficient but simple)
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

// SendSMS sends an SMS message.
func (s *SerialService) SendSMS(number, message string) error {
	if _, err := s.SendATCommand(fmt.Sprintf(cmdSendSMS, number)); err != nil {
		return err
	}
	_, err := s.sendRawCommand(message, "\x1A") // \x1A is Ctrl+Z
	return err
}

func extractValue(response string) string {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != "OK" && !strings.HasPrefix(line, "AT") {
			return line
		}
	}
	return ""
}

func extractOperator(response string) string {
	if m := regexp.MustCompile(`"([^"]+)"`).FindStringSubmatch(response); len(m) > 1 {
		return m[1]
	}
	return ""
}

func decodeHexSMS(content string) (string, int, int, int) {
	content = strings.TrimSpace(content)
	b, err := hex.DecodeString(content)
	if err != nil || len(content)%2 != 0 { return content, 0, 1, 1 }

	offset, ref, total, seq := 0, 0, 1, 1
	
	// Check for Concatenated SMS UDH (User Data Header)
	// 05 00 03 [ref] [total] [seq]
	if len(b) > 6 && b[0] == 5 && b[1] == 0 && b[2] == 3 {
		offset, ref, total, seq = 6, int(b[3]), int(b[4]), int(b[5])
	} else if len(b) > 7 && b[0] == 6 && b[1] == 8 && b[2] == 4 {
		// 06 08 04 [ref1] [ref2] [total] [seq]
		offset, ref, total, seq = 7, int(b[3])<<8|int(b[4]), int(b[5]), int(b[6])
	}

	if (len(b)-offset)%2 != 0 { return content, 0, 1, 1 }
	
	// Decode UTF-16BE
	u16 := make([]uint16, (len(b)-offset)/2)
	for i := range u16 {
		u16[i] = uint16(b[offset+i*2])<<8 | uint16(b[offset+i*2+1])
	}
	return string(utf16.Decode(u16)), ref, total, seq
}
