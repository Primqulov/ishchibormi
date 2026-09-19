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
	maps                int
	mapErr              error
	mapLat, mapLng      float64
}

func (f *fakeSender) Configured() bool { return !f.off }
func (f *fakeSender) BotUsername(context.Context) (string, error) {
	return f.username, nil
}
func (f *fakeSender) ResolveChannel(context.Context, string) (tgsend.ChannelInfo, error) {
	return tgsend.ChannelInfo{ID: testChat, BotUsername: f.username}, f.resolveErr
}
func (f *fakeSender) SendChannelLocation(_ context.Context, id int64, lat, lng float64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id >= 0 {
		panic("private destination")
	}
	f.mapLat, f.mapLng = lat, lng
	if f.mapErr != nil {
		return 0, f.mapErr
	}
	f.maps++
	return 77, nil
}
func (f *fakeSender) SendChannelHTML(_ context.Context, id int64, text string, b []tgsend.Button) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id >= 0 {
		panic("private destination")
	}
	f.text, f.buttons = text, b
	if f.sendErr != nil {
		// Yiqilgan urinish YUBORILGAN xabar emas: sanoq faqat kanalga
		// haqiqatan chiqqan postlarni hisoblaydi, aks holda "takror
		// yuborilmadi" tekshiruvlari ma'nosini yo'qotardi.
		return 0, f.sendErr
	}
	if f.calls == nil {
		f.calls = map[int64]int{}
	}
	f.calls[id]++
	f.total++
	return 123, nil
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
	// Koordinatasiz e'lon matn posti bo'lib qoladi, ya'ni HTML ekranlanishi
	// shu yerda tekshiriladi.
	post := Message(e, "testbot")
	if post.Map {
		t.Fatal("a listing without coordinates got a map card")
	}
	if strings.Contains(post.Text, "<a ") || strings.Contains(post.Text, "<b>Viloyat") || strings.Contains(post.Text, "99999") || !strings.Contains(post.Text, "Kelishiladi") {
		t.Fatal("unsafe or misleading summary")
	}
	for _, b := range post.Buttons {
		if !b.Valid() {
			t.Fatalf("invalid button: %+v", b)
		}
	}
}

// Kanalga xarita kartasi va uning ostidagi to'liq matn chiqadi, aloqa
// ma'lumoti esa CHIQMAYDI: ish qayerdaligini bilmasdan unga borib bo'lmaydi,
// telefon va to'liq tavsif esa faqat botda ochiladi.
func TestChannelPostCarriesTheMapButNotContactDetails(t *testing.T) {
	base := models.Elon{
		ID: primitive.NewObjectID(), Title: "Yuk tushirish", WorkersNeeded: 3,
		Description:  "Uzun tavsif va qo'shimcha shartlar",
		ContactPhone: "+998901112233", LocationText: "Chilonzor, 5-kvartal",
		Region: "Toshkent", District: "Chilonzor",
		PricingType: "per_worker", PerWorkerAmount: 150000,
	}
	withMap := base
	withMap.Lat, withMap.Lng = 41.3111, 69.2797

	// Koordinatasi bor e'lon VENUE bo'ladi: Telegram uni o'z xarita
	// kartasida ko'rsatadi va tashqi brauzerga chiqib ketmaydi.
	post := Message(withMap, "testbot")
	if !post.Map || post.Lat != withMap.Lat || post.Lng != withMap.Lng {
		t.Fatalf("listing with coordinates got no map card: %+v", post)
	}

	// Matn kartaning OSTIDA turadi va to'liq tafsilotni beradi. Kartaning
	// o'zi yalang'och: sarlavha/manzil bo'lsa ular shu matnda takrorlanardi.
	for _, want := range []string{"Yuk tushirish", "3 kishi", "150 000", "Chilonzor"} {
		if !strings.Contains(post.Text, want) {
			t.Fatalf("text under the card is missing %q: %s", want, post.Text)
		}
	}
	if strings.Contains(post.Text, base.ContactPhone) || strings.Contains(post.Text, base.Description) || strings.Contains(post.Text, base.LocationText) {
		t.Fatalf("text under the card leaked private details: %q", post.Text)
	}
	if len(post.Buttons) != 1 || !strings.Contains(post.Buttons[0].URL, "start=job_") {
		t.Fatalf("want only the job button, got %+v", post.Buttons)
	}

	// Koordinatasiz e'lon (eski yozuv) faqat matn posti bo'ladi — karta yo'q.
	plain := Message(base, "testbot")
	if plain.Map || plain.Text == "" {
		t.Fatalf("listing without coordinates: %+v", plain)
	}
}

