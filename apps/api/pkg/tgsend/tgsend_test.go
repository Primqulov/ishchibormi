package tgsend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSendHTMLPreservesRecipientAndRetryInformation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		code   int
		retry  time.Duration
	}{
		{"success", 200, `{"ok":true}`, 0, 0},
		{"rate limited", 429, `{"ok":false,"error_code":429,"parameters":{"retry_after":43}}`, 429, 43 * time.Second},
		{"blocked", 403, `{"ok":false,"error_code":403}`, 403, 0},
		{"upstream failure", 502, `not json`, 502, 0},
		{"unconfirmed", 200, `not json`, 200, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req sendReq
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req.ChatID != 321 || req.Text != "<b>Ariza</b>" || req.ParseMode != "HTML" || r.Method != http.MethodPost {
					t.Errorf("wrong send request: %+v", req)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := New("test-token")
			client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				req, err := http.NewRequestWithContext(r.Context(), r.Method, server.URL, r.Body)
				if err != nil {
					return nil, err
				}
				return http.DefaultTransport.RoundTrip(req)
			})
			err := client.SendHTML(context.Background(), 321, "<b>Ariza</b>")
			if tc.code == 0 {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var apiErr *APIError
			if !errors.Is(err, ErrUnreachable) || !errors.As(err, &apiErr) || apiErr.Code != tc.code || apiErr.RetryAfter != tc.retry {
				t.Fatalf("wrong error: %v", err)
			}
		})
	}
}

func TestSendHTMLDoesNotLeakBotToken(t *testing.T) {
	client := New("secret-token")
	client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("failed: " + r.URL.String())
	})
	err := client.SendHTML(context.Background(), 123, "test")
	if !errors.Is(err, ErrUnreachable) || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("transport error leaked credentials: %v", err)
	}
}

func TestSendHTMLWithButtonAttachesOrDropsMarkup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		btn    Button
		wantOn bool
	}{
		{"https", Button{Text: "Arizani ko'rish", URL: "https://ishchibormi.uz/applications/abc"}, true},
		{"numeric loopback", Button{Text: "Ko'rish", URL: "http://127.0.0.1:3000/notifications"}, true},
		{"localhost rejected by Telegram", Button{Text: "Ko'rish", URL: "http://localhost:3000/notifications"}, false},
		{"uppercase localhost", Button{Text: "Ko'rish", URL: "http://LOCALHOST:3000/notifications"}, false},
		{"single label host", Button{Text: "Ko'rish", URL: "http://web:3000/notifications"}, false},
		// Telegram answers 400 to these, and the caller treats 400 as permanent
		// — so the button is dropped and the message still goes out.
		{"empty url", Button{Text: "Ko'rish"}, false},
		{"empty text", Button{URL: "https://ishchibormi.uz/"}, false},
		{"custom scheme", Button{Text: "Ko'rish", URL: "ishchibormi://applications/abc"}, false},
		{"javascript", Button{Text: "Ko'rish", URL: "javascript:alert(1)"}, false},
		{"relative", Button{Text: "Ko'rish", URL: "/notifications"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got sendReq
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Error(err)
				}
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer server.Close()
			client := New("test-token")
			client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				req, err := http.NewRequestWithContext(r.Context(), r.Method, server.URL, r.Body)
				if err != nil {
					return nil, err
				}
				return http.DefaultTransport.RoundTrip(req)
			})
			if err := client.SendHTMLWithButton(context.Background(), 321, "<b>Ariza</b>", tc.btn); err != nil {
				t.Fatal(err)
			}
			if !tc.wantOn {
				if got.ReplyMarkup != nil || got.NoPreview {
					t.Fatalf("invalid button was sent: %+v", got)
				}
				return
			}
			if got.ReplyMarkup == nil || len(got.ReplyMarkup.InlineKeyboard) != 1 ||
				len(got.ReplyMarkup.InlineKeyboard[0]) != 1 {
				t.Fatalf("missing single-button keyboard: %+v", got.ReplyMarkup)
			}
			only := got.ReplyMarkup.InlineKeyboard[0][0]
			if only.Text != tc.btn.Text || only.URL != tc.btn.URL {
				t.Fatalf("wrong button %+v", only)
			}
			// The link is in the button; a preview card under it is noise.
			if !got.NoPreview {
				t.Fatal("link preview left enabled")
			}
		})
	}
}

