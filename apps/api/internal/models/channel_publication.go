package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Kanalga chiqarish uch qismdan iborat.
//
// NEGA BIR NECHTA HUJJAT: ilgari bitta sozlangan kanal bor edi va yetkazish
// holati e'lonning o'z hujjatiga sig'ardi. Endi botni O'Z kanaliga qo'shgan
// har kim e'lonlarni oladi, ya'ni bitta e'lon N ta kanalga ketadi va har
// birining natijasi (yuborildi / rad etildi / noaniq) alohida saqlanishi
// kerak.
//
//  1. ChannelBroadcast — e'lon hujjatidagi belgi. E'lon bilan ATOMAR
//     yoziladi: «e'lon yaratildi, lekin navbatga qo'yilmadi» holati bo'lmaydi.
//  2. TelegramChannel — bot administrator bo'lgan kanallar reyestri. Uni bot
//     `my_chat_member` update'lari asosida yuritadi.
//  3. ChannelPost — har (e'lon, kanal) juftligi uchun bitta navbat yozuvi.
//     Unikal indeks takroriy postni imkonsiz qiladi.

// Status qiymatlari ataylab satr: jurnalda va bazada o'qilishi kerak.
type ChannelBroadcast struct {
	// pending — fan-out kutmoqda; queued — kanal yozuvlari yaratilgan.
	Status     string             `bson:"status"`
	QueuedAt   time.Time          `bson:"queuedAt,omitempty"`
	LeaseID    primitive.ObjectID `bson:"leaseId,omitempty"`
	LeaseUntil time.Time          `bson:"leaseUntil,omitempty"`
	// Fan-out paytidagi faol kanallar soni — jurnal va tekshiruv uchun.
	Channels int       `bson:"channels,omitempty"`
	FannedAt time.Time `bson:"fannedAt,omitempty"`
	Attempts int       `bson:"attempts,omitempty"`
}

type ChannelPost struct {
	ID     primitive.ObjectID `bson:"_id,omitempty"`
	ElonID primitive.ObjectID `bson:"elonId"`
	ChatID int64              `bson:"chatId"`
	// Tugma havolasi shu bot nomiga quriladi. Token almashtirilsa eski
	// navbat boshqa botning nomi bilan ketmasligi uchun saqlanadi.
	BotUsername   string             `bson:"botUsername"`
	Status        string             `bson:"status"` // pending|sending|sent|skipped|failed|uncertain
	Attempts      int                `bson:"attempts"`
	NextAttemptAt time.Time          `bson:"nextAttemptAt,omitempty"`
	LeaseID       primitive.ObjectID `bson:"leaseId,omitempty"`
	LeaseUntil    time.Time          `bson:"leaseUntil,omitempty"`
	MessageID     int64              `bson:"messageId,omitempty"`
	// Xarita kartasi asosiy matndan OLDIN yuboriladi va uning ID si darhol
	// saqlanadi. Shunda matn yuborishda xato bo'lsa, qayta urinish kartani
	// ikkinchi marta chiqarmaydi.
	VenueMessageID int64     `bson:"venueMessageId,omitempty"`
	Reason         string    `bson:"reason,omitempty"`
	CreatedAt      time.Time `bson:"createdAt"`
	FinishedAt     time.Time `bson:"finishedAt,omitempty"`
}

// Bot administrator bo'lgan kanal. _id — Telegram chat ID si, ya'ni bir
// kanal ikki marta yozilmaydi va qayta qo'shilganda ayni yozuv tiklanadi.
type TelegramChannel struct {
	ChatID   int64  `bson:"_id"`
	Title    string `bson:"title,omitempty"`
	Username string `bson:"username,omitempty"`
	// active — e'lonlar ketadi; inactive — bot chiqarilgan yoki huquqi
	// olingan; blocked — operator qo'lda to'xtatgan (kod hech qachon
	// o'zi blocked yozmaydi va uni active'ga qaytarmaydi).
	Status   string    `bson:"status"`
	Reason   string    `bson:"reason,omitempty"`
	AddedBy  int64     `bson:"addedBy,omitempty"`
	JoinedAt time.Time `bson:"joinedAt,omitempty"`
	LeftAt   time.Time `bson:"leftAt,omitempty"`
	// Ketma-ket yetkazish xatolari. Kanal o'chirilgan yoki bot bloklangan
	// bo'lsa Telegram qayta-qayta rad etadi — shunda kanal o'zi uziladi.
	Failures  int       `bson:"failures,omitempty"`
	UpdatedAt time.Time `bson:"updatedAt"`
}
