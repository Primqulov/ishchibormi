// Package channelpost yangi e'lonlarni botning O'ZI administrator bo'lgan
// BARCHA Telegram kanallariga chiqaradi. Foydalanuvchilar ro'yxati hech
// qachon o'qilmaydi va shaxsiy chatlarga e'lon yuborilmaydi.
//
// Kanallar reyestrini bot yuritadi (`my_chat_member` update'lari), bu paket
// esa faqat `status: "active"` yozuvlarni o'qiydi. Ya'ni kimdir botni o'z
// kanaliga admin qilib qo'shsa, keyingi e'londan boshlab o'sha kanal ham
// oladi — hech qanday sozlama shart emas.
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
	BotUsername(context.Context) (string, error)
	ResolveChannel(context.Context, string) (tgsend.ChannelInfo, error)
	SendChannelHTML(context.Context, int64, string, []tgsend.Button) (int64, error)
}

// Bitta tsiklda yuboriladigan post soni. Telegram umumiy chegarasi sekundiga
// ~30 xabar; 5 ta/sek zaxira bilan pastda turadi va kanallar ko'paysa ham
// navbat sezilarli kechikmaydi.
const deliveryBatch = 5

// Fan-out shuncha marta yiqilsa e'lon tark etiladi. Kanal yozuvlari unikal
// indeks tufayli takrorlanmaydi, ya'ni qayta urinish xavfsiz.
const maxFanOutAttempts = 10

// Yetkazishda shuncha marta qayta urinilgach post tark etiladi.
const maxDeliveryAttempts = 24

type Publisher struct {
	elons, posts, channels *mongo.Collection
	sender                 Sender
	botUsername, seed      string
	log                    *slog.Logger
}

// New nil,nil qaytarsa kanalga chiqarish o'chiq — bu xato emas.
//
// seed — ixtiyoriy `TELEGRAM_JOBS_CHANNEL_ID`. Telegram bot ALLAQACHON
// a'zo bo'lgan kanal uchun `my_chat_member` yubormaydi, ya'ni eski sozlangan
// kanal o'z-o'zidan reyestrga tushmaydi. Shuning uchun u ishga tushishda bir
// marta qo'lda qayd etiladi.
func New(db *mongo.Database, sender Sender, botUsername, seed string, log *slog.Logger) (*Publisher, error) {
	if sender == nil || !sender.Configured() {
		return nil, nil
	}
	if !tgsend.ValidBotUsername(botUsername) {
		return nil, errors.New("TELEGRAM_BOT_USERNAME must be the bot that owns TELEGRAM_BOT_TOKEN")
	}
	seed = strings.TrimSpace(seed)
	if seed != "" && !tgsend.ValidChannelReference(seed) {
		return nil, errors.New("TELEGRAM_JOBS_CHANNEL_ID must be @username or a -100... channel ID")
	}
	return &Publisher{
		elons: db.Collection("elons"), posts: db.Collection("channel_posts"), channels: db.Collection("telegram_channels"),
		sender: sender, botUsername: botUsername, seed: seed, log: log,
	}, nil
}

// Queued e'lon hujjatiga yoziladigan belgi. E'lon bilan bitta InsertOne
// ichida ketadi: «e'lon bor, navbat yo'q» holati bo'lmaydi.
func (p *Publisher) Queued(now time.Time) *models.ChannelBroadcast {
	if p == nil {
		return nil
	}
	return &models.ChannelBroadcast{Status: "pending", QueuedAt: now}
}

func (p *Publisher) Run(ctx context.Context) {
	// Token va sozlangan nom bir botga tegishli ekanini BIR MARTA tekshiramiz.
	// Aks holda kanal postidagi «Ish haqida batafsil» tugmasi boshqa botga
	// olib borardi va hech kim ariza bera olmasdi.
	if err := p.verifyIdentity(ctx); err != nil {
		p.log.Warn("channel publication disabled", "reason", err.Error())
		return
	}
	p.registerSeed(ctx)
	go p.loop(ctx, 2*time.Second, func(c context.Context) error {
		_, err := p.fanOutNext(c)
		return err
	})
	p.loop(ctx, time.Second, p.deliverBatch)
}

