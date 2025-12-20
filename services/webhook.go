package services

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type WebhookMessage struct {
	Type    string      `json:"type"` // "sms" or "call"
	Payload interface{} `json:"payload"`
}

func SendWebhook(url string, msg WebhookMessage) error {
	b, _ := json.Marshal(msg)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
