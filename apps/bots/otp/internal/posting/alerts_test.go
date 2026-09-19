package posting

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const tashkentCategoryID = "123456789012345678901234"

func (h *harness) lastReplyKeyboard() (tg.ReplyKeyboardMarkup, bool) {
	for i := len(h.b.messages) - 1; i >= 0; i-- {
		m, ok := h.b.messages[i].(tg.MessageConfig)
		if !ok {
			continue
		}
		if kb, ok := m.ReplyMarkup.(tg.ReplyKeyboardMarkup); ok {
			return kb, true
		}
	}
	return tg.ReplyKeyboardMarkup{}, false
}

func (h *harness) keyboardHas(label string) bool {
	kb, ok := h.lastReplyKeyboard()
	if !ok {
		return false
	}
	for _, row := range kb.Keyboard {
		for _, b := range row {
			if b.Text == label {
				return true
			}
		}
	}
	return false
}

// GPS bo'lmaganda ham qidiruv ishlashi kerak: Telegram Desktop lokatsiya
// yubora olmaydi, ya'ni bu yo'lsiz kompyuterdagi foydalanuvchi uchun /jobs
// butunlay yopiq edi.
func TestRegionSearchWorksWithoutLocationAndIsRemembered(t *testing.T) {
	h := setup(t)
	h.a.nearbyResult = searchResult()
	h.text("/jobs")
	if !h.keyboardHas(btnRegionSearch) {
		t.Fatal("viloyat tugmasi taklif qilinmadi")
	}
	h.text(btnRegionSearch)
	s, ok := h.e.Searches.get(42, h.e.now())
	if !ok {
		t.Fatal("qidiruv holati yo'qoldi")
	}
	h.update++
	h.rawClick(callbackRegion(s.Key, 0))
	if h.a.nearbyCalls != 1 || math.Abs(h.a.nearbyLat-41.3111) > 0.001 || math.Abs(h.a.nearbyLng-69.2797) > 0.001 {
		t.Fatalf("viloyat markazi qidiruvga yetmadi: %v %v", h.a.nearbyLat, h.a.nearbyLng)
	}
	if !h.s.draft.HasLastSearch || !strings.Contains(h.s.draft.LastSearchPlace, "Toshkent shahri") {
		t.Fatalf("joylashuv eslab qolinmadi: %+v", h.s.draft)
	}
	if !h.hasText("Qidiruv nuqtasi: Toshkent shahri") {
		t.Fatal("foydalanuvchiga qaysi nuqtadan qidirilgani aytilmadi")
	}
}

// Takroriy qidiruvda lokatsiya qayta so'ralmasligi kerak — eng ko'p tashlab
// ketiladigan qadam shu edi.
func TestLastLocationButtonSearchesWithoutAskingAgain(t *testing.T) {
	h := setup(t)
	h.a.nearbyResult = searchResult()
	h.text("/jobs")
	h.location(41.35, 69.21)
	h.text("/jobs")
	if !h.keyboardHas(btnLastLocation) {
		t.Fatal("oxirgi joy tugmasi taklif qilinmadi")
	}
	h.text(btnLastLocation)
	if h.a.nearbyCalls != 2 || h.a.nearbyLat != 41.35 || h.a.nearbyLng != 69.21 {
		t.Fatalf("saqlangan joylashuv ishlatilmadi: %d ta chaqiruv, %v %v", h.a.nearbyCalls, h.a.nearbyLat, h.a.nearbyLng)
	}
}

// Signal joriy qidiruv hududini oladi: foydalanuvchidan hech narsa
// so'ralmaydi, shuning uchun bu bitta tugma bosishdan iborat.
func TestJobAlertUsesCurrentSearchAreaAndCategory(t *testing.T) {
	h := workerSetup(t)
	h.a.nearbyResult = searchResult()
	h.text("/jobs")
	h.location(41.3, 69.2)
	s, _ := h.e.Searches.get(42, h.e.now())
	h.update++
	h.rawClick("jobs:" + s.Key + ":category:" + tashkentCategoryID)
	h.workerClick("alert", "on")
	if h.a.alertSaves != 1 || h.a.alert == nil {
		t.Fatalf("signal saqlanmadi: %d ta yozuv", h.a.alertSaves)
	}
	if h.a.alert.Lat != 41.3 || h.a.alert.Lng != 69.2 || h.a.alert.RadiusM != alertRadiusM {
		t.Fatalf("noto'g'ri hudud: %+v", h.a.alert)
	}
	if h.a.alert.CategoryID != tashkentCategoryID {
		t.Fatalf("ish turi filtri yo'qoldi: %+v", h.a.alert)
	}
	if !h.hasText("Ish signali yoqildi") || !h.hasText("Qurilish") {
		t.Fatal("tasdiq xabari signal shartlarini ko'rsatmadi")
	}
}

