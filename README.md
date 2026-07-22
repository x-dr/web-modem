# web-modem

基于 Go 的 Web 串口 Modem 控制台：提供 REST API 与 WebSocket 事件流，浏览器内发送 AT 命令、收发短信。

## 功能

- 扫描并连接 USB 串口调制解调器（Linux / macOS / Windows）
- AT 命令透传、信号与设备信息查询
- 短信列表 / 发送 / 删除
- WebSocket 实时串口事件

## 快速开始

### 要求

- Go 1.21+
- 有权限访问串口设备的运行环境

### 构建与运行

```bash
go build -o web-modem .
export API_TOKEN='your-strong-token'   # 生产环境必填
./web-modem
# 浏览器打开 http://localhost:8080
```

本地无鉴权调试（**禁止公网暴露**）：

```bash
export ALLOW_INSECURE_NO_AUTH=true
./web-modem
```

### 环境变量

| 变量 | 说明 | 默认 |
|------|------|------|
| `PORT` | HTTP 端口 | `8080` |
| `API_TOKEN` | API/WS 鉴权令牌 | 空（未设且未开 insecure 时拒绝 API） |
| `ALLOW_INSECURE_NO_AUTH` | `true` 时允许无令牌访问 | 关闭 |
| `CORS_ALLOWED_ORIGINS` | 额外 CORS 来源，逗号分隔；`*` 允许全部 | 仅同源 |

完整示例见 [`.env.example`](.env.example)。

### API 鉴权

除静态前端资源外，`/api/v1/*` 与 `/ws` 需要鉴权（在配置了 `API_TOKEN` 或未开启 insecure 时）：

```http
Authorization: Bearer <API_TOKEN>
# 或
X-API-Token: <API_TOKEN>
```

WebSocket（浏览器不便自定义 Header 时）：

```
ws://localhost:8080/ws?token=<API_TOKEN>
```

前端页可将令牌保存在浏览器本地（输入框），请求与 WS 会自动附带。

### 主要接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/modems` | 扫描串口 |
| POST | `/api/v1/modem/at` | 发送 AT 命令 |
| GET | `/api/v1/modem/info?port=` | 设备信息 |
| GET | `/api/v1/modem/signal?port=` | 信号强度 |
| GET | `/api/v1/modem/sms/list?port=` | 短信列表 |
| POST | `/api/v1/modem/sms/send` | 发送短信 |
| POST | `/api/v1/modem/sms/delete` | 删除短信 |
| WS | `/ws` | 事件流 |

### 串口枚举

| 平台 | 候选设备 |
|------|----------|
| Linux | `/dev/ttyUSB*`、`/dev/ttyACM*` |
| macOS | 同上 + `/dev/tty.usb*`、`/dev/cu.usb*` |
| Windows | `COM1`–`COM32`（连接成功才加入池） |

## 安全建议

1. **生产必须设置 `API_TOKEN`**，勿使用 `ALLOW_INSECURE_NO_AUTH`。
2. 不要将服务直接裸奔到公网；建议置于反向代理 / VPN / 内网之后。
3. 按需配置 `CORS_ALLOWED_ORIGINS`，避免 `*`。
4. 勿将编译产物、`.env` 提交进仓库（已由 `.gitignore` 覆盖）。

## 项目结构

```
main.go                 # HTTP 入口
handlers/               # REST + WebSocket + 鉴权
modem/                  # 串口 AT/SMS
services/               # 串口池 + 事件广播
frontend/               # 静态控制台
```

## CI / 同步

- GitHub Actions：多架构 `linux/windows` 构建与 Release（见 `.github/workflows/build.yml`）。
- CNB：`master` push 可通过 `tencentcom/git-sync` 同步到 GitHub；**默认关闭 force push**（`PLUGIN_FORCE=false`），避免覆盖远端历史。若确需强推，请在 `.cnb.yml` 中显式改为 `true` 并知悉风险。

## License

按仓库原有许可使用。
