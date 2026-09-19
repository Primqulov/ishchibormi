package posting

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TelegramDownloader(bot *tg.BotAPI) func(context.Context, string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return func(ctx context.Context, id string) ([]byte, error) {
		file, err := bot.GetFile(tg.FileConfig{FileID: id})
		if err != nil || file.FileSize > 8<<20 {
			return nil, errors.New("Telegram file unavailable")
		}
		req, err := http.NewRequestWithContext(ctx, "GET", file.Link(bot.Token), nil)
		if err != nil {
			return nil, errors.New("invalid Telegram file")
		}
		res, err := client.Do(req)
		// Transport errors can contain the bot token in the file URL. Never
		// propagate or log them, even when Telegram itself is unavailable.
		if err != nil {
			return nil, errors.New("Telegram download failed")
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return nil, errors.New("Telegram file unavailable")
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
		if err != nil || len(data) > 8<<20 {
			return nil, errors.New("Telegram file too large or incomplete")
		}
		return data, nil
	}
}
