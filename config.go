package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Server  ServerConfig  `toml:"server"`
	Twitch  TwitchConfig  `toml:"twitch"`
	N8N     N8NConfig     `toml:"n8n"`
	Loki    LokiConfig    `toml:"loki"`
	Metrics MetricsConfig `toml:"metrics"`
	Reply   ReplyConfig   `toml:"reply"`
	Overlay OverlayConfig `toml:"overlay"`
}

type ServerConfig struct {
	Address string `toml:"address"`
}

type TwitchConfig struct {
	Username string `toml:"username"`
	OAuth    string `toml:"oauth"`
	Channel  string `toml:"channel"`
}

type N8NConfig struct {
	URL     string `toml:"url"`
	Timeout string `toml:"timeout"`
}

type LokiConfig struct {
	Enabled bool              `toml:"enabled"`
	URL     string            `toml:"url"`
	Timeout string            `toml:"timeout"`
	Labels  map[string]string `toml:"labels"`
}

type MetricsConfig struct {
	// Address is accepted for older config files; server.address is used.
	Address string `toml:"address"`
	Path    string `toml:"path"`
}

type ReplyConfig struct {
	Enabled bool `toml:"enabled"`
	// Address is accepted for older config files; server.address is used.
	Address          string `toml:"address"`
	Path             string `toml:"path"`
	Token            string `toml:"token"`
	MaxMessageLength int    `toml:"max_message_length"`
}

type OverlayConfig struct {
	Enabled     bool   `toml:"enabled"`
	Path        string `toml:"path"`
	MaxMessages int    `toml:"max_messages"`
	MessageTTL  string `toml:"message_ttl"`
}

func defaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Address: ":2112",
		},
		N8N: N8NConfig{
			Timeout: "5s",
		},
		Loki: LokiConfig{
			Timeout: "2s",
			Labels: map[string]string{
				"job": "twitch-irc",
			},
		},
		Metrics: MetricsConfig{
			Path: "/metrics",
		},
		Reply: ReplyConfig{
			Enabled:          true,
			Path:             "/n8n/reply",
			MaxMessageLength: 450,
		},
		Overlay: OverlayConfig{
			Enabled:     true,
			Path:        "/overlay/chat",
			MaxMessages: 60,
			MessageTTL:  "45s",
		},
	}
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()

	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("Datei %q konnte nicht geoeffnet werden: %w", path, err)
	}
	defer file.Close()

	if err := toml.NewDecoder(file).DisallowUnknownFields().Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("TOML konnte nicht gelesen werden: %w", err)
	}

	cfg.normalize()
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c *Config) normalize() {
	c.Server.Address = strings.TrimSpace(c.Server.Address)
	c.Twitch.Username = strings.TrimSpace(c.Twitch.Username)
	c.Twitch.OAuth = strings.TrimSpace(c.Twitch.OAuth)
	c.Twitch.Channel = strings.TrimPrefix(strings.TrimSpace(c.Twitch.Channel), "#")
	c.N8N.URL = strings.TrimSpace(c.N8N.URL)
	c.N8N.Timeout = strings.TrimSpace(c.N8N.Timeout)
	c.Loki.URL = strings.TrimSpace(c.Loki.URL)
	c.Loki.Timeout = strings.TrimSpace(c.Loki.Timeout)
	c.Loki.Labels = normalizeLabels(c.Loki.Labels)
	c.Metrics.Address = strings.TrimSpace(c.Metrics.Address)
	c.Metrics.Path = strings.TrimSpace(c.Metrics.Path)
	c.Reply.Address = strings.TrimSpace(c.Reply.Address)
	c.Reply.Path = strings.TrimSpace(c.Reply.Path)
	c.Reply.Token = strings.TrimSpace(c.Reply.Token)
	c.Overlay.Path = normalizeOverlayPath(c.Overlay.Path)
	c.Overlay.MessageTTL = strings.TrimSpace(c.Overlay.MessageTTL)
}

