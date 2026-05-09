package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type twitchMessenger interface {
	Say(channel, message string)
	Reply(channel, parentMessageID, message string)
}

type replyRequest struct {
	Message          string `json:"message"`
	Channel          string `json:"channel,omitempty"`
	ReplyToMessageID string `json:"reply_to_message_id,omitempty"`
}

type replyResponse struct {
	Status  string `json:"status"`
	Channel string `json:"channel"`
}

func handleReplyRequest(cfg ReplyConfig, twitchClient twitchMessenger, defaultChannel string, chatLogger chatMessageLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if !isAuthorized(r, cfg.Token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req replyRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid json: %v", err), http.StatusBadRequest)
			return
		}

		message := strings.TrimSpace(req.Message)
		if message == "" {
			http.Error(w, "message is required", http.StatusBadRequest)
			return
		}
		if len(message) > cfg.MaxMessageLength {
			http.Error(w, fmt.Sprintf("message must not exceed %d characters", cfg.MaxMessageLength), http.StatusBadRequest)
			return
		}

		channel := normalizeChannel(req.Channel)
		if channel == "" {
			channel = defaultChannel
		}

		replyToMessageID := strings.TrimSpace(req.ReplyToMessageID)
		if replyToMessageID != "" {
			twitchClient.Reply(channel, replyToMessageID, message)
		} else {
			twitchClient.Say(channel, message)
		}
		if chatLogger != nil {
			chatLogger.LogChatMessage(chatMessageLog{
				Direction:        chatMessageDirectionSent,
				Channel:          channel,
				Message:          message,
				ReplyToMessageID: replyToMessageID,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(replyResponse{Status: "queued", Channel: channel}); err != nil {
			log.Printf("n8n Rueckkanal-Antwort konnte nicht geschrieben werden: %v", err)
		}
	}
}

func isAuthorized(r *http.Request, token string) bool {
	if token == "" {
		return true
	}

	if r.Header.Get("X-N8N-Token") == token {
		return true
	}

	authHeader := r.Header.Get("Authorization")
	return strings.TrimSpace(authHeader) == "Bearer "+token
}

func normalizeChannel(channel string) string {
	return strings.TrimPrefix(strings.TrimSpace(channel), "#")
}