func (p *Publisher) loop(ctx context.Context, every time.Duration, step func(context.Context) error) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if err := step(ctx); err != nil && ctx.Err() == nil {
			p.log.Warn("channel publication queue failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Publisher) verifyIdentity(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	name, err := p.sender.BotUsername(ctx)
	if err != nil {
		return errors.New("Telegram bot identity could not be read")
	}
	if !strings.EqualFold(name, p.botUsername) {
		return errors.New("TELEGRAM_BOT_USERNAME does not match TELEGRAM_BOT_TOKEN")
	}
	return nil
}

// registerSeed eski sozlamani reyestrga kiritadi. Xatosi halokatli emas:
// kanal botga admin qilib qayta qo'shilsa ham reyestrga tushadi.
func (p *Publisher) registerSeed(parent context.Context) {
	if p.seed == "" {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	info, err := p.sender.ResolveChannel(ctx, p.seed)
	if err != nil {
		p.log.Warn("configured jobs channel not reachable", "reason", "resolve_failed")
		return
	}
	now := time.Now()
	// Operator qo'lda to'xtatgan kanal (blocked) qayta yoqilmaydi.
	_, err = p.channels.UpdateOne(ctx, bson.M{"_id": info.ID, "status": bson.M{"$ne": "blocked"}},
		bson.M{"$set": bson.M{"status": "active", "reason": "configured", "updatedAt": now, "failures": 0}, "$setOnInsert": bson.M{"joinedAt": now}},
		options.Update().SetUpsert(true))
	// Dublikat xatosi = kanal operator tomonidan to'xtatilgan (blocked):
	// filtr unga tushmaydi, upsert esa _id bo'yicha urishadi. Bu normal.
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		p.log.Warn("configured jobs channel not registered")
	}
}

// ── 1-bosqich: fan-out ────────────────────────────────────────────────────

func (p *Publisher) fanOutNext(parent context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	now := time.Now()
	// Fan-out — faqat idempotent yozuv (unikal indeks), shuning uchun uzilgan
	// urinish xavfsiz ravishda qaytariladi. Yuborish bilan farqi shunda.
	if _, err := p.elons.UpdateMany(ctx,
		bson.M{"telegramBroadcast.status": "fanning", "telegramBroadcast.leaseUntil": bson.M{"$lte": now}},
		bson.M{"$set": bson.M{"telegramBroadcast.status": "pending"}}); err != nil {
		return false, err
	}
	var claimed models.Elon
	lease := primitive.NewObjectID()
	err := p.elons.FindOneAndUpdate(ctx,
		bson.M{"telegramBroadcast.status": "pending", "telegramBroadcast.queuedAt": bson.M{"$lte": now}},
		bson.M{"$set": bson.M{"telegramBroadcast.status": "fanning", "telegramBroadcast.leaseId": lease, "telegramBroadcast.leaseUntil": now.Add(2 * time.Minute)}, "$inc": bson.M{"telegramBroadcast.attempts": 1}},
		options.FindOneAndUpdate().SetSort(bson.D{{Key: "telegramBroadcast.queuedAt", Value: 1}}).SetReturnDocument(options.After)).Decode(&claimed)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	active, err := p.activeChannels(ctx)
	if err != nil {
		if claimed.TelegramBroadcast.Attempts >= maxFanOutAttempts {
			return true, p.finishFanOut(ctx, claimed, "failed", 0)
		}
		// Lizing muddati o'tgach yuqoridagi UpdateMany uni qaytaradi.
		return true, nil
	}
	if len(active) > 0 {
		docs := make([]any, 0, len(active))
		for _, ch := range active {
			docs = append(docs, models.ChannelPost{
				ElonID: claimed.ID, ChatID: ch.ChatID, BotUsername: p.botUsername,
				Status: "pending", NextAttemptAt: now, CreatedAt: now,
			})
		}
		// Ordered=false: bitta dublikat qolganlarini to'xtatmasin. Dublikat —
		// uzilgan fan-out qayta ishlaganining normal belgisi.
		if _, err := p.posts.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false)); err != nil && !onlyDuplicates(err) {
			if claimed.TelegramBroadcast.Attempts >= maxFanOutAttempts {
				return true, p.finishFanOut(ctx, claimed, "failed", 0)
			}
			return true, nil
		}
	}
	return true, p.finishFanOut(ctx, claimed, "queued", len(active))
}

