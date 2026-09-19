package posting

import (
	"strconv"
	"strings"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Profil to'ldirish qadamlari.
//
// NEGA FAMILIYA FAQAT RO'YXATDAN O'TISHDA: mavjud hisoblarning ko'pida
// familiya yo'q (sayt uni majburiy qilmagan). Uni ariza berish o'rtasida
// talab qilsak, ish topayotgan odam kutilmaganda qo'shimcha savolga duch
// kelardi. Ro'yxatdan o'tish esa aynan profil to'ldirish jarayoni.
type profileStep struct{ step, value string }

func profileSteps(purpose string, p Profile) []profileStep {
	steps := []profileStep{{"first_name", p.FirstName}}
	if purpose == "register" {
		steps = append(steps, profileStep{"last_name", p.LastName})
	}
	return append(steps, profileStep{"region", p.Region}, profileStep{"district", p.District})
}

// Bir sahifadagi tuman tugmalari. Eng katta viloyatda 22 ta tuman bor —
// hammasini bitta xabarga solsak ro'yxat ekranga sig'may ketardi.
const districtsPerPage = 8

func districtsOf(region string) []string {
	for _, r := range uzRegions {
		if r.Name == region {
			return r.Districts
		}
	}
	return nil
}

// pickRegion tugma bosilishini ham, qo'lda yozilgan nomni ham qabul qiladi.
//
// Qo'lda yozish ATAYLAB qoldirilgan: odam tugmani bosmasdan «Samarqand» deb
// yozishi tabiiy va uni «tugmani bosing» deb qaytarish bema'ni bo'lardi.
// Lekin natija baribir yopiq ro'yxatdan chiqadi — profilga ixtiyoriy matn
// tushmaydi va sayt/ilova bilan nomlar bir xil qoladi.
func pickRegion(action, typed string) (string, bool) {
	if strings.HasPrefix(action, "r") {
		i, err := strconv.Atoi(strings.TrimPrefix(action, "r"))
		if err != nil || i < 0 || i >= len(uzRegions) {
			return "", false
		}
		return uzRegions[i].Name, true
	}
	if name := matchName(typed, regionNames()); name != "" {
		return name, true
	}
	return "", false
}

func pickDistrict(region, action, typed string) (string, bool) {
	list := districtsOf(region)
	if len(list) == 0 {
		return "", false
	}
	if strings.HasPrefix(action, "d") {
		i, err := strconv.Atoi(strings.TrimPrefix(action, "d"))
		if err != nil || i < 0 || i >= len(list) {
			return "", false
		}
		return list[i], true
	}
	if name := matchName(typed, list); name != "" {
		return name, true
	}
	return "", false
}

// districtPage tuman ro'yxatining sahifasini qaytaradi ("dp2" -> 2).
func districtPage(action string) (int, bool) {
	if !strings.HasPrefix(action, "dp") {
		return 0, false
	}
	page, err := strconv.Atoi(strings.TrimPrefix(action, "dp"))
	if err != nil || page < 0 {
		return 0, false
	}
	return page, true
}

func regionNames() []string {
	names := make([]string, len(uzRegions))
	for i, r := range uzRegions {
		names[i] = r.Name
	}
	return names
}

// matchName yozilgan matnni yopiq ro'yxatga solishtiradi.
//
// Registr va bo'shliqlar e'tiborga olinmaydi. «Tumani»/«shahri» qo'shimchasi
// ham ixtiyoriy: odam «Chilonzor» deb yozsa «Chilonzor tumani» topilishi
// kerak — aks holda ro'yxatdagi aniq nomni topa olmay qolardi.
func matchName(typed string, list []string) string {
	want := normalizeName(typed)
	if want == "" {
		return ""
	}
	for _, name := range list {
		if normalizeName(name) == want {
			return name
		}
	}
	for _, name := range list {
		if trimPlaceSuffix(normalizeName(name)) == trimPlaceSuffix(want) {
			return name
		}
	}
	return ""
}

func normalizeName(s string) string {
	// Apostroflar uch xil yoziladi (' ' ʻ) — solishtirishda ularni
	// e'tibordan qoldiramiz, aks holda «Farg'ona» hech qachon topilmasdi.
	replacer := strings.NewReplacer("'", "", "‘", "", "’", "", "ʻ", "", "`", "")
	return replacer.Replace(strings.ToLower(strings.Join(strings.Fields(s), " ")))
}

func trimPlaceSuffix(s string) string {
	for _, suffix := range []string{" tumani", " shahri", " tuman", " shahar"} {
		if strings.HasSuffix(s, suffix) {
			return strings.TrimSuffix(s, suffix)
		}
	}
	return s
}

// Ro'yxatdan o'tishdagi viloyat tugmalari. Qidiruvdagi regionRows'dan
// ALOHIDA: u yerda viloyat markazining koordinatasi kerak, bu yerda esa
// profilga yoziladigan nom.
// Viloyatlar bitta xabarga sig'adi (14 ta), shuning uchun sahifalash yo'q.
func profileRegionRows(f *WorkerFlow) [][]tg.InlineKeyboardButton {
	rows := [][]tg.InlineKeyboardButton{}
	for i := 0; i < len(uzRegions); i += 2 {
		row := []tg.InlineKeyboardButton{flowButton(f, uzRegions[i].Name, "r"+strconv.Itoa(i))}
		if i+1 < len(uzRegions) {
			row = append(row, flowButton(f, uzRegions[i+1].Name, "r"+strconv.Itoa(i+1)))
		}
		rows = append(rows, row)
	}
	return rows
}

func profileDistrictRows(f *WorkerFlow) [][]tg.InlineKeyboardButton {
	list := districtsOf(f.Profile.Region)
	page := f.Page
	if page < 1 {
		page = 1
	}
	start := (page - 1) * districtsPerPage
	if start >= len(list) {
		start, page = 0, 1
	}
	rows := [][]tg.InlineKeyboardButton{}
	for i := start; i < len(list) && i < start+districtsPerPage; i++ {
		rows = append(rows, tg.NewInlineKeyboardRow(flowButton(f, list[i], "d"+strconv.Itoa(i))))
	}
	nav := []tg.InlineKeyboardButton{}
	if page > 1 {
		nav = append(nav, flowButton(f, "← Oldingi", "dp"+strconv.Itoa(page-1)))
	}
	if start+districtsPerPage < len(list) {
		nav = append(nav, flowButton(f, "Keyingi →", "dp"+strconv.Itoa(page+1)))
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	// Viloyatni almashtirish: noto'g'ri tanlagan odam oqimni qaytadan
	// boshlamasin.
	return append(rows, tg.NewInlineKeyboardRow(flowButton(f, "⬅️ Viloyatni o'zgartirish", "region")))
}
