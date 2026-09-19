package posting

import (
	"context"
	"errors"
	"strings"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const photoCaptionLimit = 1024

func textUnits(text string) int {
	units := 0
	for _, r := range text {
		units++
		if r > 0xffff {
			units++
		}
	}
	return units
}

func captionField(text string, limit int) string {
	text = strings.TrimSpace(text)
	if textUnits(text) <= limit {
		return text
	}
	return strings.TrimSpace(textChunks(text, limit-1)[0]) + "…"
}

// Telegram permits 1024 UTF-16 units in a photo caption. Keep the contact,
// schedule and current capacity in the caption; offer full text on demand.
func workerJobCaption(j Job, now time.Time) (string, bool) {
	full := workerJobText(j, now)
	if textUnits(full) <= photoCaptionLimit {
		return full, false
	}
	j.Title = captionField(j.Title, 128)
	j.CategoryName = captionField(j.CategoryName, 40)
	j.Region = captionField(j.Region, 48)
	j.District = captionField(j.District, 48)
	j.LocationText = captionField(j.LocationText, 60)
	j.OwnerName = captionField(j.OwnerName, 40)
	j.ContactPhone = captionField(j.ContactPhone, 32)
	j.WorkTimeTo = captionField(j.WorkTimeTo, 5)
	base := workerJobSummary(j, now)
	const footer = "\n\nTo'liq tavsif — pastdagi tugmada."
	available := photoCaptionLimit - textUnits(base) - textUnits(footer) - 2
	if available > 1 {
		base += "\n\n" + captionField(j.Description, available)
	}
	return base + footer, true
}

func (e *Engine) jobPhotos(ctx context.Context, urls []string) ([]tg.RequestFileData, error) {
	if len(urls) == 0 {
		return nil, nil
	}
	if e.LoadPhoto == nil || len(urls) > 6 {
		return nil, errors.New("listing photos unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	photos := make([]tg.RequestFileData, 0, len(urls))
	for _, raw := range urls {
		photo, err := e.LoadPhoto(ctx, raw)
		if err != nil || photo == nil {
			// Prepare the whole album before sending so a failed image never
			// produces a partial album or a sequence of individual photos.
			return nil, errors.New("listing photo unavailable")
		}
		photos = append(photos, photo)
	}
	return photos, nil
}

func (e *Engine) sendJobPhotos(chat int64, photos []tg.RequestFileData, caption string) error {
	if len(photos) == 1 {
		photo := tg.NewPhoto(chat, photos[0])
		photo.Caption = caption
		_, err := e.Bot.Send(photo)
		return err
	}
	media := make([]interface{}, 0, len(photos))
	for i, file := range photos {
		photo := tg.NewInputMediaPhoto(file)
		if i == 0 {
			// Only the first item has a caption: Telegram displays it once
			// below the entire album, rather than on each individual image.
			photo.Caption = caption
		}
		media = append(media, photo)
	}
	// Send assumes a single Message response; media groups return an array.
	_, err := e.Bot.Request(tg.NewMediaGroup(chat, media))
	return err
}
