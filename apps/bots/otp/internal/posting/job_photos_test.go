package posting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func photoSetup(t *testing.T, count int) *harness {
	t.Helper()
	h := workerSetup(t)
	h.a.job.Description = "Ish joyini tayyorlash va yuklarni tushirish."
	for i := 0; i < count; i++ {
		h.a.job.Images = append(h.a.job.Images, fmt.Sprintf("photo-%d", i))
	}
	h.e.LoadPhoto = func(_ context.Context, raw string) (tg.RequestFileData, error) { return tg.FileID(raw), nil }
	return h
}

func photoGroups(b *fakeBot) []tg.MediaGroupConfig {
	var groups []tg.MediaGroupConfig
	for _, raw := range b.requests {
		if group, ok := raw.(tg.MediaGroupConfig); ok {
			groups = append(groups, group)
		}
	}
	return groups
}

func TestJobPhotosShareOneCaptionAndKeepOrder(t *testing.T) {
	for _, count := range []int{1, 2, 6} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			h := photoSetup(t, count)
			h.text("/start job_" + workerJobID)
			var singles []tg.PhotoConfig
			for _, raw := range h.b.messages {
				if photo, ok := raw.(tg.PhotoConfig); ok {
					singles = append(singles, photo)
				}
			}
			groups := photoGroups(h.b)
			caption := ""
			if count == 1 {
				if len(singles) != 1 || len(groups) != 0 || singles[0].File != tg.FileID("photo-0") || singles[0].ParseMode != "" {
					t.Fatal("one photo must use sendPhoto with plain caption")
				}
				caption = singles[0].Caption
			} else {
				if len(singles) != 0 || len(groups) != 1 || len(groups[0].Media) != count || groups[0].ChatID != 42 {
					t.Fatal("photos were not sent in exactly one album")
				}
				for i, raw := range groups[0].Media {
					photo, ok := raw.(tg.InputMediaPhoto)
					if !ok || photo.Media != tg.FileID(fmt.Sprintf("photo-%d", i)) || photo.ParseMode != "" {
						t.Fatal("album order/type changed")
					}
					if i == 0 {
						caption = photo.Caption
					} else if photo.Caption != "" {
						t.Fatal("caption repeated under individual photos")
					}
				}
			}
			for _, text := range []string{h.a.job.Title, h.a.job.Description, h.a.job.ContactPhone, "Bo'sh o'rin: 3", "Qabul qilingan: 1"} {
				if !strings.Contains(caption, text) || h.hasText(text) {
					t.Fatalf("detail must appear in the photo caption only: %s", text)
				}
			}
			if h.a.jobCalls != 1 || h.a.logins != 0 || h.a.applyCalls != 0 || !hasCallback(h.b.messages, "w:apply:"+workerJobID) {
				t.Fatal("photo view changed public details/application flow")
			}
		})
	}
}

func TestJobAlbumRefreshUsesCurrentPhotosAndVacancies(t *testing.T) {
	h := photoSetup(t, 2)
	h.text("/start job_" + workerJobID)
	h.a.job.Images[0] = "replacement"
	h.a.job.AcceptedCount, h.a.job.Status = 4, "filled"
	previousMessages := len(h.b.messages)
	h.workerClick("job", workerJobID)
	groups := photoGroups(h.b)
	if len(groups) != 2 {
		t.Fatal("refresh did not send the updated album")
	}
	photo := groups[1].Media[0].(tg.InputMediaPhoto)
	if photo.Media != tg.FileID("replacement") || !strings.Contains(photo.Caption, "Bo'sh o'rin: 0") || !strings.Contains(photo.Caption, "ariza qabul qilmayapti") {
		t.Fatal("refreshed album used stale listing details")
	}
	if hasCallback(h.b.messages[previousMessages:], "w:apply:"+workerJobID) {
		t.Fatal("filled job still offers application")
	}
}

func TestLongPhotoCaptionKeepsContactAndOffersCompleteTextOnDemand(t *testing.T) {
	h := photoSetup(t, 2)
	h.a.job.Title = strings.Repeat("🧱", 160)
	h.a.job.OwnerName = strings.Repeat("👤", 150)
	h.a.job.Description = strings.Repeat("🛠 Ish tavsifi <b>o'zgarmasin</b>. ", 200)
	h.text("/start job_" + workerJobID)
	caption := photoGroups(h.b)[0].Media[0].(tg.InputMediaPhoto).Caption
	if len(utf16.Encode([]rune(caption))) > 1024 || !utf8.ValidString(caption) || !strings.Contains(caption, h.a.job.ContactPhone) || !strings.Contains(caption, "Bo'sh o'rin: 3") {
		t.Fatal("caption is invalid or lost essential information")
	}
	if !hasCallback(h.b.messages, "w:jobtext:"+workerJobID) || h.hasText(h.a.job.Description) {
		t.Fatal("long description was sent separately without asking")
	}
	// Full-text clicks refetch the same public job and preserve every character.
	h.a.job.Description += " YANGI TAVSIF"
	before := len(h.b.messages)
	h.workerClick("jobtext", workerJobID)
	var full strings.Builder
	for _, raw := range h.b.messages[before:] {
		if msg, ok := raw.(tg.MessageConfig); ok {
			full.WriteString(msg.Text)
		}
	}
	if !strings.Contains(full.String(), h.a.job.Description) || h.a.jobCalls != 2 || len(photoGroups(h.b)) != 1 {
		t.Fatal("full text missing, stale, or resent the album")
	}
	// A photo/full-text link never grants access to a now-hidden listing.
	h.a.workerErr = &APIError{Status: 404, Code: "not_found"}
	before = len(h.b.messages)
	h.workerClick("jobtext", workerJobID)
	if len(h.b.messages[before:]) != 1 || !h.hasText("yopilgan yoki endi mavjud emas") {
		t.Fatal("hidden listing full text leaked")
	}
}

func TestPhotoDownloadFailureDoesNotSendPartialAlbum(t *testing.T) {
	h := photoSetup(t, 3)
	h.e.LoadPhoto = func(_ context.Context, raw string) (tg.RequestFileData, error) {
		if raw == "photo-1" {
			return nil, errors.New("private network error")
		}
		return tg.FileID(raw), nil
	}
	h.text("/start job_" + workerJobID)
	if len(photoGroups(h.b)) != 0 || !h.hasText("Rasmlarni hozir yuklab bo'lmadi") || !h.hasText(h.a.job.ContactPhone) || h.hasText("private network error") {
		t.Fatal("failed photo lost details or sent partial media")
	}
	for _, raw := range h.b.messages {
		if _, ok := raw.(tg.PhotoConfig); ok {
			t.Fatal("failed album fell back to individual photos")
		}
	}
}
