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

const testChat = int64(-1001234567890)
const otherChat = int64(-1009876543210)

type fakeSender struct {
	mu                  sync.Mutex
	calls               map[int64]int
	total               int
	off                 bool
	resolveErr, sendErr error
	username            string
	text                string
	buttons             []tgsend.Button
}

func (f *fakeSender) Configured() bool { return !f.off }
func (f *fakeSender) BotUsername(context.Context) (string, error) {
	return f.username, nil
}
func (f *fakeSender) ResolveChannel(context.Context, string) (tgsend.ChannelInfo, error) {
	return tgsend.ChannelInfo{ID: testChat, BotUsername: f.username}, f.resolveErr
}
func (f *fakeSender) SendChannelHTML(_ context.Context, id int64, text string, b []tgsend.Button) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id >= 0 {
		panic("private destination")
	}
	if f.calls == nil {
		f.calls = map[int64]int{}
	}
	f.calls[id]++
	f.total++
	f.text, f.buttons = text, b
	return 123, f.sendErr
}
func (f *fakeSender) sent(chat int64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[chat]
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
	// Takroriy postning oldini olish AYNAN shu unikal indeksga tayanadi
	// (pkg/db/indexes.go da ham bor) — testda usiz idempotentlik sinalmaydi.
	if _, err := db.Collection("channel_posts").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "elonId", Value: 1}, {Key: "chatId", Value: 1}}, Options: options.Index().SetUnique(true),
	}); err != nil {
		t.Fatal(err)
	}
	f := &fakeSender{username: "testbot"}
	p, err := New(db, f, "testbot", "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	e := models.Elon{ID: primitive.NewObjectID(), Title: "Yuk tushirish", Description: "Private longer details", ContactPhone: "+998901234567", Status: "recruiting", WorkersNeeded: 4, AcceptedCount: 1, PricingType: "total", PriceAmount: 1000000, PerWorkerAmount: 250000, StartDate: time.Now().Add(24 * time.Hour).Format("2006-01-02"), WorkTimeFrom: "09:00", TelegramBroadcast: p.Queued(time.Now())}
	return p, f, e
}

func addChannel(t *testing.T, p *Publisher, chat int64, status string) {
	t.Helper()
	if _, err := p.channels.InsertOne(context.Background(), models.TelegramChannel{ChatID: chat, Status: status, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}
func insertListing(t *testing.T, p *Publisher, e models.Elon) {
	t.Helper()
	if _, err := p.elons.InsertOne(context.Background(), e); err != nil {
		t.Fatal(err)
	}
}
func broadcast(t *testing.T, p *Publisher, id primitive.ObjectID) *models.ChannelBroadcast {
	t.Helper()
	var e models.Elon
	if err := p.elons.FindOne(context.Background(), bson.M{"_id": id}).Decode(&e); err != nil {
		t.Fatal(err)
	}
	return e.TelegramBroadcast
}
func post(t *testing.T, p *Publisher, elon primitive.ObjectID, chat int64) models.ChannelPost {
	t.Helper()
	var out models.ChannelPost
	if err := p.posts.FindOne(context.Background(), bson.M{"elonId": elon, "chatId": chat}).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}
func postCount(t *testing.T, p *Publisher, elon primitive.ObjectID) int64 {
	t.Helper()
	n, err := p.posts.CountDocuments(context.Background(), bson.M{"elonId": elon})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
func channelStatus(t *testing.T, p *Publisher, chat int64) string {
	t.Helper()
	var out models.TelegramChannel
	if err := p.channels.FindOne(context.Background(), bson.M{"_id": chat}).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Status
}

// Navbat bo'shaguncha yetkazadi.
func drain(t *testing.T, p *Publisher) {
	t.Helper()
	for i := 0; i < 50; i++ {
		more, err := p.deliverNext(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			return
		}
	}
	t.Fatal("delivery queue did not drain")
}
func concurrently(n int, step func()) {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); step() }()
	}
	wg.Wait()
}

