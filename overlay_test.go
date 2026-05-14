package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatOverlayKeepsBoundedSnapshot(t *testing.T) {
	overlay := newChatOverlay(OverlayConfig{MaxMessages: 2})
	overlay.now = func() time.Time {
		return time.Unix(123, 0)
	}

	overlay.LogChatMessage(chatMessageLog{Direction: chatMessageDirectionReceived, User: "one", Message: "erste"})
	overlay.LogChatMessage(chatMessageLog{Direction: chatMessageDirectionReceived, User: "two", Message: "zweite"})
	overlay.LogChatMessage(chatMessageLog{Direction: chatMessageDirectionReceived, User: "three", Message: "dritte"})

	snapshot := overlay.snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("expected two messages, got %d", len(snapshot))
	}
	if snapshot[0].User != "two" || snapshot[1].User != "three" {
		t.Fatalf("unexpected snapshot order: %+v", snapshot)
	}
	if snapshot[1].Timestamp != "1970-01-01T00:02:03Z" {
		t.Fatalf("unexpected timestamp: %q", snapshot[1].Timestamp)
	}
}

func TestChatOverlayDropsExpiredSnapshotMessages(t *testing.T) {
	currentTime := time.Unix(100, 0)
	overlay := newChatOverlay(OverlayConfig{
		MaxMessages: 10,
		MessageTTL:  "30s",
	})
	overlay.now = func() time.Time {
		return currentTime
	}

	overlay.LogChatMessage(chatMessageLog{Direction: chatMessageDirectionReceived, User: "old", Message: "zu alt"})
	currentTime = currentTime.Add(31 * time.Second)
	overlay.LogChatMessage(chatMessageLog{Direction: chatMessageDirectionReceived, User: "fresh", Message: "noch da"})

	snapshot := overlay.snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("expected one fresh message, got %d", len(snapshot))
	}
	if snapshot[0].User != "fresh" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestChatOverlayEventsStreamsSnapshotAndMessages(t *testing.T) {
	overlay := newChatOverlay(OverlayConfig{MaxMessages: 10})
	overlay.now = func() time.Time {
		return time.Unix(123, 0)
	}
	overlay.LogChatMessage(chatMessageLog{Direction: chatMessageDirectionReceived, Channel: "test", User: "alice", Message: "Hallo"})

	server := httptest.NewServer(overlay.handleEvents())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("request could not be created: %v", err)
	}

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("stream could not be opened: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("unexpected content type: %q", resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("expected X-Accel-Buffering=no, got %q", resp.Header.Get("X-Accel-Buffering"))
	}

	reader := bufio.NewReader(resp.Body)
	eventName, eventData := readSSEEvent(t, reader)
	if eventName != "snapshot" {
		t.Fatalf("expected snapshot event, got %q", eventName)
	}
	var snapshot []overlayMessage
	if err := json.Unmarshal([]byte(eventData), &snapshot); err != nil {
		t.Fatalf("snapshot could not be decoded: %v", err)
	}
	if len(snapshot) != 1 || snapshot[0].User != "alice" || snapshot[0].Message != "Hallo" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}

	overlay.LogChatMessage(chatMessageLog{Direction: chatMessageDirectionSent, Channel: "test", User: "bot", Message: "Antwort"})

	eventName, eventData = readSSEEvent(t, reader)
	if eventName != "message" {
		t.Fatalf("expected message event, got %q", eventName)
	}
	var message overlayMessage
	if err := json.Unmarshal([]byte(eventData), &message); err != nil {
		t.Fatalf("message could not be decoded: %v", err)
	}
	if message.Direction != chatMessageDirectionSent || message.User != "bot" || message.Message != "Antwort" {
		t.Fatalf("unexpected streamed message: %+v", message)
	}
}

func TestChatOverlayPageUsesRelativeEventPath(t *testing.T) {
	overlay := newChatOverlay(OverlayConfig{MaxMessages: 10})
	handler := overlay.handlePage(OverlayConfig{
		MaxMessages: 10,
		MessageTTL:  "30s",
	})

	req := httptest.NewRequest(http.MethodGet, "/proxy/twitch-chat", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "window.location.pathname.replace") || !strings.Contains(body, `path + "/events"`) {
		t.Fatalf("overlay page does not build the event stream path relative to the current browser URL")
	}
	if !strings.Contains(body, `<link rel="stylesheet" href="?asset=chat.css">`) {
		t.Fatalf("overlay page does not reference the external stylesheet")
	}
	if strings.Contains(body, "<style>") {
		t.Fatalf("overlay page should not contain inline css")
	}
	if !strings.Contains(body, `"messageTtlMs":30000`) {
		t.Fatalf("overlay page does not include the configured message ttl: %s", body)
	}
}

func TestChatOverlayPageServesStylesheetAsset(t *testing.T) {
	overlay := newChatOverlay(OverlayConfig{MaxMessages: 10})
	handler := overlay.handlePage(OverlayConfig{
		MaxMessages: 10,
		MessageTTL:  "30s",
	})

	req := httptest.NewRequest(http.MethodGet, "/proxy/twitch-chat?asset=chat.css", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if rr.Header().Get("Content-Type") != "text/css; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", rr.Header().Get("Content-Type"))
	}
	if !strings.Contains(rr.Body.String(), "#messages") {
		t.Fatalf("stylesheet asset does not contain expected overlay css")
	}
}

func TestChatOverlayEventsRejectsPost(t *testing.T) {
	overlay := newChatOverlay(OverlayConfig{MaxMessages: 10})
	req := httptest.NewRequest(http.MethodPost, "/overlay/chat/events", nil)
	rr := httptest.NewRecorder()

	overlay.handleEvents().ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
	if rr.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("expected Allow header %q, got %q", http.MethodGet, rr.Header().Get("Allow"))
	}
}

func readSSEEvent(t *testing.T, reader *bufio.Reader) (string, string) {
	t.Helper()

	var eventName string
	var dataLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("SSE event could not be read: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return eventName, strings.Join(dataLines, "\n")
		}
		if strings.HasPrefix(line, "event: ") {
			eventName = strings.TrimPrefix(line, "event: ")
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
}
