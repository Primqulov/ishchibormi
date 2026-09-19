package httpx

import (
	"net/http"
	"strings"
)

// Foydalanuvchi qaysi klientdan kelgani. Ro'yxat YOPIQ: klient yuborgan
// matn shu qiymatlarning biriga keltiriladi, aks holda [PlatformUnknown]
// bo'ladi.
//
// Nega yopiq: bu qiymat bazaga yoziladi va admin panelida guruhlanadi.
// Klient yuborgan xom satrni saqlasak, bitta noto'g'ri build ("Android",
// "android-14", "  web") hisobotda alohida ustun bo'lib chiqardi va sanoq
// jimgina buzilardi.
const (
	PlatformWeb     = "web"
	PlatformAndroid = "android"
	PlatformIOS     = "ios"
	// PlatformTelegram — Telegram bot: chatdagi suhbat ham, Mini App ham.
	// Ilova yoki brauzerdan ALOHIDA o'lchov: bu odamlar saytga ham,
	// ilovaga ham kirmagan bo'lishi mumkin, ya'ni ularni "veb" ga qo'shib
	// yuborish o'sish kanalini noto'g'ri ko'rsatardi.
	PlatformTelegram = "telegram"
	// PlatformUnknown — klient o'zini tanitmagan. Saqlashda BO'SH satr
	// ishlatiladi (maydon umuman yozilmaydi), bu esa faqat ko'rsatish uchun.
	PlatformUnknown = "unknown"
)

// ClientPlatformHeader — klient o'zini tanitadigan sarlavha.
//
// User-Agent'dan alohida sarlavha, chunki UA ishonchsiz: Dio (Flutter)
// standart holatda "Dart/3.x (dart:io)" yuboradi — bu Android'ni iOS'dan
// ajratmaydi; brauzerda esa UA'ni har qanday kengaytma o'zgartirishi mumkin.
// Sarlavhani klientlarning o'zi qo'yadi va u ATAYLAB ixtiyoriy: eski
// versiyalar uni yubormaydi va ular oddiygina "unknown" bo'lib qoladi.
const ClientPlatformHeader = "X-Client-Platform"

// Platforms — hisobotlarda ko'rsatiladigan tartib.
var Platforms = []string{PlatformWeb, PlatformAndroid, PlatformIOS, PlatformTelegram}

// ClientPlatform so'rov qaysi platformadan kelganini qaytaradi:
// "web" | "android" | "ios", yoki aniqlab bo'lmasa BO'SH satr.
//
// Bo'sh satr ataylab: chaqiruvchi uni bazaga yozmasligi kerak. "unknown"
// deb yozib qo'ysak, keyinchalik haqiqiy qiymat kelganda uni "eskirgan
// noma'lum" dan ajratib bo'lmasdi.
func ClientPlatform(r *http.Request) string {
	if p := normalizePlatform(r.Header.Get(ClientPlatformHeader)); p != "" {
		return p
	}
	// Zaxira: sarlavhani yubormaydigan eski klientlar. Faqat brauzerni
	// ishonchli tanib oladi — mobil ilova UA'si (Dart/dart:io) qaysi OS
	// ekanini aytmaydi, shuning uchun u yerda taxmin qilmaymiz.
	return platformFromUserAgent(r.UserAgent())
}

// normalizePlatform klient yuborgan qiymatni yopiq ro'yxatga keltiradi.
func normalizePlatform(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case PlatformWeb:
		return PlatformWeb
	case PlatformAndroid:
		return PlatformAndroid
	case PlatformIOS:
		return PlatformIOS
	case PlatformTelegram:
		return PlatformTelegram
	default:
		return ""
	}
}

// platformFromUserAgent — sarlavhasiz so'rov uchun zaxira.
//
// Faqat "bu brauzermi" degan savolga javob beradi. Mo'ljal — Mozilla
// prefiksi: uni har bir brauzer yuboradi, `curl`, Dio va Go'ning
// http.Client esa yubormaydi.
func platformFromUserAgent(ua string) string {
	if strings.HasPrefix(ua, "Mozilla/") {
		return PlatformWeb
	}
	return ""
}

// PlatformOrUnknown — ko'rsatish uchun: bo'sh qiymatni [PlatformUnknown] ga
// aylantiradi. Saqlashda ISHLATILMAYDI.
func PlatformOrUnknown(p string) string {
	if p == "" {
		return PlatformUnknown
	}
	return p
}

