package main

import (
	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"strings"
	"testing"
)

func TestIsOwnContact(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		sender, contactUserID int64
		want                  bool
	}{
		{"own Telegram contact", 42, 42, true},
		{"somebody else's contact", 42, 99, false},
		{"plain contact card has no owner id", 42, 0, false},
		{"missing sender", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isOwnContact(tc.sender, tc.contactUserID); got != tc.want {
				t.Fatalf("isOwnContact(%d, %d)=%v, want %v", tc.sender, tc.contactUserID, got, tc.want)
			}
		})
	}
}

func TestJobDeepLinksNeverEnterOTPHandshake(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{{"/start job_123456789012345678901234", true}, {"/start job_invalid", true}, {"/start app_123456789012345678901234", true}, {"/start", true}, {"/start jobs", true}, {"/start opaque-login-token", false}, {"/jobs", true}} {
		m := &tg.Message{Text: tc.text, Entities: []tg.MessageEntity{{Type: "bot_command", Offset: 0, Length: len(strings.Split(tc.text, " ")[0])}}}
		if isBotCommand(m) != tc.want {
			t.Fatal("wrong dispatch", tc.text)
		}
	}
}
