package posting

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var tashkent = time.FixedZone("Asia/Tashkent", 5*60*60)
var phonePattern = regexp.MustCompile(`^\+998[0-9]{9}$`)
var digitsPattern = regexp.MustCompile(`^[0-9]+$`)

type Form struct {
	Title         string   `json:"title"`
	CategoryID    string   `json:"categoryId"`
	Description   string   `json:"description"`
	Lat           float64  `json:"lat"`
	Lng           float64  `json:"lng"`
	WorkersNeeded int      `json:"workersNeeded"`
	Gender        string   `json:"gender"`
	PricingType   string   `json:"pricingType"`
	PriceAmount   int64    `json:"priceAmount"`
	StartDate     string   `json:"startDate"`
	WorkTimeFrom  string   `json:"workTimeFrom"`
	ContactPhone  string   `json:"contactPhone"`
	Images        []string `json:"images"`
}
type Draft struct {
	ChatID         int64 `bson:"_id"`
	Key            string
	Revision       int
	LastUpdate     int
	Step           string
	Editing        bool
	UserID         string
	Profile        Profile
	Form           Form
	CategoryName   string
	Categories     []Category
	CategoryPage   int
	HasLocation    bool
	PhotoIDs       []string
	PhotoFileIDs   []string
	PhotoDocuments []bool
	PublishedID    string
	Worker         *WorkerFlow
	// Registered — hisob borligi bir marta tasdiqlangan. Ro'yxatdan o'tish
	// taklifini takror chiqarmaslik va bosh menyu har ochilganda sessiya
	// so'rovi yubormaslik uchun. FAQAT ijobiy natija keshlanadi: hisobi
	// yo'q odam keyingi daqiqada ochishi mumkin.
	Registered bool
	UpdatedAt  time.Time `bson:"updatedAt"`

	// Oxirgi muvaffaqiyatli /jobs qidiruvining joyi. Xotiradagi searchState
	// bir soatda o'chadi, bu esa qoladi — takroriy qidiruvda foydalanuvchidan
	// lokatsiya qayta so'ralmaydi (search.go: rememberSearchLocation).
	LastSearchLat          float64
	LastSearchLng          float64
	LastSearchPlace        string
	LastSearchCategoryID   string
	LastSearchCategoryName string
	HasLastSearch          bool

	// Saytdagi «Telegram orqali kirish» tokeni, kontakt ulashish kutilayotgan
	// paytda. Ilgari faqat bot xotirasida turardi: /start bilan kontakt
	// orasida bot qayta ishga tushsa token yo'qolib, foydalanuvchi ishlamaydigan
	// holatga tushardi (cmd/bot/main.go).
	AuthToken   string
	AuthTokenAt time.Time
}

// lastSearchLabel — oxirgi qidiruv joyining foydalanuvchiga ko'rsatiladigan
// nomi. Aniq lokatsiyaning nomi yo'q, shuning uchun umumiy matn qaytadi.
func (d *Draft) lastSearchLabel() string {
	if d == nil || !d.HasLastSearch {
		return ""
	}
	if d.LastSearchPlace != "" {
		return d.LastSearchPlace
	}
	return "xaritada yuborilgan nuqta"
}

type Store interface {
	Load(context.Context, int64) (*Draft, error)
	Save(context.Context, *Draft) error
}
type MongoStore struct{ Col *mongo.Collection }

func (s MongoStore) Load(ctx context.Context, id int64) (*Draft, error) {
	var d Draft
	err := s.Col.FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &d, err
}
func (s MongoStore) Save(ctx context.Context, d *Draft) error {
	d.UpdatedAt = time.Now()
	_, err := s.Col.ReplaceOne(ctx, bson.M{"_id": d.ChatID}, d, options.Replace().SetUpsert(true))
	return err
}
func freshDraft(chatID int64) *Draft {
	return &Draft{ChatID: chatID, Key: primitive.NewObjectID().Hex(), Step: "consent", Form: Form{WorkersNeeded: 1, Images: []string{}}}
}

