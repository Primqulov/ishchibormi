package elon

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ishchibormi/backend/internal/channelpost"
	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/httpx"
	"github.com/ishchibormi/backend/pkg/tgsend"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const publicationSecret = "publication-test-secret-32-characters-long"

func signPublication(stamp, method, path, body string) string {
	m := hmac.New(sha256.New, []byte(publicationSecret))
	m.Write([]byte(stamp + "\n" + method + "\n" + path + "\n" + body))
	return hex.EncodeToString(m.Sum(nil))
}
func TestBotPublicationSourceAuthenticatesExactBodyAndRestoresIt(t *testing.T) {
	for _, kind := range []string{"normal", "signed", "tampered", "stale", "wrong_path", "unsigned_flag"} {
		t.Run(kind, func(t *testing.T) {
			body := `{"title":"Ish"}`
			stamp := strconv.FormatInt(time.Now().Unix(), 10)
			if kind == "stale" {
				stamp = strconv.FormatInt(time.Now().Add(-2*time.Minute).Unix(), 10)
			}
			path := "/api/elons"
			if kind == "wrong_path" {
				path = "/api/auth/bot/session"
			}
			sig := signPublication(stamp, "POST", path, body)
			if kind == "tampered" {
				body = `{"title":"Changed"}`
			}
			r := httptest.NewRequest("POST", "/api/elons", strings.NewReader(body))
			if kind != "normal" && kind != "unsigned_flag" {
				r.Header.Set("X-Bot-Signature", sig)
				r.Header.Set("X-Bot-Timestamp", stamp)
			}
			r.Header.Set("X-Source", "telegram_bot")
			called := false
			h := BotPublicationSource(publicationSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				source, _ := r.Context().Value(botSourceKey{}).(bool)
				if source != (kind == "signed") {
					t.Error("untrusted source")
				}
				got, _ := io.ReadAll(r.Body)
				if string(got) != body {
					t.Error("body not restored")
				}
				w.WriteHeader(204)
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			valid := kind == "signed" || kind == "normal" || kind == "unsigned_flag"
			if called != valid || (!valid && w.Code != 401) {
				t.Fatalf("invalid source accepted: %s %d", kind, w.Code)
			}
		})
	}
}
func TestChannelEnqueuedAtomicallyForNewListingsFromEveryClient(t *testing.T) {
	db := ownerTestDB(t)
	h := &Handler{Col: db.Collection("elons"), Users: db.Collection("users"), Categories: db.Collection("categories")}
	p, err := channelpost.New(db, tgsend.New("test-only-no-network"), "@jobs_test", "testbot", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	h.Channel = p
	owner, cat := primitive.NewObjectID(), primitive.NewObjectID()
	if _, err := h.Users.InsertOne(context.Background(), models.User{ID: owner, FirstName: "Ali"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Categories.InsertOne(context.Background(), models.Category{ID: cat, Name: "Qurilish", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	start := time.Now().In(uzTZ).Add(2 * time.Hour)
	body := `{"title":"Ish","description":"Vazifa","categoryId":"` + cat.Hex() + `","workersNeeded":2,"pricingType":"per_worker","priceAmount":150000,"startDate":"` + start.Format("2006-01-02") + `","workTimeFrom":"` + start.Format("15:04") + `"}`
	for _, tc := range []struct {
		name, platform                      string
		signed, retryable, review, disabled bool
	}{
		{name: "api_without_source_headers"},
		{name: "web", platform: "web"},
		{name: "ios", platform: "ios"},
		{name: "android", platform: "android"},
		{name: "bot", signed: true, retryable: true},
		{name: "web_retry", platform: "web", retryable: true},
		{name: "ios_retry", platform: "ios", retryable: true},
		{name: "android_retry", platform: "android", retryable: true},
		{name: "review_web", platform: "web", review: true},
		{name: "review_ios", platform: "ios", review: true},
		{name: "review_android", platform: "android", review: true},
		{name: "review_bot", signed: true, review: true},
		{name: "disabled_web", platform: "web", disabled: true},
		{name: "disabled_bot", signed: true, disabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := primitive.NewObjectID().Hex()
			h.Channel = p
			if tc.disabled {
				h.Channel = nil
			}
			call := func() *httptest.ResponseRecorder {
				r := httptest.NewRequest("POST", "/api/elons", strings.NewReader(body))
				if tc.retryable {
					r.Header.Set("Idempotency-Key", key)
				}
				if tc.platform != "" {
					r.Header.Set("X-Client-Platform", tc.platform)
				}
				ctx := context.WithValue(r.Context(), httpx.CtxUserID, owner.Hex())
				if tc.review {
					ctx = context.WithValue(ctx, httpx.CtxReviewActor, true)
				}
				r = r.WithContext(ctx)
				if tc.signed {
					stamp := strconv.FormatInt(time.Now().Unix(), 10)
					r.Header.Set("X-Bot-Timestamp", stamp)
					r.Header.Set("X-Bot-Signature", signPublication(stamp, "POST", "/api/elons", body))
				}
				w := httptest.NewRecorder()
				BotPublicationSource(publicationSecret)(http.HandlerFunc(h.Create)).ServeHTTP(w, r)
				return w
			}
			w := call()
			if w.Code != 201 {
				t.Fatalf("create: %d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "telegramChannel") {
				t.Fatal("internal delivery data leaked")
			}
			var result models.Elon
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			var saved models.Elon
			if err := h.Col.FindOne(context.Background(), bson.M{"_id": result.ID}).Decode(&saved); err != nil {
				t.Fatal(err)
			}
			wantQueued := !tc.review && !tc.disabled
			if (saved.TelegramChannel != nil) != wantQueued {
				t.Fatalf("channel queue: got %+v, want queued=%t", saved.TelegramChannel, wantQueued)
			}
			if wantQueued && (saved.TelegramChannel.Status != "pending" || saved.TelegramChannel.Reference != "@jobs_test" || saved.TelegramChannel.BotUsername != "testbot") {
				t.Fatalf("incorrect destination/state: %+v", saved.TelegramChannel)
			}
			if tc.retryable {
				if _, err := h.Col.UpdateOne(context.Background(), bson.M{"_id": result.ID}, bson.M{"$set": bson.M{"telegramChannel.status": "sent", "telegramChannel.messageId": 123}}); err != nil {
					t.Fatal(err)
				}
				retry := call()
				if retry.Code != 200 {
					t.Fatal("retry did not recover")
				}
				var recovered models.Elon
				if err := json.Unmarshal(retry.Body.Bytes(), &recovered); err != nil || recovered.ID != result.ID {
					t.Fatal("retry returned a different listing", err)
				}
				if err := h.Col.FindOne(context.Background(), bson.M{"_id": result.ID}).Decode(&saved); err != nil {
					t.Fatal(err)
				}
				if saved.TelegramChannel.Status != "sent" || saved.TelegramChannel.MessageID != 123 {
					t.Fatal("retry reenqueued channel post")
				}
			}
		})
	}
}
