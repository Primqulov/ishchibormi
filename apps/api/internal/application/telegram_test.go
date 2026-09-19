package application

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/internal/notification"
	"github.com/ishchibormi/backend/pkg/httpx"
	"github.com/ishchibormi/backend/pkg/tgsend"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type jobTelegramMessage struct {
	chatID int64
	text   string
}

type jobTelegramSender struct{ messages chan jobTelegramMessage }

func (*jobTelegramSender) Configured() bool { return true }
func (s *jobTelegramSender) SendHTMLWithButton(_ context.Context, chatID int64, text string, _ tgsend.Button) error {
	s.messages <- jobTelegramMessage{chatID, text}
	return nil
}

func jobAction(t *testing.T, handler http.HandlerFunc, id, actor primitive.ObjectID, body string, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(body))
	route := chi.NewRouteContext()
	route.URLParams.Add("id", id.Hex())
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, route)
	ctx = context.WithValue(ctx, httpx.CtxUserID, actor.Hex())
	w := httptest.NewRecorder()
	handler(w, r.WithContext(ctx))
	if w.Code != wantStatus {
		t.Fatalf("action returned %d: %s", w.Code, w.Body.String())
	}
	return w
}

func expectJobTelegrams(t *testing.T, sender *jobTelegramSender, expected ...jobTelegramMessage) {
	t.Helper()
	for len(expected) > 0 {
		select {
		case got := <-sender.messages:
			found := -1
			for i, want := range expected {
				if got.chatID == want.chatID && strings.Contains(got.text, "<b>"+want.text+"</b>") {
					found = i
					break
				}
			}
			if found < 0 {
				t.Fatalf("unexpected Telegram recipient/message: %+v; want %+v", got, expected)
			}
			expected = append(expected[:found], expected[found+1:]...)
		case <-time.After(5 * time.Second):
			t.Fatalf("missing Telegram messages: %+v", expected)
		}
	}
}

// Full HTTP-handler -> durable inbox -> running worker -> fake Telegram flow.
// No real bot token or external Telegram connection is used.
func TestApplicationLifecycleSendsTelegramToBothRoles(t *testing.T) {
	h, e, a := completionTestHandler(t)
	ctx := context.Background()
	if _, err := h.Elons.UpdateOne(ctx, bson.M{"_id": e.ID}, bson.M{"$set": bson.M{
		"status": "recruiting", "acceptedCount": 0, "title": "G'isht terish",
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Apps.UpdateOne(ctx, bson.M{"_id": a.ID}, bson.M{"$set": bson.M{
		"status": "rejected", "employerConfirmedDone": false,
	}}); err != nil {
		t.Fatal(err)
	}
	for id, chatID := range map[primitive.ObjectID]int64{a.EmployerID: 101, a.WorkerID: 202} {
		if _, err := h.Users.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"telegramId": chatID}}); err != nil {
			t.Fatal(err)
		}
	}
	other := models.User{ID: primitive.NewObjectID(), TelegramID: 303}
	if _, err := h.Users.InsertOne(ctx, other); err != nil {
		t.Fatal(err)
	}
	sender := &jobTelegramSender{messages: make(chan jobTelegramMessage, 32)}
	tg := notification.NewTelegram(h.Apps.Database(), sender, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.Notify.AttachTelegram(tg)
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); tg.Run(workerCtx) }()
	defer func() { stop(); <-done }()

	// Reapplying and a first application each notify owner + applicant.
	jobAction(t, h.Apply, e.ID, a.WorkerID, `{}`, 201)
	expectJobTelegrams(t, sender, jobTelegramMessage{101, "Yangi ariza"}, jobTelegramMessage{202, "Arizangiz yuborildi"})
	w := jobAction(t, h.Apply, e.ID, other.ID, `{}`, 201)
	var otherApp models.Application
	if err := json.Unmarshal(w.Body.Bytes(), &otherApp); err != nil {
		t.Fatal(err)
	}
	expectJobTelegrams(t, sender, jobTelegramMessage{101, "Yangi ariza"}, jobTelegramMessage{303, "Arizangiz yuborildi"})
	jobAction(t, h.Apply, e.ID, other.ID, `{}`, 409)
	if count, err := h.Notify.Col.CountDocuments(ctx, bson.M{}); err != nil || count != 4 {
		t.Fatalf("duplicate application created extra notifications: %d, %v", count, err)
	}

	// Taking the final slot accepts one worker and tells the other it is full.
	jobAction(t, h.Accept, a.ID, a.EmployerID, `{}`, 200)
	expectJobTelegrams(t, sender, jobTelegramMessage{202, "Arizangiz qabul qilindi"}, jobTelegramMessage{303, "Joy to'ldi"})
	jobAction(t, h.Cancel, a.ID, a.WorkerID, `{"reason":"Bora olmayman"}`, 200)
	expectJobTelegrams(t, sender, jobTelegramMessage{101, "Ariza bekor qilindi"})
	jobAction(t, h.Apply, e.ID, a.WorkerID, `{}`, 201)
	expectJobTelegrams(t, sender, jobTelegramMessage{101, "Yangi ariza"}, jobTelegramMessage{202, "Arizangiz yuborildi"})
	jobAction(t, h.Reject, a.ID, a.EmployerID, `{}`, 200)
	expectJobTelegrams(t, sender, jobTelegramMessage{202, "Arizangiz rad etildi"})

	jobAction(t, h.Apply, e.ID, other.ID, `{}`, 201)
	expectJobTelegrams(t, sender, jobTelegramMessage{101, "Yangi ariza"}, jobTelegramMessage{303, "Arizangiz yuborildi"})
	jobAction(t, h.Accept, otherApp.ID, a.EmployerID, `{}`, 200)
	expectJobTelegrams(t, sender, jobTelegramMessage{303, "Arizangiz qabul qilindi"})
	jobAction(t, h.ConfirmDone, otherApp.ID, a.EmployerID, `{}`, 200)
	expectJobTelegrams(t, sender, jobTelegramMessage{303, "Tasdiqlash so'rovi"})
	jobAction(t, h.ConfirmDone, otherApp.ID, other.ID, `{}`, 200)
	expectJobTelegrams(t, sender, jobTelegramMessage{101, "Ish yakunlandi"}, jobTelegramMessage{303, "Ish yakunlandi"})
}