var steps = []string{"title", "category", "description", "gender", "workers", "pricing", "amount", "date", "time", "location", "phone", "photos", "preview"}
var labels = map[string]string{"title": "Ish nomi", "category": "Kategoriya", "description": "Tavsif", "gender": "Kim kerak", "workers": "Ishchilar soni", "pricing": "To'lov turi", "amount": "Ish haqi", "date": "Sana", "time": "Vaqt", "location": "Lokatsiya", "phone": "Aloqa raqami", "photos": "Rasmlar"}

func stepIndex(step string) int {
	for i, s := range steps {
		if s == step {
			return i
		}
	}
	return -1
}
func (d *Draft) next() {
	if d.Editing {
		if d.Step == "pricing" && d.Form.PricingType != "negotiable" {
			d.Step = "amount"
			return
		}
		d.Editing = false
		d.Step = "preview"
		return
	}
	i := stepIndex(d.Step)
	if i >= 0 && i+1 < len(steps) {
		d.Step = steps[i+1]
	}
	if d.Step == "amount" && d.Form.PricingType == "negotiable" {
		d.Step = "date"
	}
}
func normalizePhone(text string) string {
	text = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(strings.TrimSpace(text))
	if len(text) == 9 {
		text = "+998" + text
	} else if strings.HasPrefix(text, "998") {
		text = "+" + text
	}
	return text
}
func number(text string, max int64) (int64, error) {
	t := strings.ReplaceAll(strings.TrimSpace(text), " ", "")
	if !digitsPattern.MatchString(t) {
		return 0, errors.New("Faqat butun son kiriting.")
	}
	n, err := strconv.ParseInt(t, 10, 64)
	if err != nil || n > max {
		return 0, fmt.Errorf("Qiymat 0 dan %d gacha bo'lishi kerak.", max)
	}
	return n, nil
}
func validText(text string, max int) bool {
	return strings.TrimSpace(text) != "" && utf8.RuneCountInString(text) <= max
}
func validCoordinates(lat, lng float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lng) && !math.IsInf(lat, 0) && !math.IsInf(lng, 0) && lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}
func validDate(date string, now time.Time) bool {
	d, err := time.ParseInLocation("2006-01-02", date, tashkent)
	n := now.In(tashkent)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, tashkent)
	return err == nil && !d.Before(today) && !d.After(today.AddDate(0, 0, 2))
}
func validStart(date, clock string, now time.Time) bool {
	d, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, tashkent)
	return err == nil && len(clock) == 5 && validDate(date, now) && !d.Before(now.Truncate(time.Minute).Add(time.Hour))
}
func (d *Draft) invalidField(now time.Time) string {
	f := d.Form
	if !validText(f.Title, 160) {
		return "title"
	}
	if f.CategoryID == "" {
		return "category"
	}
	if !validText(f.Description, 5000) {
		return "description"
	}
	if f.Gender != "male" && f.Gender != "female" && f.Gender != "mixed" {
		return "gender"
	}
	if f.WorkersNeeded < 1 || f.WorkersNeeded > 100 {
		return "workers"
	}
	if f.PricingType != "per_worker" && f.PricingType != "total" && f.PricingType != "negotiable" {
		return "pricing"
	}
	if f.PriceAmount < 0 || f.PriceAmount > 1_000_000_000_000 {
		return "amount"
	}
	if !validDate(f.StartDate, now) {
		return "date"
	}
	if !validStart(f.StartDate, f.WorkTimeFrom, now) {
		return "time"
	}
	if !d.HasLocation || !validCoordinates(f.Lat, f.Lng) {
		return "location"
	}
	if !phonePattern.MatchString(f.ContactPhone) {
		return "phone"
	}
	if len(f.Images) > 6 {
		return "photos"
	}
	return ""
}
func priceSummary(f Form) string {
	if f.PricingType == "negotiable" || f.PriceAmount == 0 {
		return "Kelishiladi"
	}
	per, total := f.PriceAmount, f.PriceAmount*int64(f.WorkersNeeded)
	if f.PricingType == "total" {
		total = f.PriceAmount
		if f.WorkersNeeded > 0 {
			per = total / int64(f.WorkersNeeded)
		}
	}
	return fmt.Sprintf("Har bir ishchiga: %d so'm\nJami: %d so'm", per, total)
}