// Kanal keyin qo'shilsa ESKI e'lonlarni olmaydi: ro'yxat aynan fan-out
// paytida o'qiladi. Bu ataylab — yangi kanalga yuzlab eski e'lon to'kilmaydi.
func (p *Publisher) activeChannels(ctx context.Context) ([]models.TelegramChannel, error) {
	cur, err := p.channels.Find(ctx, bson.M{"status": "active"})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.TelegramChannel
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func onlyDuplicates(err error) bool {
	var bulk mongo.BulkWriteException
	if !errors.As(err, &bulk) {
		return false
	}
	for _, e := range bulk.WriteErrors {
		if e.Code != 11000 {
			return false
		}
	}
	return len(bulk.WriteErrors) > 0
}

func (p *Publisher) finishFanOut(ctx context.Context, e models.Elon, status string, channels int) error {
	_, err := p.elons.UpdateOne(ctx,
		bson.M{"_id": e.ID, "telegramBroadcast.status": "fanning", "telegramBroadcast.leaseId": e.TelegramBroadcast.LeaseID},
		bson.M{"$set": bson.M{"telegramBroadcast.status": status, "telegramBroadcast.channels": channels, "telegramBroadcast.fannedAt": time.Now()},
			"$unset": bson.M{"telegramBroadcast.leaseUntil": "", "telegramBroadcast.leaseId": ""}})
	if status == "failed" {
		p.log.Warn("channel fan-out abandoned", "listing", e.ID.Hex())
	}
	return err
}

// ── 2-bosqich: yetkazish ──────────────────────────────────────────────────

func (p *Publisher) deliverBatch(ctx context.Context) error {
	for i := 0; i < deliveryBatch; i++ {
		more, err := p.deliverNext(ctx)
		if err != nil || !more {
			return err
		}
	}
	return nil
}

func (p *Publisher) deliverNext(parent context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	now := time.Now()
	// Telegram sendMessage uchun idempotency kaliti bermaydi. Uzilib qolgan
	// yuborish yetib borgan bo'lishi mumkin — restartdan keyin ko'r-ko'rona
	// qaytarilmaydi, aks holda kanalda ikki xil post chiqardi.
	if _, err := p.posts.UpdateMany(ctx,
		bson.M{"status": "sending", "leaseUntil": bson.M{"$lte": now}},
		bson.M{"$set": bson.M{"status": "uncertain", "reason": "interrupted_delivery", "finishedAt": now}}); err != nil {
		return false, err
	}
	var claimed models.ChannelPost
	lease := primitive.NewObjectID()
	err := p.posts.FindOneAndUpdate(ctx,
		bson.M{"status": "pending", "nextAttemptAt": bson.M{"$lte": now}},
		bson.M{"$set": bson.M{"status": "sending", "leaseId": lease, "leaseUntil": now.Add(2 * time.Minute)}, "$inc": bson.M{"attempts": 1}},
		options.FindOneAndUpdate().SetSort(bson.D{{Key: "nextAttemptAt", Value: 1}}).SetReturnDocument(options.After)).Decode(&claimed)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !strings.EqualFold(claimed.BotUsername, p.botUsername) {
		return true, p.finish(ctx, claimed, "failed", "bot_username_mismatch", 0)
	}
	filter := elonquery.ActiveFilter(now, false)
	filter["_id"] = claimed.ElonID
	var live models.Elon
	err = p.elons.FindOne(ctx, filter).Decode(&live)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return true, p.finish(ctx, claimed, "skipped", "listing_unavailable", 0)
	}
	if err != nil {
		return true, p.retry(ctx, claimed, time.Minute, "listing_read_failed")
	}
	if live.WorkersNeeded <= live.AcceptedCount {
		return true, p.finish(ctx, claimed, "skipped", "no_vacancies", 0)
	}
	// Bot kanaldan chiqarilgan bo'lsa navbatdagi eski postlar ham to'xtaydi.
	if p.channels.FindOne(ctx, bson.M{"_id": claimed.ChatID, "status": "active"}).Err() != nil {
		return true, p.finish(ctx, claimed, "skipped", "channel_inactive", 0)
	}
	text, buttons := Message(live, p.botUsername)
	id, err := p.sender.SendChannelHTML(ctx, claimed.ChatID, text, buttons)
	if err == nil && id > 0 {
		p.channelDelivered(ctx, claimed.ChatID)
		return true, p.finish(ctx, claimed, "sent", "", id)
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
			// Aniq rad javobi: bot chiqarilgan, kanal o'chirilgan yoki huquq
			// olingan. Kanalni uzamiz — aks holda har e'lon shu xatoni
			// qaytarib, navbatni bekorga band qilardi.
			p.deactivate(ctx, claimed.ChatID, "telegram_rejected")
			return true, p.finish(ctx, claimed, "failed", "telegram_rejected", 0)
		}
	}
	// Timeout, 5xx yoki uzilgan ulanish — xabar yetdimi, noma'lum. Takroriy
	// post chiqarmaslik uchun shu yerda to'xtaymiz.
	return true, p.finish(ctx, claimed, "uncertain", "delivery_not_confirmed", 0)
}

