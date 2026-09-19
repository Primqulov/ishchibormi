// Package tgsend is a minimal Telegram Bot API client for pushing messages to a
// user's chat from the API process.
//
// The auth bot (cmd/bot) owns the long-polling connection — only one process may
// poll a token at a time — but sending is a plain stateless HTTP call, so the API
// can use the same token to push a message without disturbing the bot.
package tgsend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrUnreachable means the message could not be delivered to the user's chat:
// no token configured, no known chat id, the user never pressed /start, or they
// blocked the bot. Callers surface this as "open the bot and press start".
var ErrUnreachable = errors.New("telegram chat unreachable")

type Client struct {
	token string
	http  *http.Client
}

func New(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 10 * time.Second}}
}

// Configured reports whether a bot token is available at all.
func (c *Client) Configured() bool { return c != nil && c.token != "" }

// Button preserves the web/app destination and can add a bot action.
// Callback updates are handled by the existing bot poller, never by the API.
type Button struct {
	Text        string
	URL         string
	BotText     string
	BotCallback string
	// Actions — xabar ostidagi amal tugmalari (bitta qator). Havola va
	// "botda ochish" tugmalaridan farqi: bular foydalanuvchini boshqa ekranga
	// olib bormaydi, balki amalni o'sha yerdan boshlaydi.
	Actions []BotAction
}

// BotAction — callback tugmasi. Callback matnini bot o'zi tushunadi
// (apps/bots/otp: "w:accept:<id>" kabi) va 64 baytdan oshmasligi kerak —
// Telegram cheklovi.
type BotAction struct {
	Text     string
	Callback string
}

// Valid rejects known-invalid buttons. Bot API rejects the
// WHOLE sendMessage call with 400 when a button URL is missing or uses a scheme
// it does not allow, and notification.Telegram treats 400 as permanent — so a
// malformed button would silently cost the user the message, not just the
// button. Callers check this and fall back to a plain send.
func (b Button) Valid() bool {
	if strings.TrimSpace(b.Text) == "" || b.URL == "" {
		return false
	}
	u, err := url.Parse(b.URL)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return false
	}
	// Telegram rejects single-label hosts such as localhost, even though
	// browsers accept them. Numeric loopback addresses work for local tests.
	if net.ParseIP(u.Hostname()) == nil && !strings.Contains(strings.TrimSuffix(u.Hostname(), "."), ".") {
		return false
	}
	// tg:// is accepted by Telegram too, but nothing here produces one and
	// allowing it would let a caller smuggle in an in-app action link.
	return u.Scheme == "https" || u.Scheme == "http"
}

type inlineButton struct {
	Text         string `json:"text"`
	URL          string `json:"url,omitempty"`
	CallbackData string `json:"callback_data,omitempty"`
}

type inlineKeyboard struct {
	InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

type sendReq struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
	// ParseMode is omitempty so the zero request in tests stays minimal.
	ParseMode string `json:"parse_mode,omitempty"`
	// NoPreview suppresses Telegram's link card. The destination link now lives
	// in the button, and a card under every job notification would double the
	// height of the message for no added information.
	NoPreview   bool            `json:"disable_web_page_preview,omitempty"`
	ReplyMarkup *inlineKeyboard `json:"reply_markup,omitempty"`
}

type sendResp struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// APIError retains retry information without exposing Telegram's request URL
// (which contains the bot token). Existing callers can still use ErrUnreachable.
type APIError struct {
	Code       int
	RetryAfter time.Duration
	// Reason is a fixed category, never Telegram's raw description or URL.
	Reason string
}

func (e *APIError) Error() string { return fmt.Sprintf("%s: telegram %d", ErrUnreachable, e.Code) }
func (e *APIError) Unwrap() error { return ErrUnreachable }

// SendHTML delivers an HTML-formatted message to chatID. Text must already be
// escaped with [EscapeHTML] everywhere it embeds untrusted or symbol-bearing
// content. Any failure — transport, non-2xx, or ok:false — is reported as
// [ErrUnreachable] so callers have a single "fall back to the bot link" branch.
func (c *Client) SendHTML(ctx context.Context, chatID int64, html string) error {
	return c.send(ctx, sendReq{ChatID: chatID, Text: html, ParseMode: "HTML"})
}

