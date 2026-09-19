package channelpost

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/tgsend"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type fakeSender struct {
	mu                  sync.Mutex
	calls               int
	resolveErr, sendErr error
	username            string
	text                string
	button              tgsend.Button
}

func (f *fakeSender) Configured() bool { return true }
func (f *fakeSender) ResolveChannel(context.Context, string) (tgsend.ChannelInfo, error) {
	return tgsend.ChannelInfo{ID: -1001234567890, BotUsername: f.username}, f.resolveErr
}
func (f *fakeSender) SendChannelHTML(_ context.Context, id int64, text string, b tgsend.Button) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id != -1001234567890 {
		panic("private destination")
	}
	f.calls++
	f.text, f.button = text, b
	return 123, f.sendErr
}
func testPublisher(t *testing.T) (*Publisher, *fakeSender, models.Elon) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://127.0.0.1:27017"))
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Ping(ctx, nil); err != nil {
		_ = c.Disconnect(context.Background())
		t.Skip("local Mongo unavailable")
	}
	db := c.Database("ib_channel_post_test_" + primitive.NewObjectID().Hex())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = db.Drop(ctx)
		_ = c.Disconnect(ctx)
	})
	f := &fakeSender{username: "testbot"}
	p, err := New(db, f, "@jobs_test", "testbot", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	e := models.Elon{ID: primitive.NewObjectID(), Title: "Yuk tushirish", Description: "Private longer details", ContactPhone: "+998901234567", Status: "recruiting", WorkersNeeded: 4, AcceptedCount: 1, PricingType: "total", PriceAmount: 1000000, PerWorkerAmount: 250000, StartDate: time.Now().Add(24 * time.Hour).Format("2006-01-02"), WorkTimeFrom: "09:00", TelegramChannel: p.Pending(time.Now())}
	return p, f, e
}
func insertListing(t *testing.T, p *Publisher, e models.Elon) {
	t.Helper()
	if _, err := p.col.InsertOne(context.Background(), e); err != nil {
		t.Fatal(err)
	}
}
func state(t *testing.T, p *Publisher, id primitive.ObjectID) *models.ChannelPublication {
	t.Helper()
	var e models.Elon
	if err := p.col.FindOne(context.Background(), bson.M{"_id": id}).Decode(&e); err != nil {
		t.Fatal(err)
	}
	return e.TelegramChannel
}
func TestChannelPublishesOnceAcrossConcurrentWorkersAndRestart(t *testing.T) {
	p, f, e := testPublisher(t)
	insertListing(t, p, e)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.deliverNext(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	s := state(t, p, e.ID)
	if f.calls != 1 || s.Status != "sent" || s.MessageID != 123 || s.ChatID != -1001234567890 {
		t.Fatalf("delivery: calls=%d state=%+v", f.calls, s)
	}
	restarted, _ := New(p.col.Database(), f, p.reference, p.botUsername, p.log)
	if _, err := restarted.deliverNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatal("restart duplicated post")
	}
	if strings.Contains(f.text, e.ContactPhone) || strings.Contains(f.text, e.Description) || !strings.Contains(f.text, "250 000") || f.button.URL != "https://t.me/testbot?start=job_"+e.ID.Hex() {
		t.Fatal("channel summary leaked details or wrong job link")
	}
}
func TestChannelSkipsUnavailableAndNeverReplaysHistoricalListings(t *testing.T) {
	p, f, base := testPublisher(t)
	for _, kind := range []string{"filled", "expired", "deleted", "blocked", "review", "no_slots", "old"} {
		e := base
		e.ID = primitive.NewObjectID()
		e.TelegramChannel = p.Pending(time.Now())
		switch kind {
		case "filled":
			e.Status = "filled"
		case "expired":
			e.StartDate = "2020-01-01"
		case "deleted":
			e.IsDeleted = true
		case "blocked":
			e.OwnerBlocked = true
		case "review":
			e.IsReviewData = true
		case "no_slots":
			e.AcceptedCount = e.WorkersNeeded
		case "old":
			e.TelegramChannel = nil
		}
		insertListing(t, p, e)
		if _, err := p.deliverNext(context.Background()); err != nil {
			t.Fatal(err)
		}
		if kind != "old" && state(t, p, e.ID).Status != "skipped" {
			t.Fatal("unavailable job not skipped", kind)
		}
	}
	if f.calls != 0 {
		t.Fatal("unavailable/historical listing published")
	}
}
func TestChannelRetryOnlyWhenDeliveryIsKnownNotToHaveHappened(t *testing.T) {
	for _, kind := range []string{"rate_limit", "resolve", "network", "server", "forbidden", "crashed", "wrong_bot"} {
		t.Run(kind, func(t *testing.T) {
			p, f, e := testPublisher(t)
			switch kind {
			case "rate_limit":
				f.sendErr = &tgsend.APIError{Code: 429, RetryAfter: time.Minute}
			case "resolve":
				f.resolveErr = errors.New("offline")
			case "network":
				f.sendErr = errors.New("timeout")
			case "server":
				f.sendErr = &tgsend.APIError{Code: 500}
			case "forbidden":
				f.sendErr = &tgsend.APIError{Code: 403}
			case "crashed":
				e.TelegramChannel.Status = "sending"
				e.TelegramChannel.LeaseUntil = time.Now().Add(-time.Minute)
			case "wrong_bot":
				f.username = "anotherbot"
			}
			insertListing(t, p, e)
			if _, err := p.deliverNext(context.Background()); err != nil {
				t.Fatal(err)
			}
			s := state(t, p, e.ID)
			want := "uncertain"
			if kind == "rate_limit" || kind == "resolve" {
				want = "pending"
			}
			if kind == "forbidden" || kind == "wrong_bot" {
				want = "failed"
			}
			if s.Status != want {
				t.Fatalf("got %s want %s", s.Status, want)
			}
			calls := f.calls
			f.sendErr, f.resolveErr = nil, nil
			if _, err := p.col.UpdateOne(context.Background(), bson.M{"_id": e.ID}, bson.M{"$set": bson.M{"telegramChannel.nextAttemptAt": time.Now().Add(-time.Minute)}}); err != nil {
				t.Fatal(err)
			}
			if _, err := p.deliverNext(context.Background()); err != nil {
				t.Fatal(err)
			}
			if want == "pending" {
				if f.calls != calls+1 || state(t, p, e.ID).Status != "sent" {
					t.Fatal("safe retry failed")
				}
			} else if f.calls != calls {
				t.Fatal("ambiguous/permanent failure resent")
			}
		})
	}
}
func TestChannelDisabledAndChangedTargetDoNotBroadcast(t *testing.T) {
	p, f, e := testPublisher(t)
	insertListing(t, p, e)
	disabled, err := New(p.col.Database(), f, "", "testbot", p.log)
	if err != nil || disabled != nil || disabled.Pending(time.Now()) != nil {
		t.Fatal("disabled publication created queue")
	}
	other, err := New(p.col.Database(), f, "@other_channel", "testbot", p.log)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.deliverNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.calls != 0 {
		t.Fatal("pending post redirected to another channel")
	}
}
func TestChannelMessageEscapesTextAndKeepsPayMeaning(t *testing.T) {
	e := models.Elon{ID: primitive.NewObjectID(), Title: `<a href="https://bad.test">Ish</a> & ish`, WorkersNeeded: 3, PricingType: "negotiable", PerWorkerAmount: 99999, Region: "<b>Viloyat</b>"}
	text, b := Message(e, "testbot")
	if strings.Contains(text, "<a ") || strings.Contains(text, "<b>Viloyat") || strings.Contains(text, "99999") || !strings.Contains(text, "Kelishiladi") || !b.Valid() {
		t.Fatal("unsafe or misleading summary")
	}
}
