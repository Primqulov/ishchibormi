package posting

import (
	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Viloyat markazlari — GPS yubora olmaydigan foydalanuvchi uchun zaxira yo'l.
//
// NEGA KERAK: Telegram Desktop lokatsiya yubora olmaydi, telefonda esa
// foydalanuvchi joylashuvga ruxsat bermasligi mumkin. Bunday odam uchun /jobs
// butunlay yopiq edi. Bu yerdagi koordinata — viloyat markazidagi shaharning
// taxminiy nuqtasi: qidiruv eng yaqinidan saralagani uchun natija o'sha
// viloyatdan boshlanadi. Aniq masofa kerak bo'lsa foydalanuvchi baribir
// lokatsiya yuborishi mumkin.
type regionCenter struct {
	Name, City string
	Lat, Lng   float64
}

// Nomlar apps/web/lib/regions.ts dagi ro'yxat bilan bir xil tartibda.
var regionCenters = []regionCenter{
	{"Toshkent shahri", "Toshkent", 41.3111, 69.2797},
	{"Toshkent viloyati", "Nurafshon", 41.0392, 69.3568},
	{"Samarqand", "Samarqand", 39.6542, 66.9597},
	{"Andijon", "Andijon", 40.7821, 72.3442},
	{"Farg'ona", "Farg'ona", 40.3864, 71.7864},
	{"Namangan", "Namangan", 40.9983, 71.6726},
	{"Buxoro", "Buxoro", 39.7747, 64.4286},
	{"Qashqadaryo", "Qarshi", 38.8606, 65.7847},
	{"Surxondaryo", "Termiz", 37.2242, 67.2783},
	{"Xorazm", "Urganch", 41.5506, 60.6317},
	{"Navoiy", "Navoiy", 40.1033, 65.3688},
	{"Jizzax", "Jizzax", 40.1158, 67.8422},
	{"Sirdaryo", "Guliston", 40.4897, 68.7842},
	{"Qoraqalpog'iston", "Nukus", 42.4531, 59.6103},
}

func regionByIndex(i int) (regionCenter, bool) {
	if i < 0 || i >= len(regionCenters) {
		return regionCenter{}, false
	}
	return regionCenters[i], true
}

// regionRows — ikki ustunli tugmalar jadvali. 14 ta viloyat bitta ekranga
// sig'adi, shuning uchun sahifalash kerak emas.
func regionRows(searchKey string) [][]tg.InlineKeyboardButton {
	rows := [][]tg.InlineKeyboardButton{}
	row := []tg.InlineKeyboardButton{}
	for i, r := range regionCenters {
		row = append(row, tg.NewInlineKeyboardButtonData(r.Name, callbackRegion(searchKey, i)))
		if len(row) == 2 {
			rows = append(rows, row)
			row = []tg.InlineKeyboardButton{}
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}
