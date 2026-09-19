package posting

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func hasMiniAppButton(h *harness) bool {
	for _, raw := range h.b.messages {
		msg, ok := raw.(tg.MessageConfig)
		if !ok {
			continue
		}
		data, _ := json.Marshal(msg.ReplyMarkup)
		var markup struct {
			Rows [][]struct {
				Text   string `json:"text"`
				URL    string `json:"url"`
				WebApp *struct {
					URL string `json:"url"`
				} `json:"web_app"`
			} `json:"inline_keyboard"`
		}
		if json.Unmarshal(data, &markup) != nil {
			continue
		}
		for _, row := range markup.Rows {
			for _, button := range row {
				if strings.Contains(button.Text, "E'lon berish") && button.WebApp != nil && button.WebApp.URL == h.e.MiniAppURL && button.URL == "" {
					return true
				}
			}
		}
	}
	return false
}

func TestPostAndMenuUseNativeMiniAppButton(t *testing.T) {
	for _, command := range []string{"/start", "/menu", "/post", "/start post", "/preview", "/back"} {
		t.Run(command, func(t *testing.T) {
			h := setup(t)
			h.text(command)
			if !hasMiniAppButton(h) || h.a.published != 0 || h.a.logins != 0 || h.s.draft.Step != "menu" {
				t.Fatal("posting still starts chat form instead of Mini App")
			}
		})
	}
}

func TestLegacyPostingButtonsCannotPublishOrEraseOldDraft(t *testing.T) {
	h := setup(t)
	h.formToPreview("total")
	h.s.draft.Step = "publishing"
	for _, button := range []string{"p:old:4:publish:", "p:old:4:agree:", "w:post:"} {
		h.update++
		h.rawClick(button)
	}
	if !hasMiniAppButton(h) || h.a.published != 0 || h.a.removed != 0 || h.s.draft.Form.Title != "Hovli tozalash" {
		t.Fatal("old chat buttons published or destroyed draft data")
	}
	before := len(h.b.messages)
	if err := h.e.Handle(context.Background(), tg.Update{UpdateID: h.update + 1, Message: &tg.Message{Chat: &tg.Chat{ID: -10, Type: "group"}, From: &tg.User{ID: 42}, Text: "/post"}}); err != nil || len(h.b.messages) != before {
		t.Fatal("Mini App button sent outside private chat")
	}
}

func TestMiniAppRequiresHTTPS(t *testing.T) {
	for _, raw := range []string{"http://localhost:3000/miniapp/post", "javascript:alert(1)", "https://user:secret@example.test/miniapp/post", ""} {
		if ValidMiniAppURL(raw) {
			t.Fatal("unsafe Mini App URL accepted")
		}
	}
	if !ValidMiniAppURL("https://test.trycloudflare.com/miniapp/post") {
		t.Fatal("HTTPS Mini App URL rejected")
	}
}
