package tgsend

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestChannelTransportResolvesRightsAndSendsOnlySilentChannelPost(t *testing.T) {
	for _, kind := range []string{"allowed", "group", "private", "no_permission"} {
		t.Run(kind, func(t *testing.T) {
			c := New("secret-token")
			sent := false
			c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := ""
				switch {
				case strings.HasSuffix(r.URL.Path, "/getMe"):
					body = `{"ok":true,"result":{"id":123,"is_bot":true,"username":"testbot"}}`
				case strings.HasSuffix(r.URL.Path, "/getChat"):
					typ := "channel"
					if kind == "group" {
						typ = "supergroup"
					}
					if kind == "private" {
						typ = "private"
					}
					body = `{"ok":true,"result":{"id":-1001234567890,"type":"` + typ + `"}}`
				case strings.HasSuffix(r.URL.Path, "/getChatMember"):
					post := "true"
					if kind == "no_permission" {
						post = "false"
					}
					body = `{"ok":true,"result":{"status":"administrator","can_post_messages":` + post + `}}`
				case strings.HasSuffix(r.URL.Path, "/sendMessage"):
					sent = true
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Fatal(err)
					}
					if payload["chat_id"] != float64(-1001234567890) || payload["disable_notification"] != true || payload["disable_web_page_preview"] != true || payload["parse_mode"] != "HTML" {
						t.Fatal("incorrect channel message")
					}
					if payload["from_chat_id"] != nil {
						t.Fatal("private message forwarded")
					}
					body = `{"ok":true,"result":{"message_id":12}}`
				default:
					t.Fatalf("unexpected Telegram action %s", r.URL.Path)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})
			info, err := c.ResolveChannel(context.Background(), "@jobs_test")
			if kind != "allowed" {
				if err == nil || sent {
					t.Fatal("non-channel/permission accepted")
				}
				return
			}
			if err != nil || info.BotUsername != "testbot" {
				t.Fatal("resolve failed", err)
			}
			id, err := c.SendChannelHTML(context.Background(), info.ID, "<b>Ish</b>", Button{Text: "Botda ko'rish", URL: "https://t.me/testbot?start=job_123456789012345678901234"})
			if err != nil || id != 12 || !sent {
				t.Fatal("send failed", err)
			}
		})
	}
}
func TestChannelTransportNeverLeaksTokenOrAcceptsPrivateTarget(t *testing.T) {
	c := New("never-log-this-token")
	calls := 0
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("transport " + r.URL.String())
	})
	_, err := c.SendChannelHTML(context.Background(), 123, "message", Button{Text: "Open", URL: "https://t.me/testbot"})
	if err == nil || calls != 0 {
		t.Fatal("private target sent")
	}
	_, err = c.ResolveChannel(context.Background(), "@jobs_test")
	if err == nil || strings.Contains(err.Error(), "never-log-this-token") {
		t.Fatal("token leaked")
	}
	for _, ref := range []string{"123", "-123", "group", "https://t.me/jobs_test", "@"} {
		if ValidChannelReference(ref) {
			t.Fatal("bad reference accepted", ref)
		}
	}
}
