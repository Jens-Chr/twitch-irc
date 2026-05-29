package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type chatOverlay struct {
	maxMessages int
	messageTTL  time.Duration
	now         func() time.Time

	mu          sync.Mutex
	nextID      uint64
	messages    []overlayMessage
	subscribers map[chan overlayMessage]struct{}
	channel     string
	channelID   string
}

type overlayMessage struct {
	ID               string             `json:"id"`
	Direction        string             `json:"direction"`
	Channel          string             `json:"channel"`
	ChannelID        string             `json:"channelId,omitempty"`
	User             string             `json:"user"`
	UserID           string             `json:"userId,omitempty"`
	Message          string             `json:"message"`
	MessageID        string             `json:"messageId,omitempty"`
	Emotes           []chatMessageEmote `json:"emotes,omitempty"`
	ReplyToMessageID string             `json:"replyToMessageId,omitempty"`
	Timestamp        string             `json:"timestamp"`
	createdAt        time.Time
}

type overlayPageConfig struct {
	MaxMessages  int    `json:"maxMessages"`
	MessageTTLMs int64  `json:"messageTtlMs"`
	Channel      string `json:"channel,omitempty"`
	ChannelID    string `json:"channelId,omitempty"`
}

func newChatOverlay(cfg OverlayConfig) *chatOverlay {
	maxMessages := cfg.MaxMessages
	if maxMessages <= 0 {
		maxMessages = 60
	}
	messageTTL := cfg.messageTTLDuration()
	if messageTTL <= 0 {
		messageTTL = 45 * time.Second
	}

	return &chatOverlay{
		maxMessages: maxMessages,
		messageTTL:  messageTTL,
		now:         time.Now,
		subscribers: make(map[chan overlayMessage]struct{}),
	}
}

func (o *chatOverlay) LogChatMessage(entry chatMessageLog) {
	if o == nil || strings.TrimSpace(entry.Message) == "" {
		return
	}

	now := o.now().UTC()
	channel := strings.TrimPrefix(strings.TrimSpace(entry.Channel), "#")
	channelID := strings.TrimSpace(entry.ChannelID)

	o.mu.Lock()
	o.pruneExpiredLocked(now)
	if channel != "" {
		o.channel = channel
	}
	if channelID != "" {
		o.channelID = channelID
	} else {
		channelID = o.channelID
	}
	messageChannel := entry.Channel
	if strings.TrimSpace(messageChannel) == "" {
		messageChannel = o.channel
	}
	o.nextID++
	message := overlayMessage{
		ID:               fmt.Sprintf("%d", o.nextID),
		Direction:        entry.Direction,
		Channel:          messageChannel,
		ChannelID:        channelID,
		User:             entry.User,
		UserID:           entry.UserID,
		Message:          entry.Message,
		MessageID:        entry.MessageID,
		ReplyToMessageID: entry.ReplyToMessageID,
		Emotes:           entry.Emotes,
		Timestamp:        now.Format(time.RFC3339Nano),
		createdAt:        now,
	}

	o.messages = append(o.messages, message)
	if len(o.messages) > o.maxMessages {
		o.messages = o.messages[len(o.messages)-o.maxMessages:]
	}

	subscribers := make([]chan overlayMessage, 0, len(o.subscribers))
	for subscriber := range o.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	o.mu.Unlock()

	for _, subscriber := range subscribers {
		select {
		case subscriber <- message:
		default:
		}
	}
}

