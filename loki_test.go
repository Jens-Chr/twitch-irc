package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLokiClientPushesChatMessage(t *testing.T) {
	received := make(chan lokiPushRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/loki/api/v1/push" {
			t.Errorf("expected Loki push path, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type, got %s", r.Header.Get("Content-Type"))
		}

		var payload lokiPushRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("Loki payload could not be decoded: %v", err)
		}
		received <- payload
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newLokiClient(LokiConfig{
		Enabled: true,
		URL:     server.URL + "/loki/api/v1/push",
		Timeout: "1s",
		Labels: map[string]string{
			"job": "test-job",
		},
	})
	client.now = func() time.Time {
		return time.Unix(123, 456)
	}

	client.LogChatMessage(chatMessageLog{
		Direction: chatMessageDirectionReceived,
		Channel:   "test-channel",
		User:      "tester",
		Message:   "Hallo Loki!",
		MessageID: "message-123",
	})

	got := <-received
	if len(got.Streams) != 1 {
		t.Fatalf("expected one stream, got %d", len(got.Streams))
	}

	stream := got.Streams[0]
	if stream.Stream["job"] != "test-job" {
		t.Fatalf("expected job label test-job, got %q", stream.Stream["job"])
	}
	if stream.Stream["direction"] != chatMessageDirectionReceived {
		t.Fatalf("expected direction label %q, got %q", chatMessageDirectionReceived, stream.Stream["direction"])
	}
	if stream.Stream["channel"] != "test-channel" {
		t.Fatalf("expected channel label test-channel, got %q", stream.Stream["channel"])
	}
	if len(stream.Values) != 1 {
		t.Fatalf("expected one value, got %d", len(stream.Values))
	}
	if stream.Values[0][0] != "123000000456" {
		t.Fatalf("unexpected timestamp: %s", stream.Values[0][0])
	}

	var line map[string]string
	if err := json.Unmarshal([]byte(stream.Values[0][1]), &line); err != nil {
		t.Fatalf("Loki log line is not JSON: %v", err)
	}
	if line["direction"] != chatMessageDirectionReceived || line["channel"] != "test-channel" || line["user"] != "tester" || line["message"] != "Hallo Loki!" || line["message_id"] != "message-123" {
		t.Fatalf("unexpected Loki log line: %+v", line)
	}
}