// Buzuq koordinata xarita kartasiga aylanmasligi kerak: Telegram uni rad
// etadi va butun post yuborilmay qolardi.
func TestChannelRejectsImpossibleCoordinates(t *testing.T) {
	for _, tc := range []struct{ lat, lng float64 }{{0, 0}, {91, 69}, {41, 181}, {-91, -181}} {
		e := models.Elon{ID: primitive.NewObjectID(), Title: "Ish", WorkersNeeded: 2, Lat: tc.lat, Lng: tc.lng}
		if post := Message(e, "testbot"); post.Map {
			t.Fatalf("lat=%v lng=%v got a map card", tc.lat, tc.lng)
		}
	}
}

// Xarita kartasi matndan OLDIN ketadi va qayta urinishda IKKINCHI marta
// chiqmaydi: aks holda kanalda ikkita bir xil karta qolardi.
func TestChannelSendsTheMapCardBeforeTheTextAndNeverTwice(t *testing.T) {
	p, f, e := testPublisher(t)
	e.Lat, e.Lng = 41.3111, 69.2797
	addChannel(t, p, testChat, "active")
	insertListing(t, p, e)
	if _, err := p.fanOutNext(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Matn yuborishda vaqtinchalik xato: karta allaqachon ketgan.
	f.sendErr = &tgsend.APIError{Code: 429, RetryAfter: time.Second}
	if _, err := p.deliverNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	saved := post(t, p, e.ID, testChat)
	if f.maps != 1 || saved.MapMessageID != 77 {
		t.Fatalf("map card not recorded: venues=%d saved=%+v", f.maps, saved)
	}
	if f.total != 0 || saved.Status != "pending" {
		t.Fatalf("text send state: total=%d status=%s", f.total, saved.Status)
	}
	if f.mapLat != e.Lat || f.mapLng != e.Lng {
		t.Fatalf("map card coordinates: %v,%v", f.mapLat, f.mapLng)
	}

	f.sendErr = nil
	if _, err := p.posts.UpdateOne(context.Background(), bson.M{"elonId": e.ID}, bson.M{"$set": bson.M{"nextAttemptAt": time.Now().Add(-time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.deliverNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.maps != 1 {
		t.Fatalf("map card sent %d times", f.maps)
	}
	if f.total != 1 || post(t, p, e.ID, testChat).Status != "sent" {
		t.Fatalf("text not delivered after retry: total=%d", f.total)
	}
}

// Karta yuborilmasa matn ham ketmaydi: tafsilotsiz karta ham, kartasiz
// tafsilot ham yarim post bo'lib qolardi.
func TestChannelSkipsTheTextWhenTheMapCardFails(t *testing.T) {
	p, f, e := testPublisher(t)
	e.Lat, e.Lng = 41.3111, 69.2797
	addChannel(t, p, testChat, "active")
	insertListing(t, p, e)
	if _, err := p.fanOutNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mapErr = &tgsend.APIError{Code: 403}
	if _, err := p.deliverNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.total != 0 {
		t.Fatal("text sent without its map card")
	}
	if s := post(t, p, e.ID, testChat); s.Status != "failed" {
		t.Fatalf("status = %s, want failed", s.Status)
	}
	// Aniq rad javobi kanalni uzadi — avvalgidek.
	if channelStatus(t, p, testChat) != "inactive" {
		t.Fatal("rejecting channel stayed active")
	}
}