func TestChannelPostsOncePerChannelAcrossConcurrentWorkersAndRestart(t *testing.T) {
	p, f, e := testPublisher(t)
	addChannel(t, p, testChat, "active")
	addChannel(t, p, otherChat, "active")
	insertListing(t, p, e)

	concurrently(5, func() { _, _ = p.fanOutNext(context.Background()) })
	if b := broadcast(t, p, e.ID); b.Status != "queued" || b.Channels != 2 {
		t.Fatalf("fan-out: %+v", b)
	}
	if n := postCount(t, p, e.ID); n != 2 {
		t.Fatalf("want one post per channel, got %d", n)
	}
	// Uzilgan fan-out qaytarilganda ham ikkinchi post yaratilmaydi.
	if _, err := p.elons.UpdateOne(context.Background(), bson.M{"_id": e.ID}, bson.M{"$set": bson.M{"telegramBroadcast.status": "pending"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.fanOutNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := postCount(t, p, e.ID); n != 2 {
		t.Fatalf("replayed fan-out duplicated posts: %d", n)
	}

	concurrently(5, func() { _, _ = p.deliverNext(context.Background()) })
	if f.total != 2 || f.sent(testChat) != 1 || f.sent(otherChat) != 1 {
		t.Fatalf("delivery: total=%d", f.total)
	}
	if s := post(t, p, e.ID, testChat); s.Status != "sent" || s.MessageID != 123 {
		t.Fatalf("post state: %+v", s)
	}
	restarted, _ := New(p.elons.Database(), f, p.botUsername, "", p.log)
	if _, err := restarted.fanOutNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	drain(t, restarted)
	if f.total != 2 {
		t.Fatal("restart duplicated posts")
	}
	if strings.Contains(f.text, e.ContactPhone) || strings.Contains(f.text, e.Description) || !strings.Contains(f.text, "250 000") {
		t.Fatal("channel summary leaked details")
	}
	if len(f.buttons) == 0 || f.buttons[0].URL != "https://t.me/testbot?start=job_"+e.ID.Hex() {
		t.Fatalf("wrong job link: %+v", f.buttons)
	}
}

// Faqat faol kanallar oladi, va keyin qo'shilgan kanal ESKI e'lonni olmaydi.
func TestChannelFanOutUsesOnlyActiveChannelsAndNeverBackfills(t *testing.T) {
	p, f, e := testPublisher(t)
	addChannel(t, p, testChat, "active")
	addChannel(t, p, otherChat, "blocked")
	addChannel(t, p, -1005555555555, "inactive")
	insertListing(t, p, e)
	if _, err := p.fanOutNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := postCount(t, p, e.ID); n != 1 {
		t.Fatalf("blocked/inactive channels were queued: %d", n)
	}
	drain(t, p)
	if f.sent(testChat) != 1 || f.sent(otherChat) != 0 {
		t.Fatal("a channel that must not receive got a post")
	}
	// Kanal endi faollashdi — fan-out allaqachon tugagan, e'lon qaytarilmaydi.
	if _, err := p.channels.UpdateOne(context.Background(), bson.M{"_id": otherChat}, bson.M{"$set": bson.M{"status": "active"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.fanOutNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	drain(t, p)
	if f.sent(otherChat) != 0 {
		t.Fatal("a newly added channel received an old listing")
	}
}

func TestChannelSkipsUnavailableAndNeverReplaysHistoricalListings(t *testing.T) {
	p, f, base := testPublisher(t)
	addChannel(t, p, testChat, "active")
	for _, kind := range []string{"filled", "expired", "deleted", "blocked", "review", "no_slots", "old"} {
		e := base
		e.ID = primitive.NewObjectID()
		e.TelegramBroadcast = p.Queued(time.Now())
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
			// Belgisi yo'q e'lon — mexanizm qo'shilishidan oldingi tarix.
			e.TelegramBroadcast = nil
		}
		insertListing(t, p, e)
		if _, err := p.fanOutNext(context.Background()); err != nil {
			t.Fatal(err)
		}
		drain(t, p)
		if kind == "old" {
			if postCount(t, p, e.ID) != 0 {
				t.Fatal("historical listing entered the queue")
			}
			continue
		}
		if s := post(t, p, e.ID, testChat); s.Status != "skipped" {
			t.Fatalf("unavailable job not skipped: %s -> %s", kind, s.Status)
		}
	}
	if f.total != 0 {
		t.Fatal("unavailable/historical listing published")
	}
}

func TestChannelRetryOnlyWhenDeliveryIsKnownNotToHaveHappened(t *testing.T) {
	for _, kind := range []string{"rate_limit", "network", "server", "forbidden", "crashed", "wrong_bot", "channel_gone"} {
		t.Run(kind, func(t *testing.T) {
			p, f, e := testPublisher(t)
			addChannel(t, p, testChat, "active")
			insertListing(t, p, e)
			if _, err := p.fanOutNext(context.Background()); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "rate_limit":
				f.sendErr = &tgsend.APIError{Code: 429, RetryAfter: time.Minute}
			case "network":
				f.sendErr = errors.New("timeout")
			case "server":
				f.sendErr = &tgsend.APIError{Code: 500}
			case "forbidden":
				f.sendErr = &tgsend.APIError{Code: 403}
			case "crashed":
				if _, err := p.posts.UpdateOne(context.Background(), bson.M{"elonId": e.ID}, bson.M{"$set": bson.M{"status": "sending", "leaseUntil": time.Now().Add(-time.Minute)}}); err != nil {
					t.Fatal(err)
				}
			case "wrong_bot":
				if _, err := p.posts.UpdateOne(context.Background(), bson.M{"elonId": e.ID}, bson.M{"$set": bson.M{"botUsername": "anotherbot"}}); err != nil {
					t.Fatal(err)
				}
			case "channel_gone":
				if _, err := p.channels.UpdateOne(context.Background(), bson.M{"_id": testChat}, bson.M{"$set": bson.M{"status": "inactive"}}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := p.deliverNext(context.Background()); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{
				"rate_limit": "pending", "network": "uncertain", "server": "uncertain",
				"forbidden": "failed", "crashed": "uncertain", "wrong_bot": "failed", "channel_gone": "skipped",
			}[kind]
			if s := post(t, p, e.ID, testChat); s.Status != want {
				t.Fatalf("got %s want %s", s.Status, want)
			}
			// Aniq rad javobidan keyin kanal uziladi: har e'lon shu xatoni
			// qaytarib navbatni band qilib turmasin.
			if kind == "forbidden" && channelStatus(t, p, testChat) != "inactive" {
				t.Fatal("rejecting channel stayed active")
			}
			calls := f.total
			f.sendErr = nil
			if _, err := p.posts.UpdateOne(context.Background(), bson.M{"elonId": e.ID}, bson.M{"$set": bson.M{"nextAttemptAt": time.Now().Add(-time.Minute)}}); err != nil {
				t.Fatal(err)
			}
			if _, err := p.deliverNext(context.Background()); err != nil {
				t.Fatal(err)
			}
			if want == "pending" {
				if f.total != calls+1 || post(t, p, e.ID, testChat).Status != "sent" {
					t.Fatal("safe retry failed")
				}
			} else if f.total != calls {
				t.Fatal("ambiguous/permanent failure resent")
			}
		})
	}
}

func TestChannelDisabledWithoutTokenAndRejectsWrongBotName(t *testing.T) {
	p, f, _ := testPublisher(t)
	off := &fakeSender{off: true, username: "testbot"}
	disabled, err := New(p.elons.Database(), off, "testbot", "", p.log)
	if err != nil || disabled != nil || disabled.Queued(time.Now()) != nil {
		t.Fatal("publication without a bot token created a queue")
	}
	if _, err := New(p.elons.Database(), f, "not-a-bot", "", p.log); err == nil {
		t.Fatal("a name that is not a bot was accepted")
	}
	if _, err := New(p.elons.Database(), f, "testbot", "not a channel", p.log); err == nil {
		t.Fatal("an invalid seed channel was accepted")
	}
}

func TestChannelMessageEscapesTextAndKeepsPayMeaning(t *testing.T) {
	e := models.Elon{ID: primitive.NewObjectID(), Title: `<a href="https://bad.test">Ish</a> & ish`, WorkersNeeded: 3, PricingType: "negotiable", PerWorkerAmount: 99999, Region: "<b>Viloyat</b>"}
	text, buttons := Message(e, "testbot")
	if strings.Contains(text, "<a ") || strings.Contains(text, "<b>Viloyat") || strings.Contains(text, "99999") || !strings.Contains(text, "Kelishiladi") {
		t.Fatal("unsafe or misleading summary")
	}
	for _, b := range buttons {
		if !b.Valid() {
			t.Fatalf("invalid button: %+v", b)
		}
	}
}

// Kanalga xarita havolasi chiqadi, aloqa ma'lumoti esa CHIQMAYDI: ish
// qayerdaligini bilmasdan unga borib bo'lmaydi, telefon va to'liq tavsif
// esa faqat botda ochiladi.
func TestChannelPostCarriesTheMapButMotContactDetails(t *testing.T) {
	base := models.Elon{
		ID: primitive.NewObjectID(), Title: "Yuk tushirish", WorkersNeeded: 3,
		Description:  "Uzun tavsif va qo'shimcha shartlar",
		ContactPhone: "+998901112233", LocationText: "Chilonzor, 5-kvartal",
		Region: "Toshkent", District: "Chilonzor",
		PricingType: "per_worker", PerWorkerAmount: 150000,
	}
	withMap := base
	withMap.Lat, withMap.Lng = 41.3111, 69.2797

	text, buttons := Message(withMap, "testbot")
	if strings.Contains(text, base.ContactPhone) || strings.Contains(text, base.Description) || strings.Contains(text, base.LocationText) {
		t.Fatalf("channel post leaked private details: %q", text)
	}
	var mapURL string
	for _, b := range buttons {
		if strings.Contains(b.URL, "maps") {
			mapURL = b.URL
		}
		if !b.Valid() {
			t.Fatalf("invalid button: %+v", b)
		}
	}
	if mapURL != "https://www.google.com/maps?q=41.311100,69.279700" {
		t.Fatalf("map link: %q", mapURL)
	}

	// Koordinatasiz e'lon (eski yozuv): tugma umuman qo'shilmaydi, ishlamaydigan
	// havola chiqarishdan ko'ra yo'qligi ma'qul.
	_, plain := Message(base, "testbot")
	for _, b := range plain {
		if strings.Contains(b.URL, "maps") {
			t.Fatalf("map button without coordinates: %+v", b)
		}
	}
	if len(plain) != 1 {
		t.Fatalf("want only the job button, got %+v", plain)
	}
}

// Buzuq koordinata ishlamaydigan havola bo'lib chiqmasligi kerak.
func TestChannelMapLinkRejectsImpossibleCoordinates(t *testing.T) {
	for _, tc := range []struct{ lat, lng float64 }{{0, 0}, {91, 69}, {41, 181}, {-91, -181}} {
		e := models.Elon{ID: primitive.NewObjectID(), Lat: tc.lat, Lng: tc.lng}
		if got := mapLink(e); got != "" {
			t.Fatalf("lat=%v lng=%v -> %q", tc.lat, tc.lng, got)
		}
	}
}
