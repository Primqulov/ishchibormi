// Package jobalert — «ish signali»: foydalanuvchi bir marta hudud (va xohlasa
// ish turi) belgilaydi, keyin o'sha hududda yangi e'lon chiqqanda xabar oladi.
//
// NEGA KERAK: ilgari ish topishning yagona yo'li foydalanuvchining o'zi kelib
// qidirishi edi. Kunlik ishda e'lon bir necha soatda to'ladi, ya'ni kech kelgan
// odam hech narsa topmaydi. Signal shu oqimni teskarisiga o'giradi.
//
// Yetkazish alohida yozilmagan: xabar odatdagi notification.Service orqali
// o'tadi, ya'ni ilova ichidagi ro'yxat, FCM push va Telegram navbati (qayta
// urinish, lizing bilan) tayyor holda ishlaydi.
package jobalert

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/internal/notification"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	// Radius chegaralari. Kunlik ishda odam piyoda yoki bir marta transportda
	// boradi — 50 km dan uzoq "yaqin ish" emas, 1 km dan kichigi esa amalda
	// hech qachon ishlamaydi (e'lon koordinatasi ham taxminiy).
	MinRadiusM     = 1000
	MaxRadiusM     = 50000
	DefaultRadiusM = 10000

	// Bitta e'lon uchun eng ko'pi bilan shuncha odamga xabar ketadi. Cheksiz
	// qoldirilsa bitta e'lon butun bazani bildirishnoma navbatiga tiqib
	// qo'yishi mumkin edi.
	maxRecipients = 300

	// internal/elon/nearby.go dagi agregatsiya bilan bir xil Yer radiusi.
	earthRadiusM = 6371008.8
)

// Alert — bitta foydalanuvchining signali. _id sifatida userId ishlatiladi:
// bir odamda bitta signal bo'ladi, shuning uchun yozish idempotent upsert va
// dublikat yozuv umuman paydo bo'lmaydi.
type Alert struct {
	UserID       primitive.ObjectID  `bson:"_id" json:"-"`
	Lat          float64             `bson:"lat" json:"lat"`
	Lng          float64             `bson:"lng" json:"lng"`
	RadiusM      int                 `bson:"radiusM" json:"radiusM"`
	CategoryID   *primitive.ObjectID `bson:"categoryId,omitempty" json:"categoryId,omitempty"`
	CategoryName string              `bson:"categoryName,omitempty" json:"categoryName,omitempty"`
	PlaceName    string              `bson:"placeName,omitempty" json:"placeName,omitempty"`
	CreatedAt    time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time           `bson:"updatedAt" json:"updatedAt"`
}

type Service struct {
	Col        *mongo.Collection
	Categories *mongo.Collection
	Notify     *notification.Service
	Log        *slog.Logger
}

func New(db *mongo.Database, n *notification.Service, log *slog.Logger) *Service {
	return &Service{
		Col:        db.Collection("job_alerts"),
		Categories: db.Collection("categories"),
		Notify:     n, Log: log,
	}
}

func (s *Service) load(ctx context.Context, userID primitive.ObjectID) (*Alert, error) {
	var a Alert
	err := s.Col.FindOne(ctx, bson.M{"_id": userID}).Decode(&a)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Service) store(ctx context.Context, a Alert) (*Alert, error) {
	now := time.Now()
	a.UpdatedAt = now
	set := bson.M{
		"lat": a.Lat, "lng": a.Lng, "radiusM": a.RadiusM,
		"categoryName": a.CategoryName, "placeName": a.PlaceName, "updatedAt": now,
	}
	// Turkum tanlanmagan bo'lsa maydonni O'CHIRAMIZ: qolib ketsa foydalanuvchi
	// "barcha ish turlari"ga o'tgan bo'lsa ham eski filtr ishlayverardi.
	unset := bson.M{}
	if a.CategoryID != nil {
		set["categoryId"] = *a.CategoryID
	} else {
		unset["categoryId"] = ""
	}
	update := bson.M{"$set": set, "$setOnInsert": bson.M{"createdAt": now}}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	if _, err := s.Col.UpdateOne(ctx, bson.M{"_id": a.UserID}, update, options.Update().SetUpsert(true)); err != nil {
		return nil, err
	}
	return s.load(ctx, a.UserID)
}

func (s *Service) remove(ctx context.Context, userID primitive.ObjectID) error {
	_, err := s.Col.DeleteOne(ctx, bson.M{"_id": userID})
	return err
}

