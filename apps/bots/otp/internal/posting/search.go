package posting

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"sync"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type searchState struct {
	Key                            string
	Lat, Lng                       float64
	HasLocation                    bool
	Page, Total, Limit, LastUpdate int
	ExpiresAt                      time.Time
	Day, CategoryID, CategoryName  string
	// Place — koordinata viloyat ro'yxatidan olingan bo'lsa uning nomi
	// ("Samarqand (Samarqand markazi)"). Aniq lokatsiya yuborilganda bo'sh.
	Place string
}

// Reply-klaviaturadagi zaxira tugmalar. Matnlari shu yerda yagona joyda
// turadi: foydalanuvchi bosganda bot aynan shu matnni qabul qiladi.
const (
	btnRegionSearch = "🏙 Viloyat bo'yicha"
	btnLastLocation = "📍 Oxirgi joyim"
)

func callbackRegion(searchKey string, index int) string {
	return fmt.Sprintf("jobs:%s:region:%d", searchKey, index)
}

// Search coordinates are short-lived and stay in memory, separate from the
// employer's persisted draft. Restarting the bot asks for a new location.
type SearchSessions struct {
	mu     sync.Mutex
	states map[int64]searchState
}

func NewSearchSessions() *SearchSessions { return &SearchSessions{states: map[int64]searchState{}} }
func (s *SearchSessions) get(id int64, now time.Time) (searchState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.states[id]
	if ok && !v.ExpiresAt.After(now) {
		delete(s.states, id)
		return searchState{}, false
	}
	return v, ok
}
func (s *SearchSessions) set(id int64, v searchState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.states[id]; !exists && len(s.states) >= 10000 {
		var oldestID int64
		var oldest time.Time
		for key, item := range s.states {
			if oldest.IsZero() || item.ExpiresAt.Before(oldest) {
				oldestID = key
				oldest = item.ExpiresAt
			}
		}
		delete(s.states, oldestID)
	}
	s.states[id] = v
}
func (s *SearchSessions) delete(id int64) { s.mu.Lock(); defer s.mu.Unlock(); delete(s.states, id) }
func (s *SearchSessions) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for id, v := range s.states {
				if !v.ExpiresAt.After(now) {
					delete(s.states, id)
				}
			}
			s.mu.Unlock()
		}
	}
}
func (e *Engine) StopJobSearch(chatID int64) {
	if e.Searches != nil {
		e.Searches.delete(chatID)
	}
}

