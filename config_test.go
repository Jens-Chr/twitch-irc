package main

import "testing"

func TestDefaultConfigEnablesOverlay(t *testing.T) {
	cfg := defaultConfig()

	if !cfg.Overlay.Enabled {
		t.Fatal("expected overlay to be enabled by default")
	}
	if cfg.Overlay.Path != "/overlay/chat" {
		t.Fatalf("unexpected overlay path: %q", cfg.Overlay.Path)
	}
	if cfg.Overlay.eventPath() != "/overlay/chat/events" {
		t.Fatalf("unexpected overlay event path: %q", cfg.Overlay.eventPath())
	}
}

func TestConfigRejectsOverlayPathCollisions(t *testing.T) {
	cfg := defaultConfig()
	cfg.Twitch.Username = "bot"
	cfg.Twitch.OAuth = "oauth:token"
	cfg.Twitch.Channel = "channel"
	cfg.N8N.URL = "http://localhost/webhook"
	cfg.Overlay.Path = "/metrics"

	err := cfg.validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if err.Error() != "metrics.path und overlay.path muessen unterschiedlich sein" {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
