package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func req(header, ua string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	if header != "" {
		r.Header.Set(ClientPlatformHeader, header)
	}
	if ua != "" {
		r.Header.Set("User-Agent", ua)
	}
	return r
}

func TestClientPlatformFromHeader(t *testing.T) {
	cases := []struct {
		name, header, want string
	}{
		{"web", "web", PlatformWeb},
		{"android", "android", PlatformAndroid},
		{"ios", "ios", PlatformIOS},
		{"telegram", "telegram", PlatformTelegram},
		// Klient noto'g'ri registr yoki ortiqcha bo'shliq yuborsa ham bitta
		// guruhga tushishi kerak — aks holda hisobotda takroriy ustun paydo
		// bo'lardi.
		{"katta harf", "Android", PlatformAndroid},
		{"bo'shliq bilan", "  web  ", PlatformWeb},
		// Ro'yxatdan tashqari qiymat — jimgina saqlanib qolmasin.
		{"notanish", "windows-phone", ""},
		{"bo'sh", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClientPlatform(req(c.header, "")); got != c.want {
				t.Fatalf("ClientPlatform(%q) = %q, kutilgan %q", c.header, got, c.want)
			}
		})
	}
}

func TestClientPlatformUserAgentFallback(t *testing.T) {
	// Sarlavhani yubormaydigan ESKI veb-klient hali ham brauzer sifatida
	// tanilishi kerak: aks holda mavjud foydalanuvchilarning hammasi
	// "noma'lum" bo'lib qolardi.
	browser := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	if got := ClientPlatform(req("", browser)); got != PlatformWeb {
		t.Fatalf("brauzer UA = %q, kutilgan %q", got, PlatformWeb)
	}

	// Mobil ilova UA'si qaysi OS ekanini AYTMAYDI — bu yerda taxmin
	// qilmaslik kerak, aks holda iOS foydalanuvchilari Android bo'lib
	// sanalardi.
	if got := ClientPlatform(req("", "Dart/3.11 (dart:io)")); got != "" {
		t.Fatalf("Dart UA = %q, kutilgan bo'sh satr", got)
	}
	if got := ClientPlatform(req("", "curl/8.4.0")); got != "" {
		t.Fatalf("curl UA = %q, kutilgan bo'sh satr", got)
	}
}

func TestHeaderBeatsUserAgent(t *testing.T) {
	// Sarlavha aniq aytilgan joyda UA taxminiga o'tilmasligi kerak.
	// Amaliy holat: mobil ilovadagi WebView brauzer UA'sini yuboradi.
	got := ClientPlatform(req("android", "Mozilla/5.0 (Linux; Android 14)"))
	if got != PlatformAndroid {
		t.Fatalf("got %q, kutilgan %q", got, PlatformAndroid)
	}
}