func (e *Engine) handleJobSearch(ctx context.Context, u tg.Update, d *Draft) (bool, error) {
	if e.Searches == nil {
		return false, nil
	}
	chat := d.ChatID
	m, cmd, data := u.Message, "", ""
	if u.CallbackQuery != nil {
		m = u.CallbackQuery.Message
		data = u.CallbackQuery.Data
	} else if m != nil && m.IsCommand() {
		cmd = m.Command()
	}
	state, active := e.Searches.get(chat, e.now())
	start := cmd == "jobs" || (cmd == "start" && strings.TrimSpace(m.CommandArguments()) == "jobs") || data == "jobs:start"
	if !start && (cmd != "" || strings.HasPrefix(data, "p:")) {
		e.Searches.delete(chat)
		if active && cmd == "cancel" {
			return true, e.searchStopped(chat)
		}
		return false, nil
	}
	if start {
		state = searchState{Key: primitive.NewObjectID().Hex()[12:], Page: 1, LastUpdate: u.UpdateID, ExpiresAt: e.now().Add(time.Hour)}
		e.Searches.set(chat, state)
		return true, e.askSearchLocation(chat, d)
	}
	if strings.HasPrefix(data, "jobs:") {
		parts := strings.Split(data, ":")
		if !active || len(parts) != 4 || parts[1] != state.Key {
			return true, e.say(chat, "Bu qidiruv tugmasi eskirgan. /jobs orqali yangilang.")
		}
		if u.UpdateID <= state.LastUpdate {
			return true, nil
		}
		switch parts[2] {
		case "day":
			if parts[3] != "all" && parts[3] != "today" && parts[3] != "tomorrow" {
				return true, nil
			}
			state.Day, state.Page = parts[3], 1
		case "categories":
			page, err := strconv.Atoi(parts[3])
			if err != nil || page < 0 {
				return true, nil
			}
			return true, e.searchCategories(ctx, chat, state, page)
		case "category":
			state.CategoryID, state.CategoryName, state.Page = "", "", 1
			if parts[3] != "all" {
				categories, err := e.API.Categories(ctx)
				if err != nil {
					return true, e.say(chat, userError(err))
				}
				for _, cat := range categories {
					if cat.ID == parts[3] {
						state.CategoryID, state.CategoryName = cat.ID, cat.Name
						break
					}
				}
				if state.CategoryID == "" {
					return true, e.say(chat, "Ish turi topilmadi. Ro'yxatni yangilang.")
				}
			}
		case "region":
			index, err := strconv.Atoi(parts[3])
			if err != nil {
				return true, nil
			}
			region, ok := regionByIndex(index)
			if !ok {
				return true, e.say(chat, "Viloyat topilmadi. /jobs orqali qaytadan boshlang.")
			}
			state.Lat, state.Lng, state.HasLocation, state.Page = region.Lat, region.Lng, true, 1
			state.Place = region.Name + " (" + region.City + " markazi)"
		case "stop":
			e.Searches.delete(chat)
			return true, e.searchStopped(chat)
		case "location":
			state.HasLocation = false
			state.Key = primitive.NewObjectID().Hex()[12:]
			state.LastUpdate = u.UpdateID
			state.ExpiresAt = e.now().Add(time.Hour)
			e.Searches.set(chat, state)
			return true, e.askSearchLocation(chat, d)
		case "refresh":
			state.Page = 1
		case "page":
			page, err := strconv.Atoi(parts[3])
			if err != nil || page < 1 || page > 1000 || page < state.Page-1 || page > state.Page+1 {
				return true, e.say(chat, "Joriy qidiruv tugmalaridan foydalaning.")
			}
			state.Page = page
		default:
			return true, nil
		}
		if !state.HasLocation {
			return true, e.askSearchLocation(chat, d)
		}
	} else {
		loc := m.Location
		if m.Venue != nil {
			loc = &m.Venue.Location
		}
		if !active && loc == nil {
			return false, nil
		}
		if active && u.UpdateID <= state.LastUpdate {
			return true, nil
		}
		place := ""
		switch strings.TrimSpace(m.Text) {
		case btnRegionSearch:
			// Kalit ataylab yangilanmaydi: quyidagi viloyat tugmalari aynan
			// shu holatga tegishli bo'lib qolishi kerak.
			state.LastUpdate = u.UpdateID
			e.Searches.set(chat, state)
			return true, e.searchRegions(chat, state)
		case btnLastLocation:
			if !d.HasLastSearch {
				return true, e.askSearchLocation(chat, d)
			}
			loc = &tg.Location{Latitude: d.LastSearchLat, Longitude: d.LastSearchLng}
			place = d.LastSearchPlace
		}
		if loc == nil {
			parts := strings.Split(strings.TrimSpace(m.Text), ",")
			if len(parts) == 2 {
				lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
				lng, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
				if err1 == nil && err2 == nil {
					loc = &tg.Location{Latitude: lat, Longitude: lng}
				}
			}
		}
		if loc == nil {
			return true, e.askSearchLocation(chat, d)
		}
		if !validCoordinates(loc.Latitude, loc.Longitude) {
			return true, e.say(chat, "Joylashuv noto'g'ri. Telegram xaritasidan lokatsiya yuboring.")
		}
		state.Lat, state.Lng, state.HasLocation, state.Page = loc.Latitude, loc.Longitude, true, 1
		state.Place = place
	}
	state.LastUpdate = u.UpdateID
	state.Key = primitive.NewObjectID().Hex()[12:]
	state.ExpiresAt = e.now().Add(time.Hour)
	e.Searches.set(chat, state)
	// Yagona joy: hudud ham, ish turi ham shu yerda eslab qolinadi — qaysi
	// yo'l bilan tanlangani (lokatsiya, viloyat, oxirgi joy, filtr) muhim emas.
	e.rememberSearchLocation(ctx, d, state)
	filter := SearchFilter{CategoryID: state.CategoryID}
	if state.Day == "today" {
		filter.Day = e.now().In(tashkent).Format("2006-01-02")
	}
	if state.Day == "tomorrow" {
		filter.Day = e.now().In(tashkent).AddDate(0, 0, 1).Format("2006-01-02")
	}
	page, err := e.API.Nearby(ctx, state.Lat, state.Lng, state.Page, filter)
	if err != nil {
		_ = e.say(chat, userError(err))
		return true, e.searchControls(chat, state, "Qayta urinib ko'ring yoki joylashuvni o'zgartiring.")
	}
	state.Total, state.Limit = page.Total, page.Limit
	if state.Limit < 1 {
		state.Limit = 5
	}
	e.Searches.set(chat, state)
	return true, e.showNearby(chat, state, page)
}

