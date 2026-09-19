package notification

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/tgsend"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TelegramSender is shared with the OTP bot's stateless sendMessage client;
// this worker never polls Telegram updates or interferes with sign-in.
//
// Only the button-carrying send is listed: every job notification has a
// destination, so a plain send would be an unused second way to do the same
// thing. [tgsend.Client] still exposes SendHTML for the callers that have
// nothing to link to (deletion codes, error alerts).
type TelegramSender interface {
	Configured() bool
	SendHTMLWithButton(ctx context.Context, chatID int64, html string, btn tgsend.Button) error
}

type Telegram struct {
	WebBaseURL string
	col        *mongo.Collection
	users      *mongo.Collection
	sender     TelegramSender
	log        *slog.Logger
	wakeup     chan struct{}
}

func NewTelegram(db *mongo.Database, sender TelegramSender, log *slog.Logger) *Telegram {
	return &Telegram{
		WebBaseURL: "https://ishchibormi.uz",
		col:        db.Collection("notifications"), users: db.Collection("users"),
		sender: sender, log: log, wakeup: make(chan struct{}, 1),
	}
}

type telegramState struct {
	Status        string             `bson:"status"`
	NextAttemptAt time.Time          `bson:"nextAttemptAt,omitempty"`
	Attempts      int                `bson:"attempts"`
	LeaseID       primitive.ObjectID `bson:"leaseId,omitempty"`
}

type notificationDocument struct {
	models.Notification `bson:",inline"`
	Telegram            *telegramState `bson:"telegram,omitempty"`
}

// Explicit allowlist: relating an admin message to an elon/application does
// not turn it into a Telegram job event. Unknown future types stay in-app.
func telegramJobEvent(n models.Notification) bool {
	if !n.SentByAdminID.IsZero() {
		return false
	}
	switch n.Type {
	case "new_application", "application_submitted", "application_accepted", "application_rejected",
		"application_cancelled", "elon_updated", "job_completed", "job_completed_request",
		// Ish signali (internal/jobalert): foydalanuvchining O'ZI yoqqan
		// obuna, ya'ni bu so'ralmagan xabar emas.
		"job_nearby":
		return true
	default:
		return false
	}
}

func (s *Service) document(n models.Notification) notificationDocument {
	doc := notificationDocument{Notification: n}
	if s.Telegram != nil && s.Telegram.sender != nil && s.Telegram.sender.Configured() && telegramJobEvent(n) {
		doc.Telegram = &telegramState{Status: "pending", NextAttemptAt: time.Now()}
	}
	return doc
}

func (t *Telegram) wake() {
	select {
	case t.wakeup <- struct{}{}:
	default:
	}
}

// Run resumes pending deliveries after a restart. Old inbox rows without an
// outbox marker are intentionally never replayed. Telegram delivery is separate
// from pushQueued (FCM), so success on one channel cannot suppress the other.
func (t *Telegram) Run(ctx context.Context) {
	if t.sender == nil || !t.sender.Configured() {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		worked, err := t.deliverNext(ctx)
		if err != nil && ctx.Err() == nil {
			t.log.Warn("telegram notification queue failed", "err", err)
		}
		if worked && err == nil {
			// Bound the send rate; per-chat Telegram limits use retry_after.
			timer := time.NewTimer(50 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-t.wakeup:
		}
	}
}

func (t *Telegram) deliverNext(parent context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	now := time.Now()
	leaseID := primitive.NewObjectID()
	var doc notificationDocument
	err := t.col.FindOneAndUpdate(ctx,
		bson.M{"telegram.status": "pending", "telegram.nextAttemptAt": bson.M{"$lte": now}},
		bson.M{
			"$set": bson.M{"telegram.nextAttemptAt": now.Add(time.Minute), "telegram.leaseId": leaseID},
			"$inc": bson.M{"telegram.attempts": 1},
		},
		options.FindOneAndUpdate().SetSort(bson.D{{Key: "telegram.nextAttemptAt", Value: 1}}).SetReturnDocument(options.After),
	).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Recheck at the delivery boundary as well as at enqueue time.
	if !telegramJobEvent(doc.Notification) {
		return true, t.finish(ctx, doc, "skipped")
	}
	var u models.User
	err = t.users.FindOne(ctx, bson.M{
		"_id": doc.UserID, "isDeleted": bson.M{"$ne": true},
		"isReviewAccount": bson.M{"$ne": true}, "telegramId": bson.M{"$gt": 0},
	}, options.FindOne().SetProjection(bson.M{"telegramId": 1})).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return true, t.finish(ctx, doc, "skipped")
	}
	if err != nil {
		return true, t.retry(ctx, doc, err)
	}
	if err = t.sender.SendHTMLWithButton(ctx, u.TelegramID,
		telegramMessage(doc.Notification), telegramButton(doc.Notification, t.WebBaseURL)); err != nil {
		return true, t.retry(ctx, doc, err)
	}
	return true, t.finish(ctx, doc, "sent")
}

