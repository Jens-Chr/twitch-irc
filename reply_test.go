package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeMessenger struct {
	sayChannel string
	sayMessage string

	replyChannel     string
	replyToMessageID string
	replyMessage     string
	sayCalls         int
	replyCalls       int
}

func (f *fakeMessenger) Say(channel, message string) {
	f.sayCalls++
	f.sayChannel = channel
	f.sayMessage = message
}

func (f *fakeMessenger) Reply(channel, parentMessageID, message string) {
	f.replyCalls++
	f.replyChannel = channel
	f.replyToMessageID = parentMessageID
	f.replyMessage = message
}

func TestHandleReplyRequestSendsMessageToDefaultChannel(t *testing.T) {
	cfg := ReplyConfig{
		Enabled:          true,
		Token:            "secret",
		MaxMessageLength: 450,
	}
	messenger := &fakeMessenger{}
	handler := handleReplyRequest(cfg, messenger, "default-channel")

	req := httptest.NewRequest(http.MethodPost, "/n8n/reply", strings.NewReader(`{"message":"Hallo Chat!"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rr.Code)
	}
	if messenger.sayCalls != 1 {
		t.Fatalf("expected Say to be called once, got %d", messenger.sayCalls)
	}
	if messenger.sayChannel != "default-channel" || messenger.sayMessage != "Hallo Chat!" {
		t.Fatalf("unexpected Say payload: channel=%q message=%q", messenger.sayChannel, messenger.sayMessage)
	}
}

func TestHandleReplyRequestSendsThreadedReply(t *testing.T) {
	cfg := ReplyConfig{
		Enabled:          true,
		MaxMessageLength: 450,
	}
	messenger := &fakeMessenger{}
	handler := handleReplyRequest(cfg, messenger, "default-channel")

	req := httptest.NewRequest(http.MethodPost, "/n8n/reply", strings.NewReader(`{"message":"Antwort","channel":"#anderer-channel","reply_to_message_id":"abc123"}`))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rr.Code)
	}
	if messenger.replyCalls != 1 {
		t.Fatalf("expected Reply to be called once, got %d", messenger.replyCalls)
	}
	if messenger.replyChannel != "anderer-channel" || messenger.replyToMessageID != "abc123" || messenger.replyMessage != "Antwort" {
		t.Fatalf("unexpected Reply payload: channel=%q id=%q message=%q", messenger.replyChannel, messenger.replyToMessageID, messenger.replyMessage)
	}
}

func TestHandleReplyRequestRejectsUnauthorizedRequest(t *testing.T) {
	cfg := ReplyConfig{
		Enabled:          true,
		Token:            "secret",
		MaxMessageLength: 450,
	}
	messenger := &fakeMessenger{}
	handler := handleReplyRequest(cfg, messenger, "default-channel")

	req := httptest.NewRequest(http.MethodPost, "/n8n/reply", strings.NewReader(`{"message":"Hallo Chat!"}`))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
	if messenger.sayCalls != 0 || messenger.replyCalls != 0 {
		t.Fatalf("expected no twitch calls, got say=%d reply=%d", messenger.sayCalls, messenger.replyCalls)
	}
}
