package posting

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPhotoLoaderReadsOnlyConfiguredStorageAndRejectsBadDownloads(t *testing.T) {
	photo := testPNG(t, 16, 12)
	calls, redirected := 0, 0
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected++ }))
	defer outside.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "" {
			t.Error("image request leaked API credentials")
		}
		switch r.URL.Path {
		case "/uploads/elons/owner/ok.png":
			_, _ = w.Write(photo)
		case "/uploads/elons/owner/redirect.png":
			http.Redirect(w, r, outside.URL+"/photo.png", http.StatusFound)
		case "/uploads/elons/owner/oversize.png":
			w.Header().Set("Content-Length", "9000000")
		case "/uploads/elons/owner/html.png":
			_, _ = io.WriteString(w, "<html>not an image</html>")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	loader, err := NewJobPhotoLoader(server.URL + "/uploads")
	if err != nil {
		t.Fatal(err)
	}
	file, err := loader(context.Background(), server.URL+"/uploads/elons/owner/ok.png")
	if err != nil {
		t.Fatal(err)
	}
	data, ok := file.(tg.FileBytes)
	if !ok || len(data.Bytes) == 0 {
		t.Fatal("local image URL was not uploaded as bytes")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data.Bytes))
	if err != nil || format != "jpeg" || cfg.Width != 16 || cfg.Height != 12 {
		t.Fatal("image was not converted to Telegram photo format", err)
	}
	for _, raw := range []string{
		outside.URL + "/uploads/elons/owner/ok.png",
		server.URL + "/uploads/avatars/owner/ok.png",
		server.URL + "/uploads/elons/../secret.png",
		server.URL + "/uploads/elons/%2e%2e/secret.png",
		server.URL + "/uploads/elons/owner/ok.png?redirect=secret",
		"file:///uploads/elons/owner/ok.png",
	} {
		if _, err := loader(context.Background(), raw); err == nil {
			t.Fatal("untrusted photo URL accepted", raw)
		}
	}
	if calls != 1 || redirected != 0 {
		t.Fatal("untrusted URL was fetched")
	}
	for _, name := range []string{"redirect", "oversize", "html", "missing"} {
		if _, err := loader(context.Background(), server.URL+"/uploads/elons/owner/"+name+".png"); err == nil {
			t.Fatal("bad download accepted", name)
		}
	}
	if redirected != 0 {
		t.Fatal("image redirect was followed")
	}
}

func TestTelegramPhotoAcceptsWebPAndBoundsUnusualDimensions(t *testing.T) {
	// A 2x2 solid-color, lossless WebP fixture, generated for this test.
	webp, err := base64.StdEncoding.DecodeString("UklGRh4AAABXRUJQVlA4TBEAAAAvAUAAAAdQrbaUsf+BiOh/AAA=")
	if err != nil {
		t.Fatal(err)
	}
	for _, original := range [][]byte{webp, testPNG(t, 3000, 10), testPNG(t, 10, 3000)} {
		data, err := telegramPhoto(original)
		if err != nil {
			t.Fatal(err)
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || format != "jpeg" || max(cfg.Width, cfg.Height) > 2048 || cfg.Width > cfg.Height*20 || cfg.Height > cfg.Width*20 {
			t.Fatalf("photo exceeds Telegram limits: %+v, %s, %v", cfg, format, err)
		}
	}
	if _, err := telegramPhoto(testPNG(t, 12001, 1)); err == nil {
		t.Fatal("oversize dimensions accepted")
	}
}

func TestJobAlbumUploadsOneMultipartMediaGroup(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "telegram_error"}[reject], func(t *testing.T) {
			photo := testPNG(t, 16, 12)
			storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(photo) }))
			defer storage.Close()
			groupCalls, photoCalls, textCalls := 0, 0, 0
			telegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/getMe"):
					_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"username":"testbot"}}`)
				case strings.HasSuffix(r.URL.Path, "/sendMediaGroup"):
					groupCalls++
					if err := r.ParseMultipartForm(2 << 20); err != nil {
						t.Error("missing multipart album", err)
						return
					}
					defer r.MultipartForm.RemoveAll()
					var media []struct {
						Type, Media, Caption string
					}
					if err := json.Unmarshal([]byte(r.FormValue("media")), &media); err != nil || len(media) != 2 {
						t.Error("invalid Telegram media payload", err)
						return
					}
					if r.FormValue("chat_id") != "42" || len(r.MultipartForm.File) != 2 || media[0].Caption == "" || media[1].Caption != "" {
						t.Error("wrong album destination, files or shared caption")
					}
					for _, item := range media {
						if item.Type != "photo" || !strings.HasPrefix(item.Media, "attach://") || len(r.MultipartForm.File[strings.TrimPrefix(item.Media, "attach://")]) != 1 {
							t.Error("album item does not reference an uploaded image")
						}
					}
					if reject {
						w.WriteHeader(http.StatusBadRequest)
						_, _ = io.WriteString(w, `{"ok":false,"error_code":400,"description":"test rejection"}`)
						return
					}
					_, _ = io.WriteString(w, `{"ok":true,"result":[{"message_id":10},{"message_id":11}]}`)
				case strings.HasSuffix(r.URL.Path, "/sendPhoto"):
					photoCalls++
					_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":12}}`)
				case strings.HasSuffix(r.URL.Path, "/sendMessage"):
					textCalls++
					_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":13}}`)
				default:
					_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":14}}`)
				}
			}))
			defer telegram.Close()
			bot, err := tg.NewBotAPIWithClient("test-token", telegram.URL+"/bot%s/%s", telegram.Client())
			if err != nil {
				t.Fatal(err)
			}
			h := workerSetup(t)
			h.e.Bot = bot
			h.e.LoadPhoto, err = NewJobPhotoLoader(storage.URL + "/uploads")
			if err != nil {
				t.Fatal(err)
			}
			h.a.job.Images = []string{storage.URL + "/uploads/elons/owner/1.png", storage.URL + "/uploads/elons/owner/2.png"}
			err = h.e.showWorkerJob(context.Background(), 42, h.a.job)
			if (err != nil) != reject || groupCalls != 1 || photoCalls != 0 {
				t.Fatal("album failed or sent duplicate/individual photos", err)
			}
			if (!reject && textCalls != 1) || (reject && textCalls != 0) {
				t.Fatal("details were sent separately or a rejected album was retried")
			}
		})
	}
}