// askSearchLocation — qidiruvning kirish nuqtasi. Uchala yo'l shu yerda:
// aniq lokatsiya (eng yaxshisi), viloyat markazi (GPS bo'lmasa) va oxirgi
// qidiruv joyi (takroriy qidiruvda hech narsa so'ralmaydi).
func (e *Engine) askSearchLocation(chat int64, d *Draft) error {
	text := "📍 Sizga yaqin ishlarni topish\n\nJoylashuvingizni yuboring: pastdagi tugmani bosing yoki Telegram 📎 → Joylashuv orqali xaritadan belgilang.\n\nKompyuterdan kirgan bo'lsangiz yoki joylashuvni ulashmoqchi bo'lmasangiz — «" + btnRegionSearch + "» tugmasi.\n\nE'lonlar eng yaqinidan boshlab ko'rsatiladi.\nBekor qilish: /cancel · E'lon berish: /post"
	rows := [][]tg.KeyboardButton{tg.NewKeyboardButtonRow(tg.NewKeyboardButtonLocation("📍 Joylashuvimni yuborish"))}
	second := tg.NewKeyboardButtonRow(tg.NewKeyboardButton(btnRegionSearch))
	if d != nil && d.HasLastSearch {
		second = append(second, tg.NewKeyboardButton(btnLastLocation))
		text += "\n\nOxirgi qidiruv joyi: " + d.lastSearchLabel()
	}
	rows = append(rows, second)
	m := tg.NewMessage(chat, text)
	kb := tg.NewReplyKeyboard(rows...)
	kb.ResizeKeyboard = true
	kb.OneTimeKeyboard = true
	m.ReplyMarkup = kb
	_, err := e.Bot.Send(m)
	return err
}

func (e *Engine) searchRegions(chat int64, s searchState) error {
	return e.workerMessage(chat, "🏙 Viloyatni tanlang.\n\nQidiruv shu viloyat markazidan boshlanadi va ishlarni eng yaqinidan saralaydi. Aniq masofa kerak bo'lsa joylashuvingizni yuboring.", regionRows(s.Key)...)
}

// rememberSearchLocation — qidiruv hududini va ish turini Mongo'dagi draft'ga
// yozadi.
//
// NEGA DRAFT: xotiradagi searchState bir soatda o'chadi, bot restart bo'lsa
// yo'qoladi va worker oqimiga o'tilganda ataylab tozalanadi (worker.go:
// StopJobSearch). Draft esa qoladi — shuning uchun keyingi /jobs da lokatsiya
// qayta so'ralmaydi va «signal yoqish» qaysi hudud haqida ekanini biladi.
func (e *Engine) rememberSearchLocation(ctx context.Context, d *Draft, s searchState) {
	if d == nil || !s.HasLocation || !validCoordinates(s.Lat, s.Lng) {
		return
	}
	if d.HasLastSearch && d.LastSearchLat == s.Lat && d.LastSearchLng == s.Lng &&
		d.LastSearchPlace == s.Place && d.LastSearchCategoryID == s.CategoryID {
		return
	}
	d.LastSearchLat, d.LastSearchLng, d.LastSearchPlace = s.Lat, s.Lng, s.Place
	d.LastSearchCategoryID, d.LastSearchCategoryName = s.CategoryID, s.CategoryName
	d.HasLastSearch = true
	// Saqlanmasa qidiruv baribir davom etadi — bu faqat qulaylik yozuvi.
	_ = e.Store.Save(ctx, d)
}