// Veb klient QAYSI QURILMADA ochilgani. Platformadan alohida o'lchov:
// "web" bizga brauzer ekanini aytadi, lekin telefonmi yoki kompyutermi —
// aytmaydi. Admin panelida "Veb Android" / "Veb iOS" shu yerdan chiqadi.
//
// Ro'yxat yopiq, [Platform*] kabi va xuddi shu sabab bilan: qiymat bazaga
// yoziladi va guruhlanadi.
const (
	DeviceAndroid  = "android"
	DeviceIOS      = "ios"
	DeviceWindows  = "windows"
	DeviceMacOS    = "macos"
	DeviceLinux    = "linux"
	DeviceChromeOS = "chromeos"
	// DeviceDesktop — ESKI qiymat: bir vaqtlar har qanday stol kompyuteri
	// shu nom bilan yozilgan, endi esa aniq OS yoziladi. YANGI yozuvlarda
	// chiqmaydi, faqat bazadagi eskilarini o'qish uchun qoladi. Ko'rsatishda
	// qo'shimcha so'z bermaydi — "Veb Desktop" emas, shunchaki "Veb".
	DeviceDesktop = "desktop"
)

// ClientDevice — VEB so'rov qaysi qurilma OS'idan kelgani: "android" |
// "ios" | "windows" | "macos" | "linux" | "chromeos", aniqlab bo'lmasa
// BO'SH satr.
//
// # NEGA YANGI SARLAVHA EMAS, User-Agent
//
// [ClientPlatform] ataylab sarlavhaga tayanadi: u yerda savol "qaysi
// KLIENT" edi va Flutter'ning Dio'si UA'da Android'ni iOS'dan ajratmaydi.
// Bu yerda savol boshqa — "qaysi BRAUZER OS'i" — va unga UA aynan
// mo'ljallangan javob beradi: har bir brauzer OS tokenini o'zi qo'yadi.
//
// Xavfsizlik nuqtai nazaridan bu ONG tanlov: yangi sarlavha kiritsak,
// klient boshqaradigan yana bitta kirish maydoni paydo bo'lardi, UA esa
// allaqachon har so'rovda keladi — hujum yuzasi kengaymaydi. Ikkalasi ham
// klient tomonidan o'zgartirilishi mumkin, shuning uchun qiymat FAQAT
// ko'rsatish va statistika uchun: hech qayerda ruxsat tekshiruviga
// kirmaydi.
//
// Xom UA satri SAQLANMAYDI — u barmoq izi darajasidagi ma'lumot. Bazaga
// faqat yuqoridagi yopiq ro'yxatdan bitta qiymat tushadi, ya'ni bu maydon
// orqali ixtiyoriy matn (CSV formulasi, HTML) o'tkazib bo'lmaydi.
//
// Ma'lum cheklov: iPadOS 13+ Safari o'zini "Macintosh" deb tanitadi, ya'ni
// bunday iPad "macos" bo'lib qoladi. UA bilan buni tuzatib bo'lmaydi va
// noto'g'ri "ios" deb belgilashdan ko'ra kam aniq javob ma'qul.
func ClientDevice(r *http.Request) string {
	// Veb va Telegram uchun ma'noga ega. Mobil ilovada platformaning o'zi
	// allaqachon qurilma OS'i ("android"/"ios"), takrorlash chalkashtirardi.
	// Telegram esa qurilma haqida hech nima demaydi: Mini App haqiqiy
	// brauzer freymida ochiladi va UA'da OS tokeni bo'ladi ("Telegram iOS").
	// Chatdagi bot suhbatida so'rov Go klientidan keladi, UA'da Mozilla
	// prefiksi yo'q — u yerda qurilma bo'sh qoladi, bu to'g'ri javob.
	if p := ClientPlatform(r); p != PlatformWeb && p != PlatformTelegram {
		return ""
	}
	ua := r.UserAgent()
	// TARTIB MUHIM — tokenlar bir-birini o'z ichiga oladi:
	//   Android UA'sida "Linux" ham bor  → Android oldinroq;
	//   ChromeOS UA'sida "X11" bor       → CrOS Linux'dan oldinroq.
	// Tartibni o'zgartirsangiz, quyidagi holatlar jimgina noto'g'ri
	// guruhga tushadi.
	switch {
	case strings.Contains(ua, "Android"):
		return DeviceAndroid
	// iOS'dagi HAR QANDAY brauzer (Safari, CriOS, FxiOS) shu tokenlardan
	// birini yuboradi — hammasi WebKit ustida ishlaydi.
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"), strings.Contains(ua, "iPod"):
		return DeviceIOS
	case strings.Contains(ua, "CrOS"):
		return DeviceChromeOS
	// "Windows NT" — stol versiyasi, "Windows Phone" — o'lgan platforma;
	// ikkalasi ham bitta guruhga tushaveradi.
	case strings.Contains(ua, "Windows"):
		return DeviceWindows
	case strings.Contains(ua, "Macintosh"), strings.Contains(ua, "Mac OS X"):
		return DeviceMacOS
	case strings.Contains(ua, "Linux"), strings.Contains(ua, "X11"):
		return DeviceLinux
	default:
		// Brauzer, lekin OS'ini tanimadik (kam uchraydigan tizim yoki UA
		// kengaytma bilan o'zgartirilgan), yoki umuman brauzer emas
		// (skript, bot). Taxmin qilmaymiz — bo'sh qiymat saqlanmaydi va
		// panelda shunchaki "Veb" ko'rinadi.
		return ""
	}
}
