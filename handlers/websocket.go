package handlers

import (
	"net/http"

	"web-modem/services"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return OriginAllowed(r)
	},
}

// HandleWebSocket 将 HTTP 连接升级为 WebSocket 连接
// 并将串口事件流式传输到客户端。
func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !AuthenticateRequest(r) {
		if apiToken() == "" {
			http.Error(w, "unauthorized: set API_TOKEN or ALLOW_INSECURE_NO_AUTH=true", http.StatusUnauthorized)
			return
		}
		http.Error(w, "unauthorized: provide token query or Authorization header", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// 订阅事件监听器
	ch, cancel := services.GetEventListener().Subscribe(100)
	defer cancel()

	// 流式传输消息
	for msg := range ch {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			return
		}
	}
}
