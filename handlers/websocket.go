package handlers

import (
	"net/http"

	"github.com/gorilla/websocket"
	"modem-manager/services"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleWebSocket upgrades the HTTP connection to a WebSocket connection
// and streams serial events to the client.
func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Subscribe to event listener
	ch, cancel := services.GetEventListener().Subscribe(100)
	defer cancel()

	// Stream messages
	for msg := range ch {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			return
		}
	}
}
