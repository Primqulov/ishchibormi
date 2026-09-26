package application

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Bir kunga bitta ish — atomik kafolat.
//
// decide() dagi "ishchi shu kunga band emasmi?" tekshiruvi o'qib-keyin-yozish
// shaklida: ikki ish beruvchi bir ishchini bir vaqtda qabul qilsa, ikkalasi
// ham tekshiruvdan o'tib ketardi. Mongo standalone rejimida tranzaksiya yo'q,
// shuning uchun (ishchi, kun) juftligiga alohida qulf hujjati olinadi. Uning
// _id si shu juftlikning o'zi — ya'ni unikallikni qo'shimcha indekssiz Mongo
// o'zi kafolatlaydi: bir vaqtda faqat bitta InsertOne muvaffaqiyatli bo'ladi.
//
// Qulf ariza holatining faqat "yordamchi nusxasi". Ariza keyin bekor qilinsa
// yoki rad etilsa, qulf o'chirilmaydi — har bir holat o'tishiga ilmoq qo'yish
// o'rniga, keyingi da'vogar qulf egasini tekshiradi va u endi kunni band
// qilmayotgan bo'lsa (eskirgan qulf), uni olib tashlab qayta urinadi.

const dayLocksCollection = "worker_day_locks"

// dayLockPendingGrace — qulfni olgan, lekin arizasi hali "accepted" ga
// o'tmagan (jarayon ichida) da'vogarni himoya qiladigan oyna. Shu vaqtdan
// keyin ham "pending" qolgan qulf — uzilib qolgan so'rov izi, eskirgan.
const dayLockPendingGrace = 2 * time.Minute

var errWorkerBusyDay = errors.New("worker already booked for this day")

func dayLockID(workerID primitive.ObjectID, day string) string {
	return workerID.Hex() + ":" + day
}

func (h *Handler) dayLocks() *mongo.Collection {
	return h.Apps.Database().Collection(dayLocksCollection)
}

// claimWorkerDay ishchining shu kunini appID uchun band qiladi. Kun boshqa
// amaldagi ariza bilan band bo'lsa errWorkerBusyDay qaytaradi. day bo'sh
// bo'lsa (sanasiz e'lon) hech narsa qilinmaydi — decide() dagi eski tekshiruv
// ham bunday e'lonlarni cheklamaydi.
func (h *Handler) claimWorkerDay(ctx context.Context, workerID primitive.ObjectID, day string, appID primitive.ObjectID) error {
	if day == "" {
		return nil
	}
	id := dayLockID(workerID, day)
	for attempt := 0; attempt < 3; attempt++ {
		now := time.Now()
		_, err := h.dayLocks().InsertOne(ctx, bson.M{
			"_id": id, "workerId": workerID, "day": day, "applicationId": appID,
			"createdAt": now, "expiresAt": dayLockExpiry(day, now),
		})
		if err == nil {
			return nil
		}
		if !mongo.IsDuplicateKeyError(err) {
			return err
		}
		var held struct {
			ApplicationID primitive.ObjectID `bson:"applicationId"`
			CreatedAt     time.Time          `bson:"createdAt"`
		}
		if err := h.dayLocks().FindOne(ctx, bson.M{"_id": id}).Decode(&held); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				continue // egasi hozirgina bo'shatdi — qayta urinamiz
			}
			return err
		}
		if held.ApplicationID == appID {
			return nil
		}
		if h.dayLockHolds(ctx, workerID, day, held.ApplicationID, held.CreatedAt) {
			return errWorkerBusyDay
		}
		// Eskirgan qulf: faqat aynan o'sha egani o'chiramiz, shunda shu orada
		// boshqa da'vogar olgan yangi qulfga tegilmaydi.
		if _, err := h.dayLocks().DeleteOne(ctx, bson.M{"_id": id, "applicationId": held.ApplicationID}); err != nil {
			return err
		}
	}
	return errWorkerBusyDay
}

// dayLockHolds — qulf egasi bo'lgan ariza hali ham shu kunni band qiladimi.
func (h *Handler) dayLockHolds(ctx context.Context, workerID primitive.ObjectID, day string, holder primitive.ObjectID, lockedAt time.Time) bool {
	var app struct {
		Status string `bson:"status"`
	}
	if err := h.Apps.FindOne(ctx, bson.M{"_id": holder}).Decode(&app); err != nil {
		// Topilmadi — eskirgan. Boshqa xato — ehtiyot uchun band deb hisoblaymiz.
		return !errors.Is(err, mongo.ErrNoDocuments)
	}
	switch app.Status {
	case "pending":
		return time.Since(lockedAt) < dayLockPendingGrace
	case "accepted":
		// E'lon sanasi o'zgargan bo'lishi mumkin — kunni hali ham shu ariza
		// egallaydimi, aynan decide() ishlatadigan qoida bilan tekshiramiz.
		for _, a := range h.workerAppsOnDay(ctx, workerID, "accepted", day, primitive.NilObjectID) {
			if a.ID == holder {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// releaseWorkerDay qulfni faqat appID o'zi egasi bo'lsa bo'shatadi.
func (h *Handler) releaseWorkerDay(ctx context.Context, workerID primitive.ObjectID, day string, appID primitive.ObjectID) {
	if day == "" {
		return
	}
	_, _ = h.dayLocks().DeleteOne(ctx, bson.M{"_id": dayLockID(workerID, day), "applicationId": appID})
}

// dayLockExpiry — kun o'tib ketgach qulf keraksiz; TTL indeksi uni ikki
// kundan keyin yig'ishtiradi (pkg/db/indexes.go).
func dayLockExpiry(day string, now time.Time) time.Time {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return now.Add(48 * time.Hour)
	}
	return t.Add(72 * time.Hour)
}