func (o *chatOverlay) handlePage(cfg OverlayConfig, channel string) http.HandlerFunc {
	channel = strings.TrimPrefix(strings.TrimSpace(channel), "#")

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		assetName := r.URL.Query().Get("asset")
		if assetName == "chat.css" {
			serveOverlayStylesheet(w, r)
			return
		}
		if assetName != "" {
			http.NotFound(w, r)
			return
		}

		pageConfig := overlayPageConfig{
			MaxMessages:  cfg.MaxMessages,
			MessageTTLMs: cfg.messageTTLDuration().Milliseconds(),
			Channel:      channel,
		}
		currentChannel, currentChannelID := o.channelContext()
		if currentChannel != "" {
			pageConfig.Channel = currentChannel
		}
		if currentChannelID != "" {
			pageConfig.ChannelID = currentChannelID
		}
		configJSON, err := json.Marshal(pageConfig)
		if err != nil {
			log.Printf("Overlay-Konfiguration konnte nicht serialisiert werden: %v", err)
			configJSON = []byte(`{"maxMessages":60,"messageTtlMs":45000}`)
		}

		data := struct {
			ConfigJSON template.JS
		}{
			ConfigJSON: template.JS(configJSON),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := overlayTemplate.ExecuteTemplate(w, "chat.html", data); err != nil {
			log.Printf("Overlay-Seite konnte nicht geschrieben werden: %v", err)
		}
	}
}

func (o *chatOverlay) handleEvents() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		events, snapshot, unsubscribe := o.subscribe()
		defer unsubscribe()

		if err := writeOverlayEvent(w, "snapshot", snapshot); err != nil {
			return
		}
		flusher.Flush()

		keepAlive := time.NewTicker(15 * time.Second)
		defer keepAlive.Stop()

		for {
			select {
			case message := <-events:
				if err := writeOverlayEvent(w, "message", message); err != nil {
					return
				}
				flusher.Flush()
			case <-keepAlive.C:
				if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	}
}

func (o *chatOverlay) SetChannelContext(channel, channelID string) {
	if o == nil {
		return
	}
	channel = strings.TrimPrefix(strings.TrimSpace(channel), "#")
	channelID = strings.TrimSpace(channelID)
	if channel == "" && channelID == "" {
		return
	}

	o.mu.Lock()
	if channel != "" {
		o.channel = channel
	}
	if channelID != "" {
		o.channelID = channelID
	}
	o.mu.Unlock()
}

func (o *chatOverlay) channelContext() (string, string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.channel, o.channelID
}

func (o *chatOverlay) snapshot() []overlayMessage {
	now := o.now().UTC()

	o.mu.Lock()
	defer o.mu.Unlock()

	o.pruneExpiredLocked(now)
	return append([]overlayMessage(nil), o.messages...)
}

func (o *chatOverlay) subscribe() (<-chan overlayMessage, []overlayMessage, func()) {
	events := make(chan overlayMessage, 16)

	o.mu.Lock()
	o.pruneExpiredLocked(o.now().UTC())
	snapshot := append([]overlayMessage(nil), o.messages...)
	o.subscribers[events] = struct{}{}
	o.mu.Unlock()

	unsubscribe := func() {
		o.mu.Lock()
		delete(o.subscribers, events)
		o.mu.Unlock()
	}

	return events, snapshot, unsubscribe
}

func (o *chatOverlay) pruneExpiredLocked(now time.Time) {
	if o.messageTTL <= 0 || len(o.messages) == 0 {
		return
	}

	keepFrom := 0
	for keepFrom < len(o.messages) {
		if now.Sub(o.messages[keepFrom].createdAt) <= o.messageTTL {
			break
		}
		keepFrom++
	}
	if keepFrom > 0 {
		o.messages = append([]overlayMessage(nil), o.messages[keepFrom:]...)
	}
}

func writeOverlayEvent(w http.ResponseWriter, eventName string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "event: %s\n", eventName); err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(w, "\n")
	return err
}

func serveOverlayStylesheet(w http.ResponseWriter, r *http.Request) {
	stylesheet, err := overlayAssets.ReadFile("overlay_assets/chat.css")
	if err != nil {
		http.Error(w, "stylesheet not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(stylesheet); err != nil {
		log.Printf("Overlay-Stylesheet konnte nicht geschrieben werden: %v", err)
	}
}

//go:embed overlay_assets/chat.html overlay_assets/chat.css
var overlayAssets embed.FS

var overlayTemplate = template.Must(template.ParseFS(overlayAssets, "overlay_assets/chat.html"))
