// Package notify sends Telegram notifications.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Notifier sends notifications.
type Notifier struct {
	enabled bool
	token   string
	chatID  string
}

// New creates a Notifier. token is the Telegram bot token; chatID is the target chat.
func New(enabled bool, token, chatID string) *Notifier {
	return &Notifier{enabled: enabled, token: token, chatID: chatID}
}

// Send sends a message. If not enabled or token is empty, it is a no-op.
func (n *Notifier) Send(message string) error {
	if !n.enabled || n.token == "" || n.chatID == "" {
		return nil
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.token)
	payload := map[string]string{
		"chat_id": n.chatID,
		"text":    message,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram: unexpected status %d: %s", resp.StatusCode, raw)
	}
	return nil
}
