package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	twitch "github.com/gempir/go-twitch-irc/v4"
	"github.com/prometheus/client_golang/prometheus"
)

var chatMessages = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "twitch_chat_messages_total",
		Help: "Total number of chat messages",
	},
)

func main() {
	configPath := flag.String("config", "config.toml", "Path to the TOML configuration file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Konfiguration konnte nicht geladen werden: %v", err)
	}

	n8nClient := &http.Client{
		Timeout: cfg.N8N.timeoutDuration(),
	}
	lokiClient := newLokiClient(cfg.Loki)
	if cfg.Loki.Enabled {
		log.Printf("Loki Push ist aktiviert: %s", cfg.Loki.URL)
	} else {
		log.Println("Loki Push ist deaktiviert")
	}

	twitchClient := twitch.NewClient(cfg.Twitch.Username, cfg.Twitch.OAuth)

	prometheus.MustRegister(chatMessages)
	startHTTPServer(cfg.Server, cfg.Metrics, cfg.Reply, twitchClient, cfg.Twitch.Channel, cfg.Twitch.Username, lokiClient)

	twitchClient.OnPrivateMessage(func(message twitch.PrivateMessage) {
		fmt.Printf("[%s]: %s\n", message.User.DisplayName, message.Message)

		chatMessages.Inc()
		lokiClient.LogChatMessage(chatMessageLog{
			Direction: chatMessageDirectionReceived,
			Channel:   message.Channel,
			User:      message.User.DisplayName,
			Message:   message.Message,
			MessageID: message.ID,
		})
		sendToN8N(n8nClient, cfg.N8N.URL, message)
	})

	twitchClient.Join(cfg.Twitch.Channel)
	log.Printf("Verbinde mit Twitch-Channel %q", cfg.Twitch.Channel)

	if err := twitchClient.Connect(); err != nil {
		log.Fatal(err)
	}
}

func sendToN8N(client *http.Client, webhookURL string, msg twitch.PrivateMessage) {
	payload := map[string]string{
		"user":       msg.User.DisplayName,
		"message":    msg.Message,
		"channel":    msg.Channel,
		"message_id": msg.ID,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("n8n Payload konnte nicht serialisiert werden: %v", err)
		return
	}

	resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		log.Printf("n8n Webhook konnte nicht aufgerufen werden: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		log.Printf("n8n Webhook antwortete mit Status %s", resp.Status)
	}
}