func (p *Publisher) lease(post models.ChannelPost) bson.M {
	return bson.M{"_id": post.ID, "status": "sending", "leaseId": post.LeaseID}
}
func (p *Publisher) finish(ctx context.Context, post models.ChannelPost, status, reason string, messageID int64) error {
	_, err := p.posts.UpdateOne(ctx, p.lease(post),
		bson.M{"$set": bson.M{"status": status, "reason": reason, "messageId": messageID, "finishedAt": time.Now()},
			"$unset": bson.M{"nextAttemptAt": "", "leaseUntil": "", "leaseId": ""}})
	if status == "failed" || status == "uncertain" {
		p.log.Warn("channel post needs attention", "listing", post.ElonID.Hex(), "chat", post.ChatID, "status", status, "reason", reason)
	}
	return err
}
func (p *Publisher) retry(ctx context.Context, post models.ChannelPost, delay time.Duration, reason string) error {
	if post.Attempts >= maxDeliveryAttempts {
		return p.finish(ctx, post, "failed", reason, 0)
	}
	_, err := p.posts.UpdateOne(ctx, p.lease(post),
		bson.M{"$set": bson.M{"status": "pending", "reason": reason, "nextAttemptAt": time.Now().Add(delay)},
			"$unset": bson.M{"leaseUntil": "", "leaseId": ""}})
	return err
}

// Operator qo'lda to'xtatgan kanal (blocked) hech qachon o'zgartirilmaydi.
func (p *Publisher) deactivate(ctx context.Context, chatID int64, reason string) {
	_, _ = p.channels.UpdateOne(ctx, bson.M{"_id": chatID, "status": "active"},
		bson.M{"$set": bson.M{"status": "inactive", "reason": reason, "leftAt": time.Now(), "updatedAt": time.Now()}})
	p.log.Info("channel deactivated after a definite rejection", "chat", chatID)
}
func (p *Publisher) channelDelivered(ctx context.Context, chatID int64) {
	_, _ = p.channels.UpdateOne(ctx, bson.M{"_id": chatID, "failures": bson.M{"$gt": 0}}, bson.M{"$set": bson.M{"failures": 0}})
}

func Message(e models.Elon, username string) (string, []tgsend.Button) {
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
	buttons := []tgsend.Button{{Text: "Ish haqida batafsil", URL: "https://t.me/" + username + "?start=job_" + e.ID.Hex()}}
	// Xarita — ochiq ma'lumot: ish qayerdaligini bilmasdan unga borib
	// bo'lmaydi va u botda ham, saytda ham allaqachon ko'rsatiladi. Aloqa
	// telefoni, to'liq tavsif va ish beruvchi haqidagi ma'lumot esa
	// kanalga ATAYLAB chiqmaydi: ularni ko'rish uchun odam botga o'tadi.
	if mapURL := mapLink(e); mapURL != "" {
		buttons = append(buttons, tgsend.Button{Text: "🗺 Xaritada ochish", URL: mapURL})
	}
	return text, buttons
}

// mapLink e'lon koordinatalarining xarita havolasi. Koordinata yo'q bo'lsa
// (eski e'lon yoki xaritasiz forma) bo'sh satr — tugma qo'shilmaydi.
//
// Manzil botdagi "Xaritada ochish" bilan bir xil shaklda quriladi, ya'ni
// foydalanuvchi qayerdan bosishidan qat'i nazar ayni joyni ko'radi.
func mapLink(e models.Elon) string {
	if e.Lat == 0 && e.Lng == 0 {
		return ""
	}
	if e.Lat < -90 || e.Lat > 90 || e.Lng < -180 || e.Lng > 180 {
		return ""
	}
	return fmt.Sprintf("https://www.google.com/maps?q=%.6f,%.6f", e.Lat, e.Lng)
}
func money(value int64) string {
	raw := fmt.Sprint(value)
	for i := len(raw) - 3; i > 0; i -= 3 {
		raw = raw[:i] + " " + raw[i:]
	}
	return raw
}
