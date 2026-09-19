package notification

import (
	"fmt"
	"html"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/tgsend"
)

func telegramAcceptedMessage(n models.Notification) string {
	d := n.AcceptedJob
	date, hours := acceptedSchedule(d.Work)
	addressParts := []string{}
	for _, value := range []string{d.Work.Region, d.Work.District, d.Work.LocationText} {
		if value = strings.TrimSpace(value); value != "" {
			addressParts = append(addressParts, value)
		}
	}
	address := strings.Join(addressParts, ", ")
	if address == "" {
		address = "Manzil matni kiritilmagan. Ish beruvchidan aniqlashtiring."
	}
	phone := strings.TrimSpace(d.ContactPhone)
	if phone == "" {
		phone = "Telefon raqami kiritilmagan. E'londagi aloqa ma'lumotlarini tekshiring."
	}
	line := func(label, value string, limit int) string {
		return label + tgsend.EscapeHTML(telegramText(value, limit))
	}
	parts := []string{
		"✅ <b>Arizangiz qabul qilindi</b>",
		"Ish beruvchi sizni ushbu ishga tanladi!",
		line("💼 <b>Ish:</b> ", n.Body, 256),
		"📅 <b>Sana:</b> " + date + "\n🕘 <b>Vaqt:</b> " + hours,
	}
	contact := line("📞 <b>Telefon:</b> ", phone, 120)
	if strings.TrimSpace(d.EmployerName) != "" {
		contact = line("👤 <b>Ish beruvchi:</b> ", d.EmployerName, 120) + "\n" + contact
	}
	parts = append(parts, contact, line("📍 <b>Manzil:</b> ", address, 1000))
	if link := acceptedMapURL(d.Work); link != "" {
		parts = append(parts, `🗺 <a href="`+html.EscapeString(link)+`">Xaritada ochish</a>`)
	} else {
		parts = append(parts, "🗺 Xarita lokatsiyasi kiritilmagan. Aniq joylashuvni ish beruvchidan so'rang.")
	}
	parts = append(parts, "Ishga borishdan oldin ish beruvchi bilan bog'lanib, uchrashuv joyi va vaqtini kelishib oling.\nAriza tafsilotlari quyidagi tugmada.")
	return strings.Join(parts, "\n\n")
}

func acceptedSchedule(work models.ListingWorkDetails) (string, string) {
	date := "Sana belgilanmagan — ish beruvchi bilan kelishiladi"
	start := strings.TrimSpace(work.StartDate)
	if len(start) >= 10 {
		if day, err := time.Parse("2006-01-02", start[:10]); err == nil {
			months := [...]string{"yanvar", "fevral", "mart", "aprel", "may", "iyun", "iyul", "avgust", "sentabr", "oktabr", "noyabr", "dekabr"}
			weekdays := [...]string{"yakshanba", "dushanba", "seshanba", "chorshanba", "payshanba", "juma", "shanba"}
			date = fmt.Sprintf("%d-%s %d, %s", day.Day(), months[day.Month()-1], day.Year(), weekdays[day.Weekday()])
		}
	}
	// Match the listing scheduler's local wall-time convention: an embedded
	// startDate time precedes workTimeFrom. Never turn an unset time into 23:59.
	from := strings.TrimSpace(work.WorkTimeFrom)
	if len(start) >= 16 {
		from = start[11:16]
	}
	clock := func(raw string) string {
		if parsed, err := time.Parse("15:04", strings.TrimSpace(raw)); err == nil {
			return parsed.Format("15:04")
		}
		return ""
	}
	from, to := clock(from), clock(work.WorkTimeTo)
	hours := "Vaqt belgilanmagan — ish beruvchi bilan kelishiladi"
	switch {
	case from != "" && to != "":
		hours = from + "–" + to + " (Toshkent vaqti)"
		if to < from {
			hours += "; tugashi ertasi kuni"
		}
	case from != "":
		hours = from + " dan (Toshkent vaqti); tugash vaqti kelishiladi"
	case to != "":
		hours = to + " gacha (Toshkent vaqti); boshlanish vaqti kelishiladi"
	}
	return date, hours
}

func acceptedMapURL(work models.ListingWorkDetails) string {
	if !math.IsNaN(work.Lat) && !math.IsNaN(work.Lng) && !math.IsInf(work.Lat, 0) && !math.IsInf(work.Lng, 0) &&
		work.Lat >= -90 && work.Lat <= 90 && work.Lng >= -180 && work.Lng <= 180 && (work.Lat != 0 || work.Lng != 0) {
		return fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%.6f,%.6f", work.Lat, work.Lng)
	}
	link := strings.TrimSpace(work.LocationURL)
	if len(link) <= 2048 && (tgsend.Button{Text: "Xaritada ochish", URL: link}).Valid() {
		if parsed, err := url.Parse(link); err == nil {
			return parsed.String()
		}
	}
	return ""
}
