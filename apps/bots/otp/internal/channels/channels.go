// Package channels bot administrator bo'lgan Telegram kanallari reyestrini
// yuritadi.
//
// NEGA BOTDA: kanalga qo'shilish/chiqarilish haqidagi `my_chat_member`
// update'lari faqat Telegram'ni so'rab turgan jarayonga — ya'ni botga —
// keladi. Backend esa reyestrdan FAQAT o'qiydi (internal/channelpost) va
// har yangi e'lonni o'sha paytdagi faol kanallarga yuboradi.
//
// Ya'ni hech qanday sozlama shart emas: kimdir botni o'z kanaliga admin
// qilib qo'shsa, kanal shu yerda qayd etiladi va keyingi e'londan boshlab
// e'lonlarni oladi.
package channels

import (
	"context"
	"strings"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Registry struct {
	col *mongo.Collection
	Now func() time.Time
}

func New(col *mongo.Collection) *Registry { return &Registry{col: col} }

func (r *Registry) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Event — update'dan ajratib olingan sof ma'lumot. Alohida tur test yozishni
// Telegram tuzilmalarini qo'lda yig'ishdan xalos qiladi.
type Event struct {
	ChatID          int64
	ChatType        string
	Title, Username string
	Status          string
	CanPost         bool
	ActorID         int64
}

// Faol — bot kanalda administrator VA xabar joylash huquqiga ega.
// Huquqsiz administrator e'lon yubora olmaydi, ya'ni u faol emas.
func (e Event) active() bool {
	return e.ChatType == "channel" && e.Status == "administrator" && e.CanPost
}

// FromUpdate faqat kanal a'zoligi o'zgarishini qabul qiladi.
//
// Guruh va superguruhlar ATAYLAB tashlab yuboriladi: e'lonlar oqimi kanal
// uchun mo'ljallangan, guruhda esa u suhbatni bosib ketardi.
func FromUpdate(u tg.Update) (Event, bool) {
	m := u.MyChatMember
	if m == nil || m.Chat.Type != "channel" || m.Chat.ID >= 0 {
		return Event{}, false
	}
	return Event{
		ChatID: m.Chat.ID, ChatType: m.Chat.Type,
		Title: strings.TrimSpace(m.Chat.Title), Username: strings.TrimSpace(m.Chat.UserName),
		Status: m.NewChatMember.Status, CanPost: m.NewChatMember.CanPostMessages,
		ActorID: m.From.ID,
	}, true
}

// Apply reyestrni yangilaydi. Ikkinchi qiymat — kanal endi e'lon oladimi;
// chaqiruvchi shunga qarab tasdiq xabarini yuboradi.
//
// Operator qo'lda to'xtatgan kanal (`status: "blocked"`) hech qachon qayta
// yoqilmaydi: bot qayta qo'shilsa ham u to'xtatilgan bo'lib qoladi. Bu —
// suiiste'mol qilgan kanalni uzishning yagona ishonchli usuli.
func (r *Registry) Apply(ctx context.Context, e Event) (bool, error) {
	now := r.now()
	set := bson.M{"updatedAt": now, "title": e.Title, "username": e.Username}
	if e.active() {
		set["status"], set["reason"], set["failures"] = "active", "", 0
		if e.ActorID > 0 {
			set["addedBy"] = e.ActorID
		}
	} else {
		set["status"] = "inactive"
		set["reason"] = "member_status:" + e.Status
		if e.Status == "administrator" && !e.CanPost {
			set["reason"] = "no_post_permission"
		}
		set["leftAt"] = now
	}
	res, err := r.col.UpdateOne(ctx,
		bson.M{"_id": e.ChatID, "status": bson.M{"$ne": "blocked"}},
		bson.M{"$set": set, "$setOnInsert": bson.M{"joinedAt": now}},
		options.Update().SetUpsert(true))
	// Bloklangan kanal filtrga TUSHMAYDI, shuning uchun upsert uni yangi
	// hujjat sifatida qo'shmoqchi bo'ladi va _id bo'yicha dublikat xatosi
	// beradi. Bu kutilgan natija: yozuv o'zgarmaydi va tasdiq yuborilmaydi.
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if res.MatchedCount == 0 && res.UpsertedCount == 0 {
		return false, nil
	}
	return e.active(), nil
}