// SendHTMLWithButton is [SendHTML] plus one inline URL button under the message.
//
// An invalid button is dropped rather than sent: losing the button is a cosmetic
// regression, losing the notification is not (see [Button.Valid]).
func (c *Client) SendHTMLWithButton(ctx context.Context, chatID int64, html string, btn Button) error {
	req := sendReq{ChatID: chatID, Text: html, ParseMode: "HTML"}
	if btn.Valid() {
		req.NoPreview = true
		req.ReplyMarkup = &inlineKeyboard{
			InlineKeyboard: [][]inlineButton{{{Text: btn.Text, URL: btn.URL}}},
		}
	}
	// Amal tugmalari havoladan keyin, "botda ochish"dan oldin: qaror qabul
	// qilish eng ko'p ishlatiladigan amal, lekin havola xabarning asosiy
	// maqsadi bo'lib qoladi.
	if row := actionRow(btn.Actions); len(row) > 0 {
		if req.ReplyMarkup == nil {
			req.ReplyMarkup = &inlineKeyboard{}
		}
		req.ReplyMarkup.InlineKeyboard = append(req.ReplyMarkup.InlineKeyboard, row)
	}
	if strings.TrimSpace(btn.BotText) != "" && len(btn.BotCallback) > 0 && len(btn.BotCallback) <= 64 {
		if req.ReplyMarkup == nil {
			req.ReplyMarkup = &inlineKeyboard{}
		}
		req.ReplyMarkup.InlineKeyboard = append(req.ReplyMarkup.InlineKeyboard, []inlineButton{{Text: btn.BotText, CallbackData: btn.BotCallback}})
	}
	err := c.send(ctx, req)
	var apiErr *APIError
	// A rejected keyboard means Telegram did not send the message. Retry only
	// that explicit rejection without markup; other failures retain the normal
	// queue policy so transport uncertainty cannot produce duplicate messages.
	if req.ReplyMarkup != nil && errors.As(err, &apiErr) && apiErr.Reason == "button_url_invalid" {
		req.ReplyMarkup = nil
		return c.send(ctx, req)
	}
	return err
}

func (c *Client) send(ctx context.Context, payload sendReq) error {
	if !c.Configured() || payload.ChatID == 0 {
		return ErrUnreachable
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := "https://api.telegram.org/bot" + c.token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		// net/http errors can include the full request URL, and Telegram embeds
		// the bot token in that URL path. Never propagate it into application
		// logs or API error chains.
		return ErrUnreachable
	}
	defer func() { _ = res.Body.Close() }()

	var out sendResp
	// A malformed body on a 2xx is still a delivery we can't confirm, so decode
	// errors fall through to the !out.OK branch below.
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode != http.StatusOK || !out.OK {
		code := out.ErrorCode
		if code == 0 {
			code = res.StatusCode
		}
		reason := ""
		description := strings.ToLower(out.Description)
		if code == 400 && strings.Contains(description, "inline keyboard button url") &&
			(strings.Contains(description, "invalid") || strings.Contains(description, "wrong http url")) {
			reason = "button_url_invalid"
		}
		return &APIError{Code: code, RetryAfter: time.Duration(out.Parameters.RetryAfter) * time.Second, Reason: reason}
	}
	return nil
}

// EscapeHTML escapes the three characters Telegram's HTML parse mode treats as
// markup. Deletion codes contain punctuation, so every interpolated value must
// go through this or the message is rejected with "can't parse entities".
func EscapeHTML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// actionRow yaroqsiz tugmalarni jimgina tashlab yuboradi: Telegram butun
// sendMessage'ni 400 bilan rad etadi, ya'ni buzuq tugma xabarning o'zini
// yo'qotardi (qarang: [Button.Valid] izohi).
func actionRow(actions []BotAction) []inlineButton {
	row := make([]inlineButton, 0, len(actions))
	for _, a := range actions {
		if strings.TrimSpace(a.Text) == "" || a.Callback == "" || len(a.Callback) > 64 {
			continue
		}
		row = append(row, inlineButton{Text: a.Text, CallbackData: a.Callback})
	}
	return row
}