func TestRejectedButtonFallsBackWithoutRetryingOtherFailures(t *testing.T) {
	for _, tc := range []struct {
		name           string
		status         int
		body           string
		fallbackStatus int
		fallbackBody   string
		wantCalls      int
		wantCode       int
	}{
		{"invalid button", 400, `{"ok":false,"error_code":400,"description":"Bad Request: inline keyboard button URL 'http://bad.example' is invalid: Wrong HTTP URL"}`, 200, `{"ok":true}`, 2, 0},
		{"fallback blocked", 400, `{"ok":false,"error_code":400,"description":"Bad Request: inline keyboard button URL is invalid"}`, 403, `{"ok":false,"error_code":403}`, 2, 403},
		{"chat missing", 400, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`, 0, "", 1, 400},
		{"text rejected", 400, `{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities"}`, 0, "", 1, 400},
		{"blocked", 403, `{"ok":false,"error_code":403}`, 0, "", 1, 403},
		{"rate limited", 429, `{"ok":false,"error_code":429,"parameters":{"retry_after":30}}`, 0, "", 1, 429},
		{"success", 200, `{"ok":true}`, 0, "", 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req sendReq
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req.ChatID != 321 || req.Text != "<b>Ariza</b>" || req.ParseMode != "HTML" {
					t.Errorf("message changed: %+v", req)
				}
				if calls == 1 {
					if req.ReplyMarkup == nil {
						t.Error("first request missing button")
					}
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
					return
				}
				if req.ReplyMarkup != nil {
					t.Error("fallback retained rejected button")
				}
				if tc.fallbackStatus == 0 {
					t.Error("unexpected fallback")
					w.WriteHeader(500)
					return
				}
				w.WriteHeader(tc.fallbackStatus)
				_, _ = w.Write([]byte(tc.fallbackBody))
			}))
			defer server.Close()
			client := New("test-token")
			client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				req, err := http.NewRequestWithContext(r.Context(), r.Method, server.URL, r.Body)
				if err != nil {
					return nil, err
				}
				return http.DefaultTransport.RoundTrip(req)
			})
			err := client.SendHTMLWithButton(context.Background(), 321, "<b>Ariza</b>", Button{Text: "Arizani ko'rish", URL: "http://bad.example/application/abc"})
			if calls != tc.wantCalls {
				t.Errorf("made %d sends, want %d", calls, tc.wantCalls)
			}
			var apiErr *APIError
			if tc.wantCode == 0 {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.As(err, &apiErr) || apiErr.Code != tc.wantCode {
				t.Fatalf("wrong failure: %v", err)
			}
			if tc.status == 429 && apiErr.RetryAfter != 30*time.Second {
				t.Fatal("lost retry_after")
			}
		})
	}
}

func TestNotificationBotActionPreservesURLAndCallbackContract(t *testing.T) {
	var got sendReq
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	client := New("test-token")
	client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		req, err := http.NewRequestWithContext(r.Context(), r.Method, server.URL, r.Body)
		if err != nil {
			return nil, err
		}
		return http.DefaultTransport.RoundTrip(req)
	})
	btn := Button{Text: "Saytda ochish", URL: "https://ishchibormi.uz/applications/test", BotText: "Botda ochish", BotCallback: "w:app:123456789012345678901234"}
	if err := client.SendHTMLWithButton(context.Background(), 123, "Ariza", btn); err != nil {
		t.Fatal(err)
	}
	if got.ReplyMarkup == nil || len(got.ReplyMarkup.InlineKeyboard) != 2 {
		t.Fatal("missing bot action")
	}
	web, bot := got.ReplyMarkup.InlineKeyboard[0][0], got.ReplyMarkup.InlineKeyboard[1][0]
	if web.URL != btn.URL || web.CallbackData != "" || bot.URL != "" || bot.CallbackData != btn.BotCallback {
		t.Fatal("ambiguous URL/callback buttons")
	}
}

func TestButtonTransportFailureDoesNotRetryOrLeakToken(t *testing.T) {
	client := New("secret-token")
	calls := 0
	client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("failed: " + r.URL.String())
	})
	err := client.SendHTMLWithButton(context.Background(), 123, "test", Button{Text: "Ariza", URL: "https://ishchibormi.uz/applications/abc"})
	if calls != 1 || !errors.Is(err, ErrUnreachable) || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("unsafe transport handling: calls=%d, err=%v", calls, err)
	}
}