func TestClientDevice(t *testing.T) {
	cases := []struct {
		name, header, ua, want string
	}{
		// Asosiy holatlar: brauzer UA'si OS tokenini o'zi qo'yadi.
		{
			"android chrome", "web",
			"Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Mobile Safari/537.36",
			DeviceAndroid,
		},
		{
			"android firefox", "web",
			"Mozilla/5.0 (Android 14; Mobile; rv:127.0) Gecko/127.0 Firefox/127.0",
			DeviceAndroid,
		},
		{
			"iphone safari", "web",
			"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
			DeviceIOS,
		},
		// iOS'dagi Chrome ham WebKit ustida ishlaydi va "iPhone" tokenini
		// saqlaydi — u Android deb sanalib qolmasligi kerak.
		{
			"iphone chrome (CriOS)", "web",
			"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/126.0 Mobile/15E148 Safari/604.1",
			DeviceIOS,
		},
		{
			"ipad", "web",
			"Mozilla/5.0 (iPad; CPU OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Safari/604.1",
			DeviceIOS,
		},
		{
			"windows", "web",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36",
			DeviceWindows,
		},
		{
			"macos", "web",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36",
			DeviceMacOS,
		},
		{
			"linux", "web",
			"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36",
			DeviceLinux,
		},
		{
			"ubuntu firefox", "web",
			"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:127.0) Gecko/20100101 Firefox/127.0",
			DeviceLinux,
		},
		// ChromeOS UA'sida "X11" bor, "Linux" esa YO'Q — agar CrOS tekshiruvi
		// X11'dan keyinga tushib qolsa, bu holat "linux" bo'lib ketadi.
		{
			"chromeos", "web",
			"Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36",
			DeviceChromeOS,
		},
		// Android UA'sida "Linux" ham bor — tartib buzilsa "linux" chiqadi.
		// Yuqoridagi "android chrome" holati bilan birga tartibni qo'riqlaydi.
		{
			"android — linux tokeni chalg'itmasin", "web",
			"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Mobile Safari/537.36",
			DeviceAndroid,
		},
		// Sarlavhada "web", lekin UA brauzernikiga o'xshamaydi (skript,
		// bot, sinov quroli). Taxmin qilmaymiz — bo'sh qiymat saqlanmaydi.
		{"web sarlavhasi, curl UA", "web", "curl/8.4.0", ""},
		{"web sarlavhasi, UA yo'q", "web", "", ""},
		// Brauzer, lekin OS tokeni tanish emas — "desktop" deb umumlashtirmaymiz.
		{"web, notanish OS", "web", "Mozilla/5.0 (FreeBSD amd64) AppleWebKit/537.36", ""},

		// MOBIL ILOVA: qurilma o'lchovi u yerda ma'nosiz, chunki
		// platformaning O'ZI qurilma OS'i. Ilova WebView orqali brauzer
		// UA'sini yuborsa ham qurilma yozilmasligi kerak — aks holda
		// hisobotda "android + android" ikki marta sanalardi.
		{
			"android ilovasi", "android",
			"Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36",
			"",
		},
		{"ios ilovasi", "ios", "Dart/3.11 (dart:io)", ""},

		// TELEGRAM: platformaning o'zi qurilma haqida hech nima demaydi,
		// shuning uchun veb kabi OS aniqlanadi -> panelda "Telegram iOS".
		{
			"mini app iphone", "telegram",
			"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15",
			DeviceIOS,
		},
		{
			"mini app android", "telegram",
			"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 Chrome/126.0 Mobile Safari/537.36",
			DeviceAndroid,
		},
		// Chatdagi bot suhbati: so'rov Go klientidan keladi, brauzer UA'si
		// yo'q. Qurilma bo'sh qolishi TO'G'RI javob — taxmin qilinmaydi.
		{"bot suhbati (Go klienti)", "telegram", "Go-http-client/2.0", ""},
		{"bot suhbati, UA yo'q", "telegram", "", ""},

		// Klient umuman tanitmagan — platforma ham bo'sh, qurilma ham.
		{"noma'lum klient", "", "Dart/3.11 (dart:io)", ""},

		// Sarlavhasiz eski VEB klient: platforma UA'dan "web" bo'lib
		// aniqlanadi, demak qurilma ham aniqlanishi kerak.
		{
			"sarlavhasiz eski veb", "",
			"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15",
			DeviceIOS,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClientDevice(req(c.header, c.ua)); got != c.want {
				t.Fatalf("ClientDevice(%q, %q) = %q, kutilgan %q", c.header, c.ua, got, c.want)
			}
		})
	}
}

// Hisobotlar httpx.Platforms bo'ylab yuriladi: ro'yxatdan tushib qolgan
// platforma admin panelida umuman ko'rinmaydi va "noma'lum" ga qo'shilib
// ketadi. Shuning uchun har bir qiymat shu ro'yxatda bo'lishi shart.
func TestEveryKnownPlatformIsReported(t *testing.T) {
	for _, want := range []string{PlatformWeb, PlatformAndroid, PlatformIOS, PlatformTelegram} {
		found := false
		for _, p := range Platforms {
			if p == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%q hisobot ro'yxatida yo'q", want)
		}
	}
}

func TestPlatformOrUnknown(t *testing.T) {
	if got := PlatformOrUnknown(""); got != PlatformUnknown {
		t.Fatalf("bo'sh -> %q, kutilgan %q", got, PlatformUnknown)
	}
	if got := PlatformOrUnknown(PlatformWeb); got != PlatformWeb {
		t.Fatalf("web -> %q, o'zgarmasligi kerak", got)
	}
}
