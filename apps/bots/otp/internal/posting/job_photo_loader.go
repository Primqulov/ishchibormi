package posting

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const maxJobPhotoBytes = 8 << 20

// Download from configured application storage, then upload bytes to Telegram.
// This also works when the API's image URLs are only reachable locally.
func NewJobPhotoLoader(storageBases ...string) (func(context.Context, string) (tg.RequestFileData, error), error) {
	var bases []*url.URL
	for _, raw := range storageBases {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		u, err := url.Parse(strings.TrimRight(raw, "/") + "/elons/")
		if err != nil || !validPhotoURL(u) {
			return nil, errors.New("invalid listing photo storage URL")
		}
		bases = append(bases, u)
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		// Never follow an image redirect outside configured storage.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return func(ctx context.Context, raw string) (tg.RequestFileData, error) {
		u, err := url.Parse(raw)
		allowed := false
		if err == nil && validPhotoURL(u) && path.Clean(u.Path) == u.Path {
			for _, base := range bases {
				if u.Scheme == base.Scheme && strings.EqualFold(u.Host, base.Host) && strings.HasPrefix(u.Path, base.Path) {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			return nil, errors.New("listing photo is outside configured storage")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, errors.New("invalid listing photo request")
		}
		res, err := client.Do(req)
		if err != nil {
			return nil, errors.New("listing photo download failed")
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK || res.ContentLength > maxJobPhotoBytes {
			return nil, errors.New("listing photo unavailable or too large")
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, maxJobPhotoBytes+1))
		if err != nil || len(data) > maxJobPhotoBytes {
			return nil, errors.New("listing photo too large or incomplete")
		}
		data, err = telegramPhoto(data)
		if err != nil {
			return nil, err
		}
		return tg.FileBytes{Name: "ish-rasmi.jpg", Bytes: data}, nil
	}, nil
}

func validPhotoURL(u *url.URL) bool {
	return u != nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil &&
		u.RawQuery == "" && u.Fragment == "" && !strings.Contains(u.Path, "\\")
}

// Convert all accepted upload formats (JPEG/PNG/WEBP) to a Telegram photo.
// Bound dimensions before decoding; shrink large images and pad extreme
// aspect ratios to satisfy Telegram's photo limits without cropping content.
func telegramPhoto(data []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 12000 || cfg.Height > 12000 || int64(cfg.Width)*int64(cfg.Height) > 50_000_000 {
		return nil, errors.New("invalid listing photo dimensions")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("invalid listing photo")
	}
	w, h := cfg.Width, cfg.Height
	if side := max(w, h); side > 2048 {
		w, h = max(1, w*2048/side), max(1, h*2048/side)
	}
	paddedW, paddedH := max(w, (h+19)/20), max(h, (w+19)/20)
	dst := image.NewRGBA(image.Rect(0, 0, paddedW, paddedH))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	x, y := (paddedW-w)/2, (paddedH-h)/2
	xdraw.ApproxBiLinear.Scale(dst, image.Rect(x, y, x+w, y+h), src, src.Bounds(), draw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, errors.New("listing photo conversion failed")
	}
	return out.Bytes(), nil
}
