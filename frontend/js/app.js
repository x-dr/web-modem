class ModemManager {
    constructor() {
        this.apiBase = '/api/v1';
        this.wsUrl = `ws://${location.host}/ws`;
        this.ws = null;
        this.init();
    }

    init() {
        this.refreshPorts();
        this.setupWebSocket();
        this.setupSMSCounter();
    }

    // ---------- WebSocket ----------

    setupWebSocket() {
        this.ws = new WebSocket(this.wsUrl);
        this.ws.onopen = () => this.addLog('WebSocket 连接已建立');
        this.ws.onmessage = (event) => this.addLog('收到: ' + event.data);
        this.ws.onerror = (error) => this.addLog('WebSocket 错误: ' + error);
        this.ws.onclose = () => {
            this.addLog('WebSocket 连接已断开');
            setTimeout(() => this.setupWebSocket(), 5000);
        };
    }

    // ---------- API ----------

    async apiRequest(endpoint, method = 'GET', body = null) {
        const options = { method, headers: { 'Content-Type': 'application/json' } };
        if (body) options.body = JSON.stringify(body);
        const response = await fetch(this.apiBase + endpoint, options);
        const data = await response.json();
        if (!response.ok) {
            const msg = data.error || '请求失败';
            this.showError(msg);
            throw new Error(msg);
        }
        return data;
    }

    // ---------- Port & actions ----------

    async refreshPorts() {
        try {
            const ports = await this.apiRequest('/modems');
            const select = document.getElementById('portSelect');
            const current = select.value;
            select.innerHTML = '<option value="">-- 选择串口 --</option>';
            ports.forEach(port => {
                const option = document.createElement('option');
                option.value = port.path;
                option.textContent = port.name + (port.connected ? ' ✅' : '');
                select.appendChild(option);
            });
            // 优先保持当前选择，否则选第一个已连接
            if (current && ports.find(p => p.path === current && p.connected)) {
                select.value = current;
            } else {
                const connected = ports.find(p => p.connected);
                if (connected) select.value = connected.path;
            }
            this.addLog('已刷新串口列表');
            const selectedPath = select.value;
            const selectedPort = ports.find(p => p.path === selectedPath && p.connected);
            this.updateConnectionStatus(!!selectedPort, selectedPort ? selectedPort.path : '');
            select.onchange = () => {
                const val = select.value;
                const item = ports.find(p => p.path === val && p.connected);
                this.updateConnectionStatus(!!item, item ? item.path : '');
            };
        } catch (error) {
            console.error('刷新串口失败:', error);
        }
    }

    async sendATCommand() {
        const port = this.getSelectedPort();
        if (!port) return;
        const command = document.getElementById('atCommand').value.trim();
        if (!command) {
            this.showError('请输入 AT 命令');
            return;
        }
        try {
            const result = await this.apiRequest('/modem/send', 'POST', { port, command });
            this.addToTerminal(`> ${command}`);
            this.addToTerminal(result.response || '');
            document.getElementById('atCommand').value = '';
        } catch (error) {
            console.error('发送命令失败:', error);
        }
    }

    async getModemInfo() {
        const port = this.getSelectedPort();
        if (!port) return;
        try {
            const info = await this.apiRequest(`/modem/info?port=${encodeURIComponent(port)}`);
            this.displayModemInfo(info);
        } catch (error) {
            console.error('获取信息失败:', error);
        }
    }

    async getSignalStrength() {
        const port = this.getSelectedPort();
        if (!port) return;
        try {
            const signal = await this.apiRequest(`/modem/signal?port=${encodeURIComponent(port)}`);
            this.displaySignalInfo(signal);
        } catch (error) {
            console.error('获取信号强度失败:', error);
        }
    }

    async listSMS() {
        const port = this.getSelectedPort();
        if (!port) return;
        try {
            this.addLog('正在读取短信列表（PDU 模式）...');
            const smsList = await this.apiRequest(`/modem/sms/list?port=${encodeURIComponent(port)}`);
            this.displaySMSList(smsList);
            this.addLog(`已读取 ${smsList.length} 条短信`);
        } catch (error) {
            console.error('获取短信列表失败:', error);
        }
    }

    async sendSMS() {
        const port = this.getSelectedPort();
        if (!port) return;
        const number = document.getElementById('smsNumber').value.trim();
        const message = document.getElementById('smsMessage').value.trim();
        if (!number || !message) {
            this.showError('请输入号码和短信内容');
            return;
        }
        try {
            this.addLog('正在发送短信（支持中文和长短信）...');
            await this.apiRequest('/modem/sms/send', 'POST', { port, number, message });
            this.showSuccess('短信发送成功！');
            document.getElementById('smsNumber').value = '';
            document.getElementById('smsMessage').value = '';
            this.updateSMSCounter();
        } catch (error) {
            console.error('发送短信失败:', error);
        }
    }

    // ---------- SMS counter ----------

    setupSMSCounter() {
        const textarea = document.getElementById('smsMessage');
        if (!textarea) return;
        const existing = document.getElementById('smsCounter');
        if (!existing) {
            const counter = document.createElement('div');
            counter.id = 'smsCounter';
            counter.style.cssText = 'margin-top: 5px; color: #666; font-size: 12px;';
            textarea.parentNode.appendChild(counter);
        }
        textarea.addEventListener('input', () => this.updateSMSCounter());
        this.updateSMSCounter();
    }

    updateSMSCounter() {
        const textarea = document.getElementById('smsMessage');
        const counter = document.getElementById('smsCounter');
        if (!textarea || !counter) return;
        const message = textarea.value;
        const hasUnicode = /[^\x00-\x7F]/.test(message);
        const maxChars = hasUnicode ? (message.length <= 70 ? 70 : 67) : (message.length <= 160 ? 160 : 153);
        const parts = Math.ceil(message.length / maxChars) || 1;
        const encoding = hasUnicode ? 'UCS2 (中文)' : 'GSM 7-bit';
        counter.innerHTML = `<span>字符数: ${message.length} / ${maxChars}</span> | <span>短信条数: ${parts}</span> | <span>编码: ${encoding}</span>`;
        if (parts > 3) {
            counter.style.color = '#ff4444';
            counter.innerHTML += ` <strong>⚠️ 消息过长，将分为 ${parts} 条发送</strong>`;
        } else if (parts > 1) {
            counter.style.color = '#ff9800';
        } else {
            counter.style.color = '#666';
        }
    }

    // ---------- UI helpers ----------

    getSelectedPort() {
        const port = document.getElementById('portSelect').value;
        if (!port) {
            this.showError('请选择可用串口');
            return null;
        }
        return port;
    }

    updateConnectionStatus(connected, portLabel = '') {
        const statusElement = document.getElementById('connectionStatus');
        const statusText = document.getElementById('statusText');
        if (connected) {
            statusElement.classList.add('connected');
            statusText.textContent = portLabel ? `已选择 ${portLabel}` : '已连接';
        } else {
            statusElement.classList.remove('connected');
            statusText.textContent = '未连接';
        }
    }

    addToTerminal(text) {
        const terminal = document.getElementById('terminal');
        terminal.innerHTML += this.escapeHtml(text) + '\n';
        terminal.scrollTop = terminal.scrollHeight;
    }

    addLog(text) {
        const log = document.getElementById('log');
        const timestamp = new Date().toLocaleTimeString();
        log.innerHTML += `[${timestamp}] ${this.escapeHtml(text)}\n`;
        log.scrollTop = log.scrollHeight;
    }

    clearLog() {
        document.getElementById('log').innerHTML = '';
    }

    showError(message) {
        this.addLog('❌ 错误: ' + message);
        alert('错误: ' + message);
    }

    showSuccess(message) {
        this.addLog('✅ 成功: ' + message);
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // ---------- Render ----------

    displayModemInfo(info) {
        const container = document.getElementById('modemInfo');
        container.innerHTML = `
            <div class="info-item"><span class="info-label">串口:</span><span class="info-value">${info.port || '-'}</span></div>
            <div class="info-item"><span class="info-label">制造商:</span><span class="info-value">${info.manufacturer || '-'}</span></div>
            <div class="info-item"><span class="info-label">型号:</span><span class="info-value">${info.model || '-'}</span></div>
            <div class="info-item"><span class="info-label">IMEI:</span><span class="info-value">${info.imei || '-'}</span></div>
            <div class="info-item"><span class="info-label">手机号:</span><span class="info-value">${info.phoneNumber || '-'}</span></div>
            <div class="info-item"><span class="info-label">运营商:</span><span class="info-value">${info.operator || '-'}</span></div>
        `;
    }

    displaySignalInfo(signal) {
        const container = document.getElementById('modemInfo');
        container.innerHTML = `
            <div class="info-item"><span class="info-label">信号强度 (RSSI):</span><span class="info-value">${signal.rssi}</span></div>
            <div class="info-item"><span class="info-label">信号质量:</span><span class="info-value">${signal.quality}</span></div>
            <div class="info-item"><span class="info-label">dBm:</span><span class="info-value">${signal.dbm}</span></div>
        `;
    }

    displaySMSList(smsList) {
        const container = document.getElementById('smsList');
        if (!smsList || smsList.length === 0) {
            container.innerHTML = '<p>暂无短信</p>';
            return;
        }
        container.innerHTML = smsList.map(sms => `
            <div class="sms-item">
                <div class="sms-header">
                    <span class="sms-number">📱 ${this.escapeHtml(sms.number)}</span>
                    <span class="sms-time">🕐 ${this.escapeHtml(sms.time)}</span>
                </div>
                <div class="sms-message">${this.escapeHtml(sms.message)}</div>
            </div>
        `).join('');
    }
}

const app = new ModemManager();
document.getElementById('atCommand')?.addEventListener('keypress', (e) => {
    if (e.key === 'Enter') {
        app.sendATCommand();
    }
});