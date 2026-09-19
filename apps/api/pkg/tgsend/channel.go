package tgsend

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var channelReference = regexp.MustCompile(`^(?:-100[0-9]{5,}|@[A-Za-z][A-Za-z0-9_]{4,31})$`)
var botUsername = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)

func ValidChannelReference(ref string) bool { return channelReference.MatchString(ref) }
func ValidBotUsername(name string) bool {
	return botUsername.MatchString(name) && strings.HasSuffix(strings.ToLower(name), "bot")
}

type ChannelInfo struct {
	ID          int64
	BotUsername string
}

// ResolveChannel is read-only. Never publish to a group or private chat even
// if a valid Telegram chat ID was mistakenly entered in channel configuration.
func (c *Client) ResolveChannel(ctx context.Context, ref string) (ChannelInfo, error) {
	if !ValidChannelReference(ref) {
		return ChannelInfo{}, &APIError{Code: 400, Reason: "invalid_channel"}
	}
	var me struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		IsBot    bool   `json:"is_bot"`
	}
	if err := c.channelCall(ctx, "getMe", map[string]any{}, &me); err != nil {
		return ChannelInfo{}, err
	}
	var chat struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	}
	if err := c.channelCall(ctx, "getChat", map[string]any{"chat_id": ref}, &chat); err != nil {
		return ChannelInfo{}, err
	}
	if chat.Type != "channel" || chat.ID >= 0 || !me.IsBot || !ValidBotUsername(me.Username) {
		return ChannelInfo{}, &APIError{Code: 400, Reason: "not_a_channel"}
	}
	var member struct {
		Status  string `json:"status"`
		CanPost bool   `json:"can_post_messages"`
	}
	if err := c.channelCall(ctx, "getChatMember", map[string]any{"chat_id": chat.ID, "user_id": me.ID}, &member); err != nil {
		return ChannelInfo{}, err
	}
	if member.Status != "administrator" || !member.CanPost {
		return ChannelInfo{}, &APIError{Code: 403, Reason: "channel_post_permission"}
	}
	return ChannelInfo{ID: chat.ID, BotUsername: me.Username}, nil
}

// A new channel post, not a forwarded private message. Silent delivery avoids
// a notification sound for every listing; it does not suppress the channel post.
func (c *Client) SendChannelHTML(ctx context.Context, channelID int64, text string, button Button) (int64, error) {
	if channelID >= 0 || !button.Valid() {
		return 0, &APIError{Code: 400, Reason: "invalid_channel_post"}
	}
	payload := struct {
		ChatID    int64          `json:"chat_id"`
		Text      string         `json:"text"`
		ParseMode string         `json:"parse_mode"`
		Silent    bool           `json:"disable_notification"`
		NoPreview bool           `json:"disable_web_page_preview"`
		Markup    inlineKeyboard `json:"reply_markup"`
	}{channelID, text, "HTML", true, true, inlineKeyboard{InlineKeyboard: [][]inlineButton{{{Text: button.Text, URL: button.URL}}}}}
	var result struct {
		MessageID int64 `json:"message_id"`
	}
	err := c.channelCall(ctx, "sendMessage", payload, &result)
	if err == nil && result.MessageID <= 0 {
		err = ErrUnreachable
	}
	return result.MessageID, err
}

func (c *Client) channelCall(ctx context.Context, method string, payload, result any) error {
	if !c.Configured() {
		return ErrUnreachable
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ErrUnreachable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+c.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return ErrUnreachable
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return ErrUnreachable
	}
	defer res.Body.Close()
	var out struct {
		OK         bool            `json:"ok"`
		Result     json.RawMessage `json:"result"`
		Code       int             `json:"error_code"`
		Parameters struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out) != nil {
		return ErrUnreachable
	}
	if !out.OK || res.StatusCode != http.StatusOK {
		code := out.Code
		if code == 0 {
			code = res.StatusCode
		}
		if code < 400 {
			return ErrUnreachable
		}
		return &APIError{Code: code, Reason: "channel_request_rejected", RetryAfter: time.Duration(out.Parameters.RetryAfter) * time.Second}
	}
	if json.Unmarshal(out.Result, result) != nil {
		return ErrUnreachable
	}
	return nil
}