func (e *Engine) searchCategories(ctx context.Context, chat int64, s searchState, page int) error {
	categories, err := e.API.Categories(ctx)
	if err != nil {
		return e.say(chat, userError(err))
	}
	if page > len(categories)/8 {
		page = 0
	}
	button := func(label, action, value string) tg.InlineKeyboardButton {
		return tg.NewInlineKeyboardButtonData(label, "jobs:"+s.Key+":"+action+":"+value)
	}
	rows := [][]tg.InlineKeyboardButton{tg.NewInlineKeyboardRow(button("Barcha ish turlari", "category", "all"))}
	for i := page * 8; i < len(categories) && i < (page+1)*8; i++ {
		rows = append(rows, tg.NewInlineKeyboardRow(button(shortText(categories[i].Name, 50), "category", categories[i].ID)))
	}
	nav := []tg.InlineKeyboardButton{}
	if page > 0 {
		nav = append(nav, button("← Oldingi", "categories", strconv.Itoa(page-1)))
	}
	if (page+1)*8 < len(categories) {
		nav = append(nav, button("Keyingi →", "categories", strconv.Itoa(page+1)))
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	return e.workerMessage(chat, "Qaysi turdagi ish kerak?", rows...)
}
func (e *Engine) searchStopped(chat int64) error {
	m := tg.NewMessage(chat, "Qidiruv to'xtatildi. Yangi qidiruv: /jobs. Mini App orqali e'lon berish: /post.")
	// Lokatsiya so'ragan klaviatura o'rniga doimiy menyu qaytariladi.
	m.ReplyMarkup = MainKeyboard()
	_, err := e.Bot.Send(m)
	return err
}
func (e *Engine) searchControls(chat int64, s searchState, text string) error {
	button := func(label, action string, page int) tg.InlineKeyboardButton {
		return tg.NewInlineKeyboardButtonData(label, fmt.Sprintf("jobs:%s:%s:%d", s.Key, action, page))
	}
	rows := [][]tg.InlineKeyboardButton{}
	rows = append(rows, tg.NewInlineKeyboardRow(
		tg.NewInlineKeyboardButtonData("Bugun", "jobs:"+s.Key+":day:today"), tg.NewInlineKeyboardButtonData("Ertaga", "jobs:"+s.Key+":day:tomorrow"), tg.NewInlineKeyboardButtonData("Barcha kunlar", "jobs:"+s.Key+":day:all")),
		tg.NewInlineKeyboardRow(button("🏷 Ish turini tanlash", "categories", 0), workerButton("📋 Arizalarim", "apps", "all")))
	nav := []tg.InlineKeyboardButton{}
	if s.Page > 1 {
		nav = append(nav, button("← Oldingi", "page", s.Page-1))
	}
	if s.Page*s.Limit < s.Total && s.Page < 1000 {
		nav = append(nav, button("Keyingi →", "page", s.Page+1))
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	rows = append(rows,
		tg.NewInlineKeyboardRow(button("🔄 Yangilash", "refresh", 0), button("📍 Joylashuvni o'zgartirish", "location", 0)),
		// Signal shu yerda: foydalanuvchi aynan hozir hududni tanlagan, ya'ni
		// obunani yoqish uchun boshqa hech narsa so'ralmaydi.
		tg.NewInlineKeyboardRow(workerButton("🔔 Signal yoqish", "alert", "on")),
		tg.NewInlineKeyboardRow(button("Qidiruvni tugatish", "stop", 0)))
	m := tg.NewMessage(chat, text)
	m.ReplyMarkup = tg.NewInlineKeyboardMarkup(rows...)
	_, err := e.Bot.Send(m)
	return err
}
func (e *Engine) showNearby(chat int64, s searchState, page NearbyPage) error {
	text := fmt.Sprintf("📍 Sizga eng yaqin ishlar · %d-sahifa\nJami: %d ta faol e'lon.\nMasofa to'g'ri chiziq bo'yicha taxminiy.", s.Page, page.Total)
	if s.Place != "" {
		text += "\nQidiruv nuqtasi: " + s.Place
	}
	if s.Day == "today" {
		text += "\nSana: bugun."
	}
	if s.Day == "tomorrow" {
		text += "\nSana: ertaga."
	}
	if s.CategoryName != "" {
		text += "\nIsh turi: " + s.CategoryName
	}
	if len(page.Items) == 0 {
		text = "Tanlangan shartlarga mos faol ish e'lonlari topilmadi. Barcha kunlar yoki boshqa ish turini tanlab ko'ring."
		if s.Page > 1 {
			text = "Bu sahifada e'lon qolmagan. Qidiruvni yangilang."
		}
	}
	header := tg.NewMessage(chat, text)
	header.ReplyMarkup = MainKeyboard()
	if _, err := e.Bot.Send(header); err != nil {
		return err
	}
	for i, job := range page.Items {
		m := tg.NewMessage(chat, nearbyCard(job, (s.Page-1)*s.Limit+i+1))
		m.ParseMode = "HTML"
		m.DisableWebPagePreview = true
		m.ReplyMarkup = tg.NewInlineKeyboardMarkup(tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonURL("E'lonni ko'rish", strings.TrimRight(e.WebURL, "/")+"/elon/"+job.ID),
			tg.NewInlineKeyboardButtonURL("Xaritada ochish", fmt.Sprintf("https://www.google.com/maps?q=%f,%f", job.Lat, job.Lng)),
		), tg.NewInlineKeyboardRow(workerButton("📄 Botda batafsil", "job", job.ID), workerButton("🙋 Ariza berish", "apply", job.ID)))
		if _, err := e.Bot.Send(m); err != nil {
			return err
		}
	}
	return e.searchControls(chat, s, "Davom etish yoki qidiruvni yangilash uchun tugmalardan foydalaning.\nE'lon berish: /post")
}
func shortText(text string, limit int) string {
	r := []rune(strings.TrimSpace(text))
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return string(r)
}
func distanceText(meters float64) string {
	if meters < 1000 {
		return fmt.Sprintf("%.0f m", meters)
	}
	return fmt.Sprintf("%.1f km", meters/1000)
}
func nearbyCard(j NearbyJob, index int) string {
	esc := func(v string, n int) string { return html.EscapeString(shortText(v, n)) }
	pay := "Kelishiladi"
	if j.PricingType != "negotiable" && j.PerWorkerAmount > 0 {
		pay = fmt.Sprintf("%d so'm / ishchi", j.PerWorkerAmount)
	}
	date, clock := j.StartDate, j.WorkTimeFrom
	if len(date) >= 16 {
		clock = date[11:16]
	}
	if len(date) >= 10 {
		date = date[:10]
	}
	if parsed, err := time.Parse("2006-01-02", date); err == nil {
		date = parsed.Format("02.01.2006")
	}
	if date == "" {
		date = "Ko'rsatilmagan"
	}
	if clock == "" {
		clock = "Ko'rsatilmagan"
	}
	address := strings.Trim(strings.TrimSpace(j.Region)+", "+strings.TrimSpace(j.District), ", ")
	if address == "" {
		address = "Xaritada ko'ring"
	}
	gender := map[string]string{"male": "Erkak", "female": "Ayol", "mixed": "Aralash"}[j.Gender]
	if gender == "" {
		gender = "Aralash"
	}
	return fmt.Sprintf("<b>%d. %s</b>\n📍 Sizdan: <b>%s</b>\n💰 %s\n📅 %s · %s (Toshkent)\n👥 Bo'sh o'rin: %d · %s\n🏷 %s\n🗺 %s\n\n%s", index, esc(j.Title, 160), distanceText(j.DistanceMeters), pay, esc(date, 40), esc(clock, 20), j.WorkersNeeded-j.AcceptedCount, gender, esc(j.CategoryName, 100), esc(address, 220), esc(j.Description, 450))
}