func telegramLease(doc notificationDocument) bson.M {
	return bson.M{"_id": doc.ID, "telegram.leaseId": doc.Telegram.LeaseID}
}

func (t *Telegram) finish(ctx context.Context, doc notificationDocument, status string) error {
	_, err := t.col.UpdateOne(ctx, telegramLease(doc), bson.M{
		"$set":   bson.M{"telegram.status": status, "telegram.finishedAt": time.Now()},
		"$unset": bson.M{"telegram.nextAttemptAt": "", "telegram.leaseId": ""},
	})
	return err
}

func (t *Telegram) retry(ctx context.Context, doc notificationDocument, cause error) error {
	delay := 15 * time.Second
	for i := 1; i < doc.Telegram.Attempts && delay < 15*time.Minute; i++ {
		delay *= 2
	}
	if delay > 15*time.Minute {
		delay = 15 * time.Minute
	}
	var apiErr *tgsend.APIError
	permanent := false
	errorCode, reason := 0, "transport_error"
	if errors.As(cause, &apiErr) {
		errorCode, reason = apiErr.Code, apiErr.Reason
		// Bad chat/message and blocked bot cannot be fixed by retrying.
		permanent = apiErr.Code == 400 || apiErr.Code == 403
		if apiErr.RetryAfter > delay {
			delay = apiErr.RetryAfter
		}
	}
	if permanent || doc.Telegram.Attempts >= 12 {
		t.log.Warn("telegram notification delivery stopped", "notification", doc.ID.Hex(), "attempts", doc.Telegram.Attempts, "errorCode", errorCode, "reason", reason)
		return t.finish(ctx, doc, "failed")
	}
	// Never log the sender error: custom transports may include the bot token.
	t.log.Warn("telegram notification retry scheduled", "notification", doc.ID.Hex(), "attempt", doc.Telegram.Attempts, "errorCode", errorCode, "reason", reason)
	_, err := t.col.UpdateOne(ctx, telegramLease(doc), bson.M{
		"$set":   bson.M{"telegram.nextAttemptAt": time.Now().Add(delay)},
		"$unset": bson.M{"telegram.leaseId": ""},
	})
	return err
}

// telegramButton is the "open this" button under the message.
//
// The URL is an ordinary https link to the website, and that is the whole
// trick: ishchibormi.uz is a verified Android App Link
// (flutter-app/android/app/src/main/AndroidManifest.xml +
// /.well-known/assetlinks.json), so on a phone with the app installed Telegram
// hands the link to the app, and everywhere else — desktop, iOS, no app — the
// same link opens the web page. A tg:// or custom scheme could not do both, and
// Telegram would reject the button outright.
//
// Every path used here MUST also be covered by that intent-filter and by
// DeepLinkService.parse; a path covered by only one of the two either opens the
// browser when the app exists, or opens the app on a screen it cannot render.
func telegramButton(n models.Notification, webBaseURL string) tgsend.Button {
	path, label := "/notifications", "Bildirishnomalarni ko'rish"
	if rel := n.RelatedEntity; rel != nil && !rel.ID.IsZero() {
		switch rel.Type {
		case "application":
			path, label = "/applications/"+rel.ID.Hex(), "Arizani ko'rish"
		case "elon":
			path, label = "/elon/"+rel.ID.Hex(), "E'lonni ko'rish"
		}
	}
	button := tgsend.Button{Text: label, URL: strings.TrimRight(webBaseURL, "/") + path}
	if rel := n.RelatedEntity; rel != nil && !rel.ID.IsZero() {
		switch rel.Type {
		case "application":
			button.BotText, button.BotCallback = "Botda arizani ochish", "w:app:"+rel.ID.Hex()
		case "elon":
			button.BotText, button.BotCallback = "Botda e'lonni ochish", "w:job:"+rel.ID.Hex()
		}
		// Ish beruvchi qarorni xabarning o'zidan boshlashi mumkin. Tugma
		// amalni BAJARMAYDI — bot avval tasdiq so'raydi (cmd/bot: "confirm"
		// bosqichi), shuning uchun tasodifan bosish xavfsiz.
		if n.Type == "new_application" && rel.Type == "application" {
			button.Actions = []tgsend.BotAction{
				{Text: "✅ Qabul qilish", Callback: "w:accept:" + rel.ID.Hex()},
				{Text: "❌ Rad etish", Callback: "w:reject:" + rel.ID.Hex()},
			}
		}
	}
	return button
}

func telegramText(value string, maxUnits int) string {
	value = strings.TrimSpace(value)
	units := 0
	for i, r := range value {
		width := 1
		if r > 0xFFFF {
			width = 2
		}
		if units+width > maxUnits-1 {
			return value[:i] + "…"
		}
		units += width
	}
	return value
}