func TestAcceptedJobTelegramKeepsDecisionDetailsForWorker(t *testing.T) {
	for _, listingPhone := range []string{"+998909876543", ""} {
		t.Run("listing_phone_"+listingPhone, func(t *testing.T) {
			h, e, a := completionTestHandler(t)
			ctx := context.Background()
			if _, err := h.Elons.UpdateOne(ctx, bson.M{"_id": e.ID}, bson.M{"$set": bson.M{
				"status": "recruiting", "acceptedCount": 0, "title": "G'isht terish", "ownerName": "Ali Valiyev",
				"startDate": "2026-09-17", "workTimeFrom": "09:00", "workTimeTo": "18:00", "contactPhone": listingPhone,
				"region": "Toshkent shahri", "district": "Chilonzor", "locationText": "Bunyodkor ko'chasi, 10-uy", "lat": 41.311081, "lng": 69.240562,
			}}); err != nil {
				t.Fatal(err)
			}
			if _, err := h.Apps.UpdateOne(ctx, bson.M{"_id": a.ID}, bson.M{"$set": bson.M{"status": "pending"}}); err != nil {
				t.Fatal(err)
			}
			if _, err := h.Users.UpdateOne(ctx, bson.M{"_id": a.WorkerID}, bson.M{"$set": bson.M{"telegramId": int64(202)}}); err != nil {
				t.Fatal(err)
			}
			if _, err := h.Users.UpdateOne(ctx, bson.M{"_id": a.EmployerID}, bson.M{"$set": bson.M{"phone": "+998901234567"}}); err != nil {
				t.Fatal(err)
			}
			sender := &jobTelegramSender{messages: make(chan jobTelegramMessage, 4)}
			tg := notification.NewTelegram(h.Apps.Database(), sender, slog.New(slog.NewTextHandler(io.Discard, nil)))
			h.Notify.AttachTelegram(tg)
			jobAction(t, h.Accept, a.ID, a.EmployerID, `{}`, 200)
			jobAction(t, h.Accept, a.ID, a.EmployerID, `{}`, 400)
			var n models.Notification
			if err := h.Notify.Col.FindOne(ctx, bson.M{"type": "application_accepted"}).Decode(&n); err != nil {
				t.Fatal(err)
			}
			wantPhone := listingPhone
			if wantPhone == "" {
				wantPhone = "+998901234567"
			}
			if n.UserID != a.WorkerID || n.AcceptedJob == nil || n.AcceptedJob.ContactPhone != wantPhone || n.AcceptedJob.Work.StartDate != "2026-09-17" {
				t.Fatalf("wrong acceptance snapshot: %+v", n)
			}
			data, err := json.Marshal(n)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), wantPhone) || strings.Contains(string(data), "Bunyodkor") {
				t.Fatal("private Telegram details leaked into inbox JSON")
			}
			// Change the listing before starting the worker: the queued acceptance
			// must still describe the decision, not this later revision.
			if _, err := h.Elons.UpdateOne(ctx, bson.M{"_id": e.ID}, bson.M{"$set": bson.M{"title": "Changed later", "contactPhone": "+998000000000", "workTimeFrom": "12:00"}}); err != nil {
				t.Fatal(err)
			}
			workerCtx, stop := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() { defer close(done); tg.Run(workerCtx) }()
			defer func() { stop(); <-done }()
			select {
			case got := <-sender.messages:
				if got.chatID != 202 {
					t.Fatalf("sent to wrong chat: %d", got.chatID)
				}
				for _, want := range []string{"G'isht terish", "17-sentabr 2026, payshanba", "09:00–18:00", "Ali Valiyev", wantPhone, "Chilonzor", "Bunyodkor", "query=41.311081,69.240562"} {
					if !strings.Contains(got.text, want) {
						t.Errorf("acceptance message missing %q: %s", want, got.text)
					}
				}
			case <-time.After(5 * time.Second):
				t.Fatal("acceptance Telegram was not delivered")
			}
			if count, err := h.Notify.Col.CountDocuments(ctx, bson.M{}); err != nil || count != 1 {
				t.Fatalf("duplicate acceptance notifications: %d %v", count, err)
			}
		})
	}
}
