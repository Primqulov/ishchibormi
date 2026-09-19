package notification

import (
	"context"
	"errors"
	"html"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/httpx"
	"github.com/ishchibormi/backend/pkg/tgsend"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type telegramCall struct {
	chatID int64
	text   string
	btn    tgsend.Button
}

type fakeTelegramSender struct {
	mu    sync.Mutex
	calls []telegramCall
	err   error
}

func (*fakeTelegramSender) Configured() bool { return true }
func (s *fakeTelegramSender) SendHTMLWithButton(_ context.Context, chatID int64, text string, btn tgsend.Button) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, telegramCall{chatID, text, btn})
	return s.err
}

type inboxPushRecorder struct{ calls int }

func (p *inboxPushRecorder) PushUser(primitive.ObjectID, string, any) { p.calls++ }

func telegramTestDB(t *testing.T) *mongo.Database {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://127.0.0.1:27017"))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("local Mongo unavailable: %v", err)
	}
	db := client.Database("ib_telegram_test_" + primitive.NewObjectID().Hex())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = db.Drop(ctx)
		_ = client.Disconnect(ctx)
	})
	return db
}

func telegramTestService(t *testing.T) (*Service, *Telegram, *fakeTelegramSender, primitive.ObjectID) {
	t.Helper()
	db := telegramTestDB(t)
	sender := &fakeTelegramSender{}
	tg := NewTelegram(db, sender, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc := New(db)
	svc.AttachTelegram(tg)
	uid := primitive.NewObjectID()
	if _, err := svc.Users.InsertOne(context.Background(), models.User{ID: uid, TelegramID: 987654321}); err != nil {
		t.Fatal(err)
	}
	return svc, tg, sender, uid
}

func deliverTelegram(t *testing.T, tg *Telegram, wantWork bool) {
	t.Helper()
	worked, err := tg.deliverNext(context.Background())
	if err != nil || worked != wantWork {
		t.Fatalf("deliverNext = %v, %v; want worked=%v", worked, err, wantWork)
	}
}

func telegramStored(t *testing.T, svc *Service, id primitive.ObjectID) notificationDocument {
	t.Helper()
	var doc notificationDocument
	if err := svc.Col.FindOne(context.Background(), bson.M{"_id": id}).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestTelegramDeliversJobEventsAndLeavesAdminMessagesInApp(t *testing.T) {
	svc, tg, sender, uid := telegramTestService(t)
	pusher := &inboxPushRecorder{}
	svc.AttachPusher(pusher)
	ctx := context.Background()
	allowed := []struct{ typ, title string }{
		{"new_application", "Yangi ariza"},
		{"application_submitted", "Arizangiz yuborildi"},
		{"application_accepted", "Arizangiz qabul qilindi"},
		{"application_rejected", "Arizangiz rad etildi"},
		{"application_rejected", "Joy to'ldi"},
		{"application_cancelled", "Ariza bekor qilindi"},
		{"elon_updated", "Ish shartlari o'zgardi"},
		{"job_completed", "Ish yakunlandi"},
		{"job_completed_request", "Tasdiqlash so'rovi"},
	}
	for _, event := range allowed {
		// Some cancellations summarize several applications and have no relation.
		svc.Push(ctx, uid, event.typ, event.title, "Qurilish", nil)
		deliverTelegram(t, tg, true)
		last := sender.calls[len(sender.calls)-1]
		if last.chatID != 987654321 || !strings.Contains(last.text, event.title) {
			t.Fatalf("wrong recipient/content: %+v", last)
		}
	}
	related := &models.RelatedEntity{Type: "application", ID: primitive.NewObjectID()}
	for _, typ := range []string{"system", "security", "broadcast", "message", "payment", "unknown", "new_application_extra"} {
		// Even a job-related entity or title cannot bypass the type allowlist.
		svc.Push(ctx, uid, typ, "Yangi ariza", "Admin xabari", related)
	}
	svc.PushFromAdmin(ctx, uid, primitive.NewObjectID(), "Ariza", "Shaxsiy admin xabari")
	forged := models.Notification{
		ID: primitive.NewObjectID(), UserID: uid, Type: "new_application",
		SentByAdminID: primitive.NewObjectID(), RelatedEntity: related,
	}
	if err := svc.PushOnce(ctx, forged); err != nil {
		t.Fatal(err)
	}
	deliverTelegram(t, tg, false)
	if len(sender.calls) != len(allowed) {
		t.Fatalf("admin message reached Telegram: %d sends", len(sender.calls))
	}
	count, err := svc.Col.CountDocuments(ctx, bson.M{})
	if err != nil || count != 18 || pusher.calls != 18 {
		t.Fatalf("in-app/mobile delivery changed: inbox=%d pushes=%d err=%v", count, pusher.calls, err)
	}
}

func TestTelegramQueueSurvivesRestartAndPushOnceDoesNotResend(t *testing.T) {
	svc, tg, sender, uid := telegramTestService(t)
	pusher := &inboxPushRecorder{}
	svc.AttachPusher(pusher)
	ctx := context.Background()
	n := models.Notification{ID: primitive.NewObjectID(), UserID: uid, Type: "elon_updated", Title: "Ish shartlari o'zgardi"}
	if err := svc.PushOnce(ctx, n); err != nil {
		t.Fatal(err)
	}
	// The process stopped after inbox persistence. Another worker uses only DB state.
	restarted := NewTelegram(tg.col.Database(), sender, tg.log)
	deliverTelegram(t, restarted, true)
	if err := svc.PushOnce(ctx, n); err != nil {
		t.Fatal(err)
	}
	deliverTelegram(t, restarted, false)
	if len(sender.calls) != 1 || telegramStored(t, svc, n.ID).Telegram.Status != "sent" {
		t.Fatal("durable event was lost or sent twice")
	}
	if pusher.calls != 1 {
		t.Fatalf("FCM and Telegram delivery were not independent: %d mobile pushes", pusher.calls)
	}
	// Pre-feature notifications and already queued FCM must not be replayed.
	legacy := models.Notification{ID: primitive.NewObjectID(), UserID: uid, Type: "new_application"}
	if _, err := svc.Col.InsertOne(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if err := svc.PushOnce(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	deliverTelegram(t, restarted, false)
	if telegramStored(t, svc, legacy.ID).Telegram != nil {
		t.Fatal("legacy inbox was added to the Telegram queue")
	}
}

func TestTelegramFiltersRecipientsAndReviewActors(t *testing.T) {
	svc, tg, sender, uid := telegramTestService(t)
	ctx := context.Background()
	for _, fields := range []bson.M{
		{"isDeleted": true, "telegramId": int64(123)},
		{"isReviewAccount": true, "telegramId": int64(123)},
		{"telegramId": int64(0)},
		{"telegramId": int64(-123)},
		{},
	} {
		id := primitive.NewObjectID()
		fields["_id"] = id
		if _, err := svc.Users.InsertOne(ctx, fields); err != nil {
			t.Fatal(err)
		}
		svc.Push(ctx, id, "new_application", "Ariza", "", nil)
		deliverTelegram(t, tg, true)
	}
	svc.Push(ctx, primitive.NewObjectID(), "new_application", "Missing user", "", nil)
	deliverTelegram(t, tg, true)
	reviewCtx := context.WithValue(ctx, httpx.CtxReviewActor, true)
	svc.Push(reviewCtx, uid, "new_application", "Demo ariza", "", nil)
	if err := svc.PushOnce(reviewCtx, models.Notification{ID: primitive.NewObjectID(), UserID: uid, Type: "application_cancelled"}); err != nil {
		t.Fatal(err)
	}
	deliverTelegram(t, tg, false)
	if len(sender.calls) != 0 {
		t.Fatalf("ineligible recipients were sent %d messages", len(sender.calls))
	}
}

func TestTelegramRetryAfterAndBlockedBot(t *testing.T) {
	svc, tg, sender, uid := telegramTestService(t)
	ctx := context.Background()
	sender.err = &tgsend.APIError{Code: 429, RetryAfter: 2 * time.Minute}
	n := models.Notification{ID: primitive.NewObjectID(), UserID: uid, Type: "application_accepted"}
	if err := svc.PushOnce(ctx, n); err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	deliverTelegram(t, tg, true)
	doc := telegramStored(t, svc, n.ID)
	if doc.Telegram.Status != "pending" || doc.Telegram.NextAttemptAt.Before(before.Add(2*time.Minute-time.Second)) {
		t.Fatalf("retry_after ignored: %+v", doc.Telegram)
	}
	deliverTelegram(t, tg, false)
	// Simulate time passing without making the test wait.
	if _, err := svc.Col.UpdateOne(ctx, bson.M{"_id": n.ID}, bson.M{"$set": bson.M{"telegram.nextAttemptAt": time.Now().Add(-time.Second)}}); err != nil {
		t.Fatal(err)
	}
	sender.err = nil
	deliverTelegram(t, tg, true)
	if got := telegramStored(t, svc, n.ID).Telegram; got.Status != "sent" || got.Attempts != 2 {
		t.Fatalf("retry did not succeed: %+v", got)
	}
	sender.err = &tgsend.APIError{Code: 403}
	n.ID = primitive.NewObjectID()
	if err := svc.PushOnce(ctx, n); err != nil {
		t.Fatal(err)
	}
	deliverTelegram(t, tg, true)
	if telegramStored(t, svc, n.ID).Telegram.Status != "failed" {
		t.Fatal("blocked bot kept retrying")
	}
	deliverTelegram(t, tg, false)
}

func TestTelegramConcurrentWorkersClaimOnceAndRecoverExpiredLease(t *testing.T) {
	svc, tg, sender, uid := telegramTestService(t)
	ctx := context.Background()
	svc.Push(ctx, uid, "new_application", "Yangi ariza", "", nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := tg.deliverNext(ctx); err != nil {
				t.Errorf("concurrent delivery: %v", err)
			}
		}()
	}
	wg.Wait()
	if len(sender.calls) != 1 {
		t.Fatalf("one event sent %d times", len(sender.calls))
	}
	n := models.Notification{ID: primitive.NewObjectID(), UserID: uid, Type: "application_cancelled"}
	if err := svc.PushOnce(ctx, n); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Col.UpdateOne(ctx, bson.M{"_id": n.ID}, bson.M{"$set": bson.M{
		"telegram.leaseId": primitive.NewObjectID(), "telegram.nextAttemptAt": time.Now().Add(-time.Minute),
	}}); err != nil {
		t.Fatal(err)
	}
	deliverTelegram(t, tg, true)
	if len(sender.calls) != 2 {
		t.Fatal("expired process lease was not recovered")
	}
}

func TestTelegramRechecksPolicyAndRecipientAtSendTime(t *testing.T) {
	svc, tg, sender, uid := telegramTestService(t)
	ctx := context.Background()
	n := models.Notification{ID: primitive.NewObjectID(), UserID: uid, Type: "new_application"}
	if err := svc.PushOnce(ctx, n); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Col.UpdateOne(ctx, bson.M{"_id": n.ID}, bson.M{"$set": bson.M{"sentByAdminId": primitive.NewObjectID()}}); err != nil {
		t.Fatal(err)
	}
	deliverTelegram(t, tg, true)
	if telegramStored(t, svc, n.ID).Telegram.Status != "skipped" {
		t.Fatal("admin-tagged event bypassed delivery filter")
	}
	n.ID = primitive.NewObjectID()
	if err := svc.PushOnce(ctx, n); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Users.UpdateOne(ctx, bson.M{"_id": uid}, bson.M{"$set": bson.M{"isDeleted": true}}); err != nil {
		t.Fatal(err)
	}
	deliverTelegram(t, tg, true)
	if len(sender.calls) != 0 {
		t.Fatal("event sent after the user deleted their account")
	}
}

func TestTelegramTransientFailureStopsAtAttemptLimit(t *testing.T) {
	svc, tg, sender, uid := telegramTestService(t)
	ctx := context.Background()
	sender.err = errors.New("network unavailable")
	n := models.Notification{ID: primitive.NewObjectID(), UserID: uid, Type: "job_completed"}
	if err := svc.PushOnce(ctx, n); err != nil {
		t.Fatal(err)
	}
	deliverTelegram(t, tg, true)
	if telegramStored(t, svc, n.ID).Telegram.Status != "pending" {
		t.Fatal("transient failure was dropped")
	}
	if _, err := svc.Col.UpdateOne(ctx, bson.M{"_id": n.ID}, bson.M{"$set": bson.M{
		"telegram.attempts": 11, "telegram.nextAttemptAt": time.Now().Add(-time.Second),
	}}); err != nil {
		t.Fatal(err)
	}
	deliverTelegram(t, tg, true)
	if telegramStored(t, svc, n.ID).Telegram.Status != "failed" {
		t.Fatal("delivery kept retrying after the attempt limit")
	}
}

func TestTelegramDisabledDoesNotBuildBacklog(t *testing.T) {
	svc := &Service{Telegram: &Telegram{sender: tgsend.New("")}}
	if svc.document(models.Notification{Type: "new_application"}).Telegram != nil {
		t.Fatal("unconfigured bot queued messages")
	}
}

func TestTelegramMessageEscapesContentAndFitsLimit(t *testing.T) {
	n := models.Notification{Title: `Ish <b> & "sarlavha"`, Body: `<a href="https://bad.example">Ariza</a> & sabab`}
	msg := telegramMessage(n)
	if strings.Contains(msg, `<a href="https://bad.example">`) || !strings.Contains(msg, "&lt;b&gt;") || !strings.Contains(msg, "&amp;") {
		t.Fatalf("unescaped user content: %s", msg)
	}
	// The destination now lives in the inline button, so a body that carries an
	// anchor must not be able to plant a second, clickable one in the text.
	if strings.Contains(msg, "<a ") {
		t.Fatalf("message body produced markup of its own: %s", msg)
	}
	if strings.Count(msg, "<b>") != 1 || strings.Count(msg, "</b>") != 1 {
		t.Fatalf("unbalanced/extra markup: %s", msg)
	}
	for _, body := range []string{strings.Repeat("😀", 4500), strings.Repeat("<&>", 4500)} {
		msg := telegramMessage(models.Notification{Title: strings.Repeat("😀", 300), Body: body})
		plain := html.UnescapeString(strings.ReplaceAll(strings.ReplaceAll(msg, "<b>", ""), "</b>", ""))
		if len(utf16.Encode([]rune(plain))) > 4096 || !strings.Contains(plain, "…") {
			t.Fatal("long notification exceeds Telegram's parsed text limit")
		}
	}
}

func TestTelegramButtonPointsAtRelatedDetails(t *testing.T) {
	id := primitive.NewObjectID()
	for _, tc := range []struct {
		name, base, path, label string
		related                 *models.RelatedEntity
	}{
		{"application", "https://ishchibormi.uz", "/applications/" + id.Hex(), "Arizani ko'rish", &models.RelatedEntity{Type: "application", ID: id}},
		{"local application", "http://127.0.0.1:3000/", "/applications/" + id.Hex(), "Arizani ko'rish", &models.RelatedEntity{Type: "application", ID: id}},
		{"listing", "https://ishchibormi.uz", "/elon/" + id.Hex(), "E'lonni ko'rish", &models.RelatedEntity{Type: "elon", ID: id}},
		{"summary", "https://ishchibormi.uz", "/notifications", "Bildirishnomalarni ko'rish", nil},
		{"missing id", "https://ishchibormi.uz", "/notifications", "Bildirishnomalarni ko'rish", &models.RelatedEntity{Type: "application"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			btn := telegramButton(models.Notification{Title: "Yangi ariza", RelatedEntity: tc.related}, tc.base)
			want := strings.TrimRight(tc.base, "/") + tc.path
			if btn.URL != want || btn.Text != tc.label {
				t.Fatalf("wrong button %+v, want %q / %q", btn, tc.label, want)
			}
			if tc.related != nil && !tc.related.ID.IsZero() {
				action := "job"
				if tc.related.Type == "application" {
					action = "app"
				}
				if btn.BotCallback != "w:"+action+":"+id.Hex() || btn.BotText == "" {
					t.Fatal("native bot destination missing")
				}
			}
			// A button Telegram refuses takes the whole message down with it.
			if !btn.Valid() {
				t.Fatalf("telegram would reject button %+v", btn)
			}
		})
	}
}

// The button is what makes a job notification actionable, so a delivery that
// reaches Telegram without one is a regression even though it "succeeded".
func TestTelegramDeliverySendsButton(t *testing.T) {
	svc, tg, sender, uid := telegramTestService(t)
	elonID := primitive.NewObjectID()

	svc.Push(context.Background(), uid, "new_application", "Yangi ariza",
		"E'loningizga ariza tushdi", &models.RelatedEntity{Type: "elon", ID: elonID})
	deliverTelegram(t, tg, true)

	call := sender.calls[0]
	if call.btn.URL != "https://ishchibormi.uz/elon/"+elonID.Hex() || call.btn.Text != "E'lonni ko'rish" {
		t.Fatalf("wrong button: %+v", call.btn)
	}
	// The destination belongs to the button now; a URL left in the text would
	// also bring back the link preview the button send suppresses.
	if strings.Contains(call.text, "http") {
		t.Fatalf("link left in the message text: %s", call.text)
	}
}
