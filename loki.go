package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	chatMessageDirectionReceived = "received"
	chatMessageDirectionSent     = "sent"
)

type chatMessageLogger interface {
	LogChatMessage(chatMessageLog)
}

type multiChatMessageLogger []chatMessageLogger

type chatMessageLog struct {
	Direction        string
	Channel          string
	ChannelID        string
	User             string
	UserID           string
	Message          string
	MessageID        string
	ReplyToMessageID string
	Emotes           []chatMessageEmote
}

type chatMessageEmote struct {
	ID        string                     `json:"id"`
	Name      string                     `json:"name,omitempty"`
	Positions []chatMessageEmotePosition `json:"positions,omitempty"`
}

type chatMessageEmotePosition struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

func (l multiChatMessageLogger) LogChatMessage(entry chatMessageLog) {
	for _, logger := range l {
		if logger != nil {
			logger.LogChatMessage(entry)
		}
	}
}

type lokiClient struct {
	enabled bool
	url     string
	labels  map[string]string
	client  *http.Client
	now     func() time.Time
}

type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

func newLokiClient(cfg LokiConfig) *lokiClient {
	return &lokiClient{
		enabled: cfg.Enabled,
		url:     cfg.URL,
		labels:  cfg.Labels,
		client: &http.Client{
			Timeout: cfg.timeoutDuration(),
		},
		now: time.Now,
	}
}

func (c *lokiClient) LogChatMessage(entry chatMessageLog) {
	if c == nil || !c.enabled {
		return
	}

	line, err := json.Marshal(lokiLinePayload(entry))
	if err != nil {
		log.Printf("Loki Payload konnte nicht serialisiert werden: %v", err)
		return
	}

	payload := lokiPushRequest{
		Streams: []lokiStream{
			{
				Stream: c.streamLabels(entry),
				Values: [][2]string{
					{strconv.FormatInt(c.now().UnixNano(), 10), string(line)},
				},
			},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Loki Push konnte nicht serialisiert werden: %v", err)
		return
	}

	resp, err := c.client.Post(c.url, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		log.Printf("Loki Push konnte nicht aufgerufen werden: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		log.Printf("Loki Push antwortete mit Status %s", resp.Status)
	}
}

func (c *lokiClient) streamLabels(entry chatMessageLog) map[string]string {
	labels := make(map[string]string, len(c.labels)+2)
	for name, value := range c.labels {
		labels[name] = value
	}
	labels["direction"] = entry.Direction
	if entry.Channel != "" {
		labels["channel"] = entry.Channel
	}
	return labels
}

func lokiLinePayload(entry chatMessageLog) map[string]string {
	payload := map[string]string{
		"direction": entry.Direction,
		"channel":   entry.Channel,
		"message":   entry.Message,
	}

	if entry.User != "" {
		payload["user"] = entry.User
	}
	if entry.UserID != "" {
		payload["user_id"] = entry.UserID
	}
	if entry.ChannelID != "" {
		payload["channel_id"] = entry.ChannelID
	}
	if entry.MessageID != "" {
		payload["message_id"] = entry.MessageID
	}
	if entry.ReplyToMessageID != "" {
		payload["reply_to_message_id"] = entry.ReplyToMessageID
	}

	return payload
}
