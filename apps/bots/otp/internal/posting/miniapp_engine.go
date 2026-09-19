package posting

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Messenger interface {
	Send(tg.Chattable) (tg.Message, error)
	Request(tg.Chattable) (*tg.APIResponse, error)
}

type Engine struct {
	Store              Store
	API                API
	Bot                Messenger
	WebURL, MiniAppURL string
	LoadPhoto          func(context.Context, string) (tg.RequestFileData, error)
	Now                func() time.Time
	Searches           *SearchSessions
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}
func (e *Engine) say(chat int64, text string) error {
	_, err := e.Bot.Send(tg.NewMessage(chat, text))
	return err
}
func (e *Engine) persist(ctx context.Context, d *Draft, update int) error {
	d.Revision++
	d.LastUpdate = update
	return e.Store.Save(ctx, d)
}

func ValidMiniAppURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == ""
}

// The installed Telegram SDK predates WebAppInfo, but ReplyMarkup accepts
// arbitrary JSON. Keep Telegram's native web_app field, not an ordinary URL.
func (e *Engine) miniAppMessage(chat int64, text string, rows ...[]tg.InlineKeyboardButton) error {
	if !ValidMiniAppURL(e.MiniAppURL) {
		return errors.New("Mini App requires an HTTPS URL")
	}
	keyboard := make([][]any, 0, len(rows)+1)
	keyboard = append(keyboard, []any{map[string]any{"text": "➕ E'lon berish", "web_app": map[string]string{"url": e.MiniAppURL}}})
	for _, row := range rows {
		buttons := make([]any, len(row))
		for i, button := range row {
			buttons[i] = button
		}
		keyboard = append(keyboard, buttons)
	}
	m := tg.NewMessage(chat, text)
	m.ReplyMarkup = map[string]any{"inline_keyboard": keyboard}
	_, err := e.Bot.Send(m)
	return err
}

func postingEntry(u tg.Update) bool {
	if u.CallbackQuery != nil {
		return strings.HasPrefix(u.CallbackQuery.Data, "p:") || strings.HasPrefix(u.CallbackQuery.Data, "w:post:")
	}
	m := u.Message
	if m == nil || !m.IsCommand() {
		return false
	}
	switch m.Command() {
	case "post", "new", "preview", "back":
		return true
	case "start":
		return strings.TrimSpace(m.CommandArguments()) == "post"
	}
	return false
}

// Worker conversations still run in chat. Posting commands and even old form
// buttons only open the Mini App; they can never publish an old chat draft.
func (e *Engine) Handle(ctx context.Context, u tg.Update) error {
	m, sender := u.Message, int64(0)
	if u.CallbackQuery != nil {
		m = u.CallbackQuery.Message
		if u.CallbackQuery.From != nil {
			sender = u.CallbackQuery.From.ID
		}
	} else if m != nil && m.From != nil {
		sender = m.From.ID
	}
	if m == nil || m.Chat == nil || m.Chat.Type != "private" || sender <= 0 || sender != m.Chat.ID {
		return nil
	}
	if u.CallbackQuery != nil {
		_, _ = e.Bot.Request(tg.NewCallback(u.CallbackQuery.ID, ""))
	}
	d, err := e.Store.Load(ctx, sender)
	if err != nil {
		return err
	}
	if d == nil {
		d = freshDraft(sender)
		d.Step = "menu"
		if err := e.Store.Save(ctx, d); err != nil {
			return err
		}
	}
	if postingEntry(u) {
		if u.UpdateID <= d.LastUpdate && d.LastUpdate != 0 {
			return nil
		}
		d.Worker, d.Step = nil, "menu"
		if err := e.persist(ctx, d, u.UpdateID); err != nil {
			return err
		}
		e.StopJobSearch(sender)
		return e.miniAppMessage(sender, "Ish e'lonini Telegram ichida joylashtirish uchun pastdagi tugmani bosing.")
	}
	if handled, err := e.handleWorker(ctx, u, d); handled || err != nil {
		return err
	}
	if handled, err := e.handleJobSearch(ctx, u, d); handled || err != nil {
		return err
	}
	if u.UpdateID <= d.LastUpdate && d.LastUpdate != 0 {
		return nil
	}
	if err := e.persist(ctx, d, u.UpdateID); err != nil {
		return err
	}
	return e.workerHome(sender)
}
