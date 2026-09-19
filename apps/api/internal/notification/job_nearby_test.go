package notification

import (
	"strings"
	"testing"

	"github.com/ishchibormi/backend/internal/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Signal xabari Telegramga chiqishi kerak: obuna aynan shuning uchun yoqiladi.
// Admin yozgan xabar esa hech qachon "ish hodisasi" bo'lib qolmasligi kerak.
func TestJobNearbyIsATelegramJobEvent(t *testing.T) {
	if !telegramJobEvent(models.Notification{Type: "job_nearby"}) {
		t.Fatal("job_nearby Telegramga yuborilmayapti")
	}
	fromAdmin := models.Notification{Type: "job_nearby", SentByAdminID: primitive.NewObjectID()}
	if telegramJobEvent(fromAdmin) {
		t.Fatal("admin yuborgan xabar ish hodisasi sifatida qaraldi")
	}
}

func TestJobNearbyMessageNamesTheListingAndHowToStop(t *testing.T) {
	text := telegramMessage(models.Notification{
		Type: "job_nearby", Title: "Yaqiningizda yangi ish",
		Body: "Hovli tozalash — 1.2 km uzoqlikda",
	})
	for _, want := range []string{"Hovli tozalash", "1.2 km", "hududda yangi ish e'loni", "/alerts"} {
		if !strings.Contains(text, want) {
			t.Fatalf("xabarda %q yo'q:\n%s", want, text)
		}
	}
}

// Ish beruvchi qarorni xabarning o'zidan boshlay olishi kerak. Tugmalar
// callback'i bot tushunadigan shaklda va Telegramning 64 baytlik chegarasida
// bo'lishi shart.
func TestNewApplicationCarriesDecisionButtons(t *testing.T) {
	id := primitive.NewObjectID()
	btn := telegramButton(models.Notification{
		Type: "new_application", Title: "Yangi ariza",
		RelatedEntity: &models.RelatedEntity{Type: "application", ID: id},
	}, "https://ishchibormi.uz")
	if len(btn.Actions) != 2 {
		t.Fatalf("qaror tugmalari yo'q: %+v", btn.Actions)
	}
	if btn.Actions[0].Callback != "w:accept:"+id.Hex() || btn.Actions[1].Callback != "w:reject:"+id.Hex() {
		t.Fatalf("noto'g'ri callback: %+v", btn.Actions)
	}
	for _, a := range btn.Actions {
		if len(a.Callback) > 64 {
			t.Fatalf("callback 64 baytdan uzun: %q", a.Callback)
		}
	}
}

// Qolgan xabarlarda qaror tugmasi bo'lmasligi kerak: ishchi o'z arizasini
// "qabul qila" olmaydi, e'lon xabarida esa ariza umuman yo'q.
func TestOnlyNewApplicationGetsDecisionButtons(t *testing.T) {
	id := primitive.NewObjectID()
	for _, typ := range []string{"application_submitted", "application_accepted", "job_nearby", "elon_updated"} {
		related := &models.RelatedEntity{Type: "application", ID: id}
		if typ == "job_nearby" || typ == "elon_updated" {
			related = &models.RelatedEntity{Type: "elon", ID: id}
		}
		btn := telegramButton(models.Notification{Type: typ, RelatedEntity: related}, "https://ishchibormi.uz")
		if len(btn.Actions) != 0 {
			t.Fatalf("%s uchun ortiqcha qaror tugmasi: %+v", typ, btn.Actions)
		}
	}
}
