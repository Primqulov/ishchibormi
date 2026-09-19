// Package channelpost publishes new listings from all clients to one configured channel.
// It never enumerates users or sends announcements to private chats.
package channelpost

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/elonquery"
	"github.com/ishchibormi/backend/pkg/tgsend"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Sender interface {
	Configured() bool
	ResolveChannel(context.Context, string) (tgsend.ChannelInfo, error)
	SendChannelHTML(context.Context, int64, string, tgsend.Button) (int64, error)
}
type Publisher struct {
	col                    *mongo.Collection
	sender                 Sender
	reference, botUsername string
	log                    *slog.Logger
}

func New(db *mongo.Database, sender Sender, reference, botUsername string, log *slog.Logger) (*Publisher, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, nil
	}
	if !tgsend.ValidChannelReference(reference) || !tgsend.ValidBotUsername(botUsername) {
		return nil, errors.New("TELEGRAM_JOBS_CHANNEL_ID and TELEGRAM_BOT_USERNAME must identify a channel and a bot")
	}
	if sender == nil || !sender.Configured() {
		return nil, errors.New("channel publication requires TELEGRAM_BOT_TOKEN")
	}
	return &Publisher{col: db.Collection("elons"), sender: sender, reference: reference, botUsername: botUsername, log: log}, nil
}
func (p *Publisher) Pending(now time.Time) *models.ChannelPublication {
	if p == nil {
		return nil
	}
	return &models.ChannelPublication{Reference: p.reference, BotUsername: p.botUsername, Status: "pending", NextAttemptAt: now}
}
func (p *Publisher) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if _, err := p.deliverNext(ctx); err != nil && ctx.Err() == nil {
			p.log.Warn("channel publication queue failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Publisher) deliverNext(parent context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	now := time.Now()
	// Telegram sendMessage has no idempotency key. An abandoned in-flight send
	// may already have arrived, so never blindly resend it after a restart.
	_, err := p.col.UpdateMany(ctx, bson.M{"telegramChannel.reference": p.reference, "telegramChannel.status": "sending", "telegramChannel.leaseUntil": bson.M{"$lte": now}}, bson.M{"$set": bson.M{"telegramChannel.status": "uncertain", "telegramChannel.reason": "interrupted_delivery", "telegramChannel.finishedAt": now}})
	if err != nil {
		return false, err
	}
	var claimed models.Elon
	lease := primitive.NewObjectID()
	err = p.col.FindOneAndUpdate(ctx, bson.M{"telegramChannel.reference": p.reference, "telegramChannel.status": "pending", "telegramChannel.nextAttemptAt": bson.M{"$lte": now}}, bson.M{
		"$set": bson.M{"telegramChannel.status": "sending", "telegramChannel.leaseId": lease, "telegramChannel.leaseUntil": now.Add(2 * time.Minute)}, "$inc": bson.M{"telegramChannel.attempts": 1},
	}, options.FindOneAndUpdate().SetSort(bson.D{{Key: "telegramChannel.nextAttemptAt", Value: 1}}).SetReturnDocument(options.After)).Decode(&claimed)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	filter := elonquery.ActiveFilter(now, false)
	filter["_id"] = claimed.ID
	var live models.Elon
	err = p.col.FindOne(ctx, filter).Decode(&live)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return true, p.finish(ctx, claimed, "skipped", "listing_unavailable", 0, 0)
	}
	if err != nil {
		return true, p.retry(ctx, claimed, time.Minute, "listing_read_failed")
	}
	if live.WorkersNeeded <= live.AcceptedCount {
		return true, p.finish(ctx, claimed, "skipped", "no_vacancies", 0, 0)
	}
	info, err := p.sender.ResolveChannel(ctx, p.reference)
	if err != nil {
		// Resolution only reads Telegram state; it is safe to retry permissions
		// or connectivity before attempting to send any channel message.
		return true, p.retry(ctx, claimed, 5*time.Minute, "channel_unavailable")
	}
	if !strings.EqualFold(info.BotUsername, claimed.TelegramChannel.BotUsername) {
		return true, p.finish(ctx, claimed, "failed", "bot_username_mismatch", 0, 0)
	}
	text, button := Message(live, info.BotUsername)
	id, err := p.sender.SendChannelHTML(ctx, info.ID, text, button)
	if err == nil && id > 0 {
		return true, p.finish(ctx, claimed, "sent", "", info.ID, id)
	}
	var apiErr *tgsend.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Code == 429 {
			delay := apiErr.RetryAfter
			if delay < time.Second {
				delay = time.Second
			}
			return true, p.retry(ctx, claimed, delay, "telegram_rate_limit")
		}
		if apiErr.Code >= 400 && apiErr.Code < 500 && apiErr.Code != 408 {
			return true, p.finish(ctx, claimed, "failed", "telegram_rejected", info.ID, 0)
		}
	}
	// A timeout, 5xx, invalid response, or interrupted connection is ambiguous.
	// Stop here to avoid duplicate posts; retain the listing ID for inspection.
	return true, p.finish(ctx, claimed, "uncertain", "delivery_not_confirmed", info.ID, 0)
}
func (p *Publisher) lease(e models.Elon) bson.M {
	return bson.M{"_id": e.ID, "telegramChannel.status": "sending", "telegramChannel.leaseId": e.TelegramChannel.LeaseID}
}
func (p *Publisher) finish(ctx context.Context, e models.Elon, status, reason string, chatID, messageID int64) error {
	_, err := p.col.UpdateOne(ctx, p.lease(e), bson.M{"$set": bson.M{"telegramChannel.status": status, "telegramChannel.reason": reason, "telegramChannel.chatId": chatID, "telegramChannel.messageId": messageID, "telegramChannel.finishedAt": time.Now()}, "$unset": bson.M{"telegramChannel.nextAttemptAt": "", "telegramChannel.leaseUntil": "", "telegramChannel.leaseId": ""}})
	if status == "failed" || status == "uncertain" {
		p.log.Warn("channel publication needs attention", "listing", e.ID.Hex(), "status", status, "reason", reason)
	}
	return err
}
func (p *Publisher) retry(ctx context.Context, e models.Elon, delay time.Duration, reason string) error {
	if e.TelegramChannel.Attempts >= 24 {
		return p.finish(ctx, e, "failed", reason, 0, 0)
	}
	_, err := p.col.UpdateOne(ctx, p.lease(e), bson.M{"$set": bson.M{"telegramChannel.status": "pending", "telegramChannel.reason": reason, "telegramChannel.nextAttemptAt": time.Now().Add(delay)}, "$unset": bson.M{"telegramChannel.leaseUntil": "", "telegramChannel.leaseId": ""}})
	return err
}

