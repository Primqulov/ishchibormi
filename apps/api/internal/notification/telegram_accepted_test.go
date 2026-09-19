package notification

import (
	"html"
	"math"
	"regexp"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/ishchibormi/backend/internal/models"
)

func TestAcceptedScheduleUsesListingTimeWithoutInventingMissingTime(t *testing.T) {
	for _, tc := range []struct {
		name, date, from, to, wantDate, wantTime string
	}{
		{"date and hours", "2026-09-17", "09:00", "18:00", "17-sentabr 2026, payshanba", "09:00–18:00 (Toshkent vaqti)"},
		{"embedded local time", "2026-09-17T10:30:00", "09:00", "18:00", "17-sentabr 2026, payshanba", "10:30–18:00 (Toshkent vaqti)"},
		{"overnight", "2026-09-17", "22:00", "06:00", "17-sentabr 2026, payshanba", "22:00–06:00 (Toshkent vaqti); tugashi ertasi kuni"},
		{"missing time", "2026-09-17", "", "", "17-sentabr 2026, payshanba", "Vaqt belgilanmagan — ish beruvchi bilan kelishiladi"},
		{"missing date", "", "09:00", "", "Sana belgilanmagan — ish beruvchi bilan kelishiladi", "09:00 dan (Toshkent vaqti); tugash vaqti kelishiladi"},
		{"invalid legacy values", "invalid", "99:99", "bad", "Sana belgilanmagan — ish beruvchi bilan kelishiladi", "Vaqt belgilanmagan — ish beruvchi bilan kelishiladi"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			date, hours := acceptedSchedule(models.ListingWorkDetails{StartDate: tc.date, WorkTimeFrom: tc.from, WorkTimeTo: tc.to})
			if date != tc.wantDate || hours != tc.wantTime {
				t.Fatalf("got %q / %q", date, hours)
			}
		})
	}
}

func TestAcceptedMapUsesCoordinatesOrSafeSavedURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		work models.ListingWorkDetails
		want string
	}{
		{"coordinates", models.ListingWorkDetails{Lat: 41.311081, Lng: 69.240562, LocationURL: "https://maps.example/old"}, "https://www.google.com/maps/search/?api=1&query=41.311081,69.240562"},
		{"saved URL", models.ListingWorkDetails{LocationURL: "https://maps.example/place?a=1&b=2"}, "https://maps.example/place?a=1&b=2"},
		{"unset coordinates", models.ListingWorkDetails{}, ""},
		{"out of range", models.ListingWorkDetails{Lat: 95, Lng: 200}, ""},
		{"not a number", models.ListingWorkDetails{Lat: math.NaN(), Lng: 60}, ""},
		{"script URL", models.ListingWorkDetails{LocationURL: "javascript:alert(1)"}, ""},
		{"credentials", models.ListingWorkDetails{LocationURL: "https://user:secret@maps.example/place"}, ""},
		{"localhost", models.ListingWorkDetails{LocationURL: "http://localhost:3000/map"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := acceptedMapURL(tc.work); got != tc.want {
				t.Fatalf("map URL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAcceptedDetailsEscapeHTMLAndRetainContactWithinTelegramLimit(t *testing.T) {
	n := models.Notification{Type: "application_accepted", Title: "Arizangiz qabul qilindi", Body: "Ta'mirlash <b>&", AcceptedJob: &models.AcceptedJobDetails{
		EmployerName: "Ali <admin>", ContactPhone: "+998901234567",
		Work: models.ListingWorkDetails{StartDate: "2026-09-17", WorkTimeFrom: "09:00", WorkTimeTo: "18:00", LocationText: "Ko'cha <script>&", LocationURL: "https://maps.example/place?x=1&y=2"},
	}}
	msg := telegramMessage(n)
	for _, want := range []string{"Ta'mirlash &lt;b&gt;&amp;", "Ali &lt;admin&gt;", "+998901234567", "Ko'cha &lt;script&gt;&amp;", `href="https://maps.example/place?x=1&amp;y=2"`, "17-sentabr 2026", "09:00–18:00"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q: %s", want, msg)
		}
	}
	n.Body = strings.Repeat("😀<&>", 3000)
	n.AcceptedJob.EmployerName = n.Body
	n.AcceptedJob.Work.LocationText = n.Body
	msg = telegramMessage(n)
	plain := html.UnescapeString(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(msg, ""))
	if len(utf16.Encode([]rune(plain))) > 4096 || !strings.Contains(msg, "+998901234567") || !strings.Contains(msg, "Xaritada ochish") {
		t.Fatal("long listing lost contact/map or exceeded Telegram's limit")
	}
	n.AcceptedJob = &models.AcceptedJobDetails{}
	msg = telegramMessage(n)
	for _, want := range []string{"Sana belgilanmagan", "Vaqt belgilanmagan", "Telefon raqami kiritilmagan", "Manzil matni kiritilmagan", "Xarita lokatsiyasi kiritilmagan"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing honest fallback %q", want)
		}
	}
}
