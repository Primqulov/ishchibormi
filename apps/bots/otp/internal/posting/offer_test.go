package posting

import (
	"strings"
	"testing"
)

// Taklif bo'limi: buyruq ham, doimiy menyu tugmasi ham, bosh menyudagi inline
// tugma ham AYNI matnni ochadi va to'rtala havola ham joyida bo'ladi.
func TestOfferListsAppSiteAndBothChannels(t *testing.T) {
	h := workerSetup(t)
	h.text("/taklif")

	for _, part := range []string{"Android ilova", "ishchibormi.uz", "@Ishchibormi", "@Ishchibormi_elonlar", "o'zi tushadi"} {
		if !h.hasText(part) {
			t.Fatal("taklif matnida yo'q:", part)
		}
	}
	for label, want := range map[string]string{
		"Android ilova":        "https://play.google.com/store/apps/details?id=uz.ishchibormi.app",
		"Sayt":                 "https://ishchibormi.uz",
		"Yangiliklar kanali":   "https://t.me/Ishchibormi",
		"Ish e'lonlari kanali": "https://t.me/Ishchibormi_elonlar",
	} {
		if got := offerURL(h, label); got != want {
			t.Fatalf("%q tugmasi %q ga emas, %q ga olib boradi", label, want, got)
		}
	}
	if !hasCallback(h.b.messages, "w:home:") {
		t.Fatal("taklifdan bosh menyuga qaytish yo'li yo'q")
	}
	// Taklif — oddiy ma'lumot: u hech qanday suhbat oqimini ochib qo'ymasligi
	// va ochilgan oqimni buzmasligi kerak.
	if h.s.draft.Worker != nil {
		t.Fatal("taklif suhbat oqimini ochib yubordi")
	}
	if KeyboardCommand(btnOffer) != "taklif" {
		t.Fatal("doimiy menyu tugmasi taklifni ochmaydi")
	}
}

// Kanal havolasi matnda ham, tugmada ham bir xil manzilni ko'rsatishi kerak:
// obuna bo'lmoqchi odam @nom ni qo'lda qidirsa ham o'sha kanalga tushadi.
func TestOfferChannelNamesMatchLinks(t *testing.T) {
	for _, url := range []string{newsChannelURL, jobsChannelURL} {
		name := "@" + strings.TrimPrefix(url, "https://t.me/")
		if !strings.Contains(offerText, name) {
			t.Fatalf("matnda %s kanalining nomi yo'q", name)
		}
	}
}

// Taklif e'lon qoralamasini ham, ish qidiruvini ham buzmasligi kerak —
// u shunchaki ma'lumot ko'rsatadi.
func TestOfferPreservesPostingDraft(t *testing.T) {
	h := workerSetup(t)
	h.formToPreview("total")
	title := h.s.draft.Form.Title
	h.text("/taklif")
	if h.s.draft.Form.Title != title || h.s.draft.Step != "preview" {
		t.Fatal("taklif e'lon qoralamasini buzdi")
	}
}