// NewElon — yangi e'lon chop etilgandan keyin chaqiriladi va DARHOL qaytadi:
// tarqatish alohida goroutine'da, so'rov kontekstidan mustaqil ketadi.
// E'lon yaratish javobi hech qachon signal tarqatishni kutmaydi.
func (s *Service) NewElon(e models.Elon) {
	if s == nil || s.Notify == nil {
		return
	}
	// Play review demo ma'lumoti real odamga hech qachon ko'rinmasligi kerak.
	if e.IsReviewData || e.Status != "recruiting" {
		return
	}
	if !validCoordinates(e.Lat, e.Lng) || (e.Lat == 0 && e.Lng == 0) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.fanout(ctx, e); err != nil && s.Log != nil {
			s.Log.Warn("job alert fanout failed", "elon", e.ID.Hex(), "err", err)
		}
	}()
}

func (s *Service) fanout(ctx context.Context, e models.Elon) error {
	// Mongo'dan faqat qo'pol to'rtburchak ichidagilar olinadi (indeks shu
	// yerda ishlaydi), aniq masofa esa Go tomonida — har bir signalning O'Z
	// radiusi bor, ya'ni bitta so'rov bilan filtrlab bo'lmaydi.
	latDelta, lngDelta := boundingDelta(e.Lat, MaxRadiusM)
	filter := bson.M{
		"lat": bson.M{"$gte": e.Lat - latDelta, "$lte": e.Lat + latDelta},
		"lng": bson.M{"$gte": e.Lng - lngDelta, "$lte": e.Lng + lngDelta},
		"_id": bson.M{"$ne": e.OwnerID},
	}
	if !e.CategoryID.IsZero() {
		filter["$or"] = []bson.M{
			{"categoryId": bson.M{"$exists": false}},
			{"categoryId": e.CategoryID},
		}
	}
	cur, err := s.Col.Find(ctx, filter, options.Find().SetLimit(maxRecipients).SetMaxTime(5*time.Second))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var a Alert
		if err := cur.Decode(&a); err != nil {
			continue
		}
		radius := a.RadiusM
		if radius <= 0 {
			radius = DefaultRadiusM
		}
		meters := DistanceMeters(a.Lat, a.Lng, e.Lat, e.Lng)
		if meters > float64(radius) {
			continue
		}
		s.Notify.Push(ctx, a.UserID, "job_nearby", "Yaqiningizda yangi ish",
			e.Title+" — "+DistanceText(meters)+" uzoqlikda",
			&models.RelatedEntity{Type: "elon", ID: e.ID})
	}
	return cur.Err()
}

// boundingDelta — radiusni gradusga o'giradi. Uzunlik daraja kengaylik bo'yicha
// toraygani uchun kosinusga bo'linadi; qutblarga yaqin joyda nolga bo'linmaslik
// uchun pastdan cheklanadi (O'zbekistonda bunday qiymat uchramaydi, lekin
// buzuq koordinata kelib qolsa ham funksiya xavfsiz qoladi).
func boundingDelta(lat float64, radiusM int) (float64, float64) {
	// Bir gradus necha metr — AYNAN quyidagi haversine bilan bir xil Yer
	// radiusidan olinadi. Boshqa konstanta (masalan 111320) olinsa to'rtburchak
	// radiusdan bir necha o'n metr kichik chiqadi va chekkadagi signallar
	// jimgina tushib qolardi. safety — suzuvchi nuqta yaxlitlashi uchun.
	const safety = 1.001
	metersPerDegree := earthRadiusM * math.Pi / 180
	latDelta := float64(radiusM) * safety / metersPerDegree
	cos := math.Cos(lat * math.Pi / 180)
	if cos < 0.01 {
		cos = 0.01
	}
	return latDelta, float64(radiusM) * safety / (metersPerDegree * cos)
}

// DistanceMeters — haversine. internal/elon/nearby.go dagi agregatsiya bilan
// bir xil formula va Yer radiusi, ya'ni signal va qidiruv bir xil masofani
// ko'rsatadi.
func DistanceMeters(lat1, lng1, lat2, lng2 float64) float64 {
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dPhi, dLambda := (lat2-lat1)*math.Pi/180, (lng2-lng1)*math.Pi/180
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dLambda/2)*math.Sin(dLambda/2)
	return 2 * earthRadiusM * math.Asin(math.Sqrt(math.Min(1, math.Max(0, a))))
}

func DistanceText(meters float64) string {
	if meters < 1000 {
		return strconv.FormatFloat(math.Round(meters), 'f', 0, 64) + " m"
	}
	return strconv.FormatFloat(meters/1000, 'f', 1, 64) + " km"
}

func validCoordinates(lat, lng float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lng) && !math.IsInf(lat, 0) && !math.IsInf(lng, 0) &&
		lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}