// Hudud ma'lum bo'lmasa signal yozilmasligi kerak: aks holda foydalanuvchi
// "yoqdim" deb o'ylab, hech qachon xabar olmasdi.
func TestJobAlertWithoutAreaAsksForSearchFirst(t *testing.T) {
	h := workerSetup(t)
	h.workerClick("alert", "on")
	if h.a.alertSaves != 0 {
		t.Fatal("hududsiz signal saqlandi")
	}
	if !h.hasText("avval hududni belgilang") {
		t.Fatal("nima qilish kerakligi tushuntirilmadi")
	}
}

func TestAlertsCommandShowsStatusAndOffDeletes(t *testing.T) {
	h := workerSetup(t)
	h.text("/alerts")
	if !h.hasText("Ish signali o'chiq") {
		t.Fatal("o'chiq holat ko'rsatilmadi")
	}
	h.a.alert = &JobAlert{Lat: 41.3, Lng: 69.2, RadiusM: alertRadiusM, PlaceName: "Toshkent shahri (Toshkent markazi)"}
	h.text("/alerts")
	if !h.hasText("Ish signali yoqilgan") || !h.hasText("Toshkent shahri") {
		t.Fatal("yoqilgan signal tafsilotlari ko'rsatilmadi")
	}
	h.workerClick("alert", "off")
	if h.a.alertDeletes != 1 || h.a.alert != nil {
		t.Fatalf("signal o'chirilmadi: %d", h.a.alertDeletes)
	}
	if !h.hasText("Ish signali o'chirildi") {
		t.Fatal("o'chirilgani tasdiqlanmadi")
	}
}

// Radius bo'sh kelsa ham matn buzilmasligi kerak (eski yozuvlar).
func TestAlertSummaryFallsBackToDefaults(t *testing.T) {
	text := alertSummary(&JobAlert{Lat: 41.3111, Lng: 69.2797})
	if !strings.Contains(text, "10 km") || !strings.Contains(text, "barcha ish turlari") || !strings.Contains(text, "41.3111") {
		t.Fatalf("standart qiymatlar ko'rsatilmadi:\n%s", text)
	}
}

// Token draft'da saqlanadi — bot /start bilan kontakt orasida qayta ishga
// tushsa ham foydalanuvchi ishlaydigan kod oladi.
func TestAuthTokenIsPersistedAndUsedOnce(t *testing.T) {
	h := setup(t)
	if err := h.e.SaveAuthToken(context.Background(), 42, "session-token"); err != nil {
		t.Fatal(err)
	}
	if got := h.e.TakeAuthToken(context.Background(), 42); got != "session-token" {
		t.Fatalf("token o'qilmadi: %q", got)
	}
	if got := h.e.TakeAuthToken(context.Background(), 42); got != "" {
		t.Fatalf("token ikkinchi marta berildi: %q", got)
	}
}

func TestExpiredAuthTokenIsDiscarded(t *testing.T) {
	h := setup(t)
	if err := h.e.SaveAuthToken(context.Background(), 42, "session-token"); err != nil {
		t.Fatal(err)
	}
	h.s.draft.AuthTokenAt = h.e.now().Add(-authTokenTTL - time.Minute)
	if got := h.e.TakeAuthToken(context.Background(), 42); got != "" {
		t.Fatalf("muddati o'tgan token ishlatildi: %q", got)
	}
	if h.s.draft.AuthToken != "" {
		t.Fatal("eskirgan token draft'da qoldi")
	}
}