func Message(e models.Elon, username string) (string, tgsend.Button) {
	title := []rune(strings.Join(strings.Fields(e.Title), " "))
	if len(title) > 160 {
		title = title[:160]
	}
	pay := "Kelishiladi"
	if e.PricingType != "negotiable" && e.PerWorkerAmount > 0 {
		pay = money(e.PerWorkerAmount) + " so'm / kishi"
	}
	text := fmt.Sprintf("💼 <b>%s</b>\n\n👥 Kerak: <b>%d kishi</b>\n💰 Ish haqi: <b>%s</b>", tgsend.EscapeHTML(string(title)), e.WorkersNeeded, pay)
	date, clock := e.StartDate, e.WorkTimeFrom
	if len(date) >= 16 && clock == "" {
		clock = date[11:16]
	}
	if len(date) >= 10 {
		date = date[:10]
	}
	if parsed, err := time.Parse("2006-01-02", date); err == nil {
		text += "\n📅 " + parsed.Format("02.01.2006")
		if clock != "" {
			text += " · " + tgsend.EscapeHTML(clock) + " (Toshkent)"
		}
	}
	address := strings.Trim(strings.TrimSpace(e.Region)+", "+strings.TrimSpace(e.District), ", ")
	r := []rune(address)
	if len(r) > 180 {
		r = r[:180]
	}
	if len(r) > 0 {
		text += "\n📍 " + tgsend.EscapeHTML(string(r))
	}
	text += "\n\nIsh haqida batafsil ma'lumot olish uchun pastdagi tugmani bosing."
	return text, tgsend.Button{Text: "Ish haqida batafsil", URL: "https://t.me/" + username + "?start=job_" + e.ID.Hex()}
}
func money(value int64) string {
	raw := fmt.Sprint(value)
	for i := len(raw) - 3; i > 0; i -= 3 {
		raw = raw[:i] + " " + raw[i:]
	}
	return raw
}