func (c Config) validate() error {
	var problems []string

	if c.Server.Address == "" {
		problems = append(problems, "server.address fehlt")
	}

	if c.Twitch.Username == "" {
		problems = append(problems, "twitch.username fehlt")
	}
	if c.Twitch.OAuth == "" {
		problems = append(problems, "twitch.oauth fehlt")
	} else if !strings.HasPrefix(c.Twitch.OAuth, "oauth:") {
		problems = append(problems, "twitch.oauth muss mit \"oauth:\" beginnen")
	}
	if c.Twitch.Channel == "" {
		problems = append(problems, "twitch.channel fehlt")
	}

	if c.N8N.URL == "" {
		problems = append(problems, "n8n.url fehlt")
	} else if err := validateHTTPURL(c.N8N.URL); err != nil {
		problems = append(problems, fmt.Sprintf("n8n.url ist ungueltig: %v", err))
	}
	if _, err := time.ParseDuration(c.N8N.Timeout); err != nil {
		problems = append(problems, fmt.Sprintf("n8n.timeout ist ungueltig: %v", err))
	}

	if c.Loki.Enabled {
		if c.Loki.URL == "" {
			problems = append(problems, "loki.url fehlt")
		} else if err := validateHTTPURL(c.Loki.URL); err != nil {
			problems = append(problems, fmt.Sprintf("loki.url ist ungueltig: %v", err))
		}
	}
	if c.Loki.Timeout != "" {
		if _, err := time.ParseDuration(c.Loki.Timeout); err != nil {
			problems = append(problems, fmt.Sprintf("loki.timeout ist ungueltig: %v", err))
		}
	}
	for labelName := range c.Loki.Labels {
		if !isValidLokiLabelName(labelName) {
			problems = append(problems, fmt.Sprintf("loki.labels.%s ist kein gueltiger Loki-Labelname", labelName))
		}
	}

	if c.Metrics.Path == "" {
		problems = append(problems, "metrics.path fehlt")
	} else if !strings.HasPrefix(c.Metrics.Path, "/") {
		problems = append(problems, "metrics.path muss mit \"/\" beginnen")
	}

	if c.Reply.Enabled {
		if c.Reply.Path == "" {
			problems = append(problems, "reply.path fehlt")
		} else if !strings.HasPrefix(c.Reply.Path, "/") {
			problems = append(problems, "reply.path muss mit \"/\" beginnen")
		}
		if c.Reply.MaxMessageLength <= 0 {
			problems = append(problems, "reply.max_message_length muss groesser als 0 sein")
		}
		if c.Metrics.Path == c.Reply.Path {
			problems = append(problems, "metrics.path und reply.path muessen unterschiedlich sein")
		}
	}

	if c.Overlay.Enabled {
		overlayEventPath := c.Overlay.eventPath()
		if c.Overlay.Path == "" {
			problems = append(problems, "overlay.path fehlt")
		} else if c.Overlay.Path == "/" {
			problems = append(problems, "overlay.path darf nicht \"/\" sein")
		} else if !strings.HasPrefix(c.Overlay.Path, "/") {
			problems = append(problems, "overlay.path muss mit \"/\" beginnen")
		}
		if c.Overlay.MaxMessages <= 0 {
			problems = append(problems, "overlay.max_messages muss groesser als 0 sein")
		}
		if c.Overlay.MessageTTL == "" {
			problems = append(problems, "overlay.message_ttl fehlt")
		} else {
			messageTTL, err := time.ParseDuration(c.Overlay.MessageTTL)
			if err != nil {
				problems = append(problems, fmt.Sprintf("overlay.message_ttl ist ungueltig: %v", err))
			} else if messageTTL <= 0 {
				problems = append(problems, "overlay.message_ttl muss groesser als 0 sein")
			}
		}
		if c.Metrics.Path == c.Overlay.Path || c.Metrics.Path == overlayEventPath {
			problems = append(problems, "metrics.path und overlay.path muessen unterschiedlich sein")
		}
		if c.Reply.Enabled && (c.Reply.Path == c.Overlay.Path || c.Reply.Path == overlayEventPath) {
			problems = append(problems, "reply.path und overlay.path muessen unterschiedlich sein")
		}
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}

	return nil
}

func validateHTTPURL(rawURL string) error {
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return err
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("Schema muss http oder https sein")
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("Host fehlt")
	}
	return nil
}

func (c N8NConfig) timeoutDuration() time.Duration {
	timeout, _ := time.ParseDuration(c.Timeout)
	return timeout
}

func (c LokiConfig) timeoutDuration() time.Duration {
	timeout, _ := time.ParseDuration(c.Timeout)
	return timeout
}

func (c OverlayConfig) eventPath() string {
	return strings.TrimRight(c.Path, "/") + "/events"
}

func (c OverlayConfig) messageTTLDuration() time.Duration {
	timeout, _ := time.ParseDuration(c.MessageTTL)
	return timeout
}

func normalizeOverlayPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return path
	}
	return strings.TrimRight(path, "/")
}

func normalizeLabels(labels map[string]string) map[string]string {
	normalized := make(map[string]string, len(labels))
	for name, value := range labels {
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" || value == "" {
			continue
		}
		normalized[name] = value
	}
	return normalized
}

func isValidLokiLabelName(name string) bool {
	if name == "" {
		return false
	}

	for index, char := range name {
		if index == 0 {
			if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || char == '_' || char == ':' {
				continue
			}
			return false
		}

		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' || char == ':' {
			continue
		}
		return false
	}

	return true
}
