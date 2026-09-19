package posting

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Only an unfinished conversation is stored here. API credentials are obtained
// again for every operation; the employer's posting draft stays independent.
type WorkerFlow struct {
	Key, Step, Purpose, ID, UserID string
	Profile                        Profile
	Job                            Job
	App                            Application
	People                         int
	Reason, Filter                 string
	Page                           int
	Employer                       bool
	// Filled — shu oqim davomida hisob bog'landi yoki profil maydoni
	// to'ldirildi. Ro'yxatdan o'tish yakunida «ro'yxatdan o'tdingiz» va
	// «allaqachon ro'yxatdan o'tgansiz» xabarlarini ajratish uchun kerak:
	// sessiyaning o'zi hisob YANGI yaratilganini bildirmaydi.
	Filled    bool
	ExpiresAt time.Time
}

func workerButton(label, action, id string) tg.InlineKeyboardButton {
	return tg.NewInlineKeyboardButtonData(label, "w:"+action+":"+id)
}
func flowButton(f *WorkerFlow, label, action string) tg.InlineKeyboardButton {
	return tg.NewInlineKeyboardButtonData(label, "w:flow:"+f.Key+":"+action)
}
func (e *Engine) workerMessage(chat int64, text string, rows ...[]tg.InlineKeyboardButton) error {
	m := tg.NewMessage(chat, text)
	m.DisableWebPagePreview = true
	if len(rows) > 0 {
		m.ReplyMarkup = tg.NewInlineKeyboardMarkup(rows...)
	} else {
		m.ReplyMarkup = tg.NewRemoveKeyboard(true)
	}
	_, err := e.Bot.Send(m)
	return err
}
func (e *Engine) workerHome(chat int64) error {
	// Xabar ATAYLAB workerMessage orqali emas: u klaviaturani olib tashlaydi,
	// bu yerda esa aksincha, doimiy menyu o'rnatiladi.
	welcome := tg.NewMessage(chat, "Ishchi Bormi — kunlik ish toping va arizalaringizni shu botda boshqaring.\n\nPastdagi tugmalar doim shu yerda turadi — buyruqlarni eslab qolish shart emas.")
	welcome.ReplyMarkup = MainKeyboard()
	if _, err := e.Bot.Send(welcome); err != nil {
		return err
	}
	return e.miniAppMessage(chat, "Kerakli bo'limni tanlang:",
		tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonData("📍 Yaqin ishlarni topish", "jobs:start")),
		tg.NewInlineKeyboardRow(workerButton("📋 Arizalarim", "apps", "all"), workerButton("✅ Qabul qilingan ishlar", "apps", "accepted")),
		tg.NewInlineKeyboardRow(workerButton("📨 Kelgan arizalar", "inbox", "pending"), tg.NewInlineKeyboardButtonURL("Mening e'lonlarim", strings.TrimRight(e.WebURL, "/")+"/my-elons")),
		tg.NewInlineKeyboardRow(workerButton("📝 Ro'yxatdan o'tish", "register", "new"), workerButton("ℹ️ Qanday ishlaydi?", "help", "")))
}

// registrationDone ro'yxatdan o'tish oqimining yakuniy xabari.
//
// NEGA ALOHIDA OQIM: ilgari hisob faqat yo'l-yo'lakay — ariza berish ichida —
// yaratilardi. Ya'ni «avval ro'yxatdan o'tay, ishni keyin qidiraman» degan
// odam saytga o'tishga majbur edi. Bu oqim ayni qadamlarni (rozilik, o'z
// kontakti, ism/viloyat/tuman) mustaqil ravishda bajaradi va ayni API'ga
// yozadi — sayt, mobil ilova va botdagi hisob bitta bo'lib qoladi.
//
// fresh=false — hisob allaqachon to'liq edi: bu holda xabar «ro'yxatdan
// o'tdingiz» demaydi, aks holda foydalanuvchi ikkinchi hisob ochdim deb
// o'ylashi mumkin.
func (e *Engine) registrationDone(chat int64, p Profile, fresh bool) error {
	name := strings.TrimSpace(p.FirstName + " " + p.LastName)
	if name == "" {
		name = "—"
	}
	head := "✅ Ro'yxatdan o'tdingiz."
	if !fresh {
		head = "Siz allaqachon ro'yxatdan o'tgansiz."
	}
	text := head + "\n\n👤 " + name + "\n📞 " + p.Phone + "\n📍 " + strings.TrimSpace(p.Region+", "+p.District) +
		"\n\nShu hisob sayt va mobil ilovada ham ishlaydi — u yerda qaytadan ro'yxatdan o'tish shart emas." +
		"\n\nMa'lumotni o'zgartirish uchun /register ni qayta yuboring."
	return e.workerMessage(chat, text,
		tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonData("📍 Ish topish", "jobs:start")),
		tg.NewInlineKeyboardRow(workerButton("⬅️ Bosh menyu", "home", "")))
}

func (e *Engine) saveWorker(ctx context.Context, d *Draft, update int) error {
	if d.Worker != nil {
		d.Worker.Key = primitive.NewObjectID().Hex()[12:]
		d.Worker.ExpiresAt = e.now().Add(24 * time.Hour)
	}
	return e.persist(ctx, d, update)
}
func (e *Engine) handleWorker(ctx context.Context, u tg.Update, d *Draft) (bool, error) {
	m, data, cmd := u.Message, "", ""
	if u.CallbackQuery != nil {
		m, data = u.CallbackQuery.Message, u.CallbackQuery.Data
	} else if m.IsCommand() {
		cmd = m.Command()
	}
	if d.Worker != nil && !d.Worker.ExpiresAt.After(e.now()) {
		d.Worker = nil
	}
	action, id, value := "", "", ""
	if strings.HasPrefix(data, "w:") {
		parts := strings.Split(data, ":")
		if len(parts) < 3 || len(parts) > 4 {
			return true, e.say(d.ChatID, "Tugma eskirgan. /menu orqali davom eting.")
		}
		action, id = parts[1], parts[2]
		if len(parts) == 4 {
			value = parts[3]
		}
	} else {
		switch cmd {
		case "start":
			arg := strings.TrimSpace(m.CommandArguments())
			if arg == "" {
				action = "home"
			} else if strings.HasPrefix(arg, "app_") {
				action, id = "app", strings.TrimPrefix(arg, "app_")
			} else if strings.HasPrefix(arg, "job_") {
				action, id = "job", strings.TrimPrefix(arg, "job_")
			}
		case "menu":
			action = "home"
		case "help":
			action = "help"
		case "applications":
			action, id = "apps", "all"
		case "myjobs":
			action, id = "apps", "accepted"
		case "candidates":
			action, id = "inbox", "pending"
		case "alerts":
			action, id = "alert", "status"
		case "register":
			action, id = "register", "new"
		}
	}
	active := d.Worker != nil
	if action == "" && (cmd != "" || (data != "" && !strings.HasPrefix(data, "w:"))) {
		if active {
			d.Worker = nil
			if err := e.Store.Save(ctx, d); err != nil {
				return true, err
			}
			if cmd == "cancel" {
				return true, e.workerHome(d.ChatID)
			}
		}
		return false, nil
	}
	if action == "" && !active {
		return false, nil
	}
	e.StopJobSearch(d.ChatID)
	if u.UpdateID <= d.LastUpdate && d.LastUpdate != 0 {
		return true, nil
	}
	if action == "flow" {
		if !active || id != d.Worker.Key {
			return true, e.say(d.ChatID, "Bu tugma eskirgan. /applications orqali joriy holatni oching.")
		}
		return true, e.continueWorker(ctx, d, u.UpdateID, m, value)
	}
	if action == "" {
		return true, e.continueWorker(ctx, d, u.UpdateID, m, "")
	}
	d.Worker = nil
	if action == "home" || action == "help" || action == "post" {
		if err := e.saveWorker(ctx, d, u.UpdateID); err != nil {
			return true, err
		}
		if action == "help" {
			return true, e.workerMessage(d.ChatID, "0. /register → ro'yxatdan o'tish: rozilik, o'z telefoningiz va ism/hudud. Ish qidirish uchun shart emas, ariza berish uchun kerak.\n1. /jobs → joylashuvingizni yuboring (kompyuterdan kirgan bo'lsangiz «🏙 Viloyat bo'yicha»), so'ng sana va ish turini tanlang.\n2. «Ariza berish» → o'z telefoningizni ulashing, necha kishi borishingizni tanlab tasdiqlang.\n3. /applications orqali javobni kuzating. Ariza yuborish ishga qabul qilindingiz degani emas.\n4. Qabul qilingach, ish beruvchi bilan bog'lanib, vaqt va manzilni kelishib oling.\n5. Ish haqiqatan tugagach, «Ishni tugatdim»ni tasdiqlang. Bu to'lov qabul qilinganini tasdiqlamaydi.\n6. /alerts — ish signali: belgilagan hududingizda yangi e'lon chiqqanda bot o'zi xabar beradi.\n\nBora olmasangiz, arizani sababini yozib bekor qiling.\nPastdagi tugmalar doim shu yerda: ish topish, arizalarim, e'lon berish.\n/menu — bosh menyu. /post — Mini App orqali e'lon berish.", tg.NewInlineKeyboardRow(workerButton("Bosh menyu", "home", "")))
		}
		if action == "post" {
			return true, e.miniAppMessage(d.ChatID, "E'lon berish uchun Mini App'ni oching.")
		}
		return true, e.workerHome(d.ChatID)
	}
	if action == "job" || action == "jobtext" {
		if !validWorkerID(id) {
			return true, e.say(d.ChatID, "E'lon manzili noto'g'ri.")
		}
		if err := e.saveWorker(ctx, d, u.UpdateID); err != nil {
			return true, err
		}
		job, err := e.API.Job(ctx, Session{}, id)
		if err != nil {
			if errorCode(err) == "not_found" {
				return true, e.workerMessage(d.ChatID, "Bu ish e'loni yopilgan yoki endi mavjud emas. /jobs orqali boshqa ish topishingiz mumkin.", tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonData("📍 Boshqa ishlarni topish", "jobs:start")))
			}
			return true, e.say(d.ChatID, workerError(err))
		}
		if action == "jobtext" {
			return true, e.sendText(d.ChatID, workerJobText(job, e.now()))
		}
		return true, e.showWorkerJob(ctx, d.ChatID, job)
	}
	f := &WorkerFlow{Purpose: action, ID: id, People: 1, Page: 1, Filter: "all", Employer: action == "inbox"}
	switch action {
	case "apps", "inbox":
		if id != "all" && id != "pending" && id != "accepted" && id != "history" {
			return true, nil
		}
		f.Filter = id
		if value != "" {
			page, err := strconv.Atoi(value)
			if err != nil || page < 1 || page > 1000 {
				return true, nil
			}
			f.Page = page
		}
	case "alert":
		// Bu yerda ObjectID emas, rejim keladi.
		if id != "on" && id != "off" && id != "status" {
			return true, nil
		}
	case "register":
		// Ro'yxatdan o'tishda e'lon/ariza ID si yo'q — faqat sobit belgi.
		if id != "new" {
			return true, nil
		}
	case "apply", "app", "cancel", "done", "accept", "reject":
		if !validWorkerID(id) {
			return true, e.say(d.ChatID, "Manzil noto'g'ri. /menu orqali davom eting.")
		}
	default:
		return true, e.workerHome(d.ChatID)
	}
	d.Worker = f
	if err := e.saveWorker(ctx, d, u.UpdateID); err != nil {
		return true, err
	}
	s, err := e.API.Login(ctx, d.ChatID, "")
	if errorCode(err) == "contact_required" {
		f.Step = "consent"
		if err := e.saveWorker(ctx, d, u.UpdateID); err != nil {
			return true, err
		}
		return true, e.workerPrompt(d)
	}
	if err != nil {
		d.Worker = nil
		if saveErr := e.saveWorker(ctx, d, u.UpdateID); saveErr != nil {
			return true, saveErr
		}
		return true, e.say(d.ChatID, workerError(err))
	}
	return true, e.workerAuthenticated(ctx, d, u.UpdateID, s)
}
func validWorkerID(id string) bool { _, err := primitive.ObjectIDFromHex(id); return err == nil }

func (e *Engine) StopWorker(ctx context.Context, chat int64) error {
	d, err := e.Store.Load(ctx, chat)
	if err != nil || d == nil {
		return err
	}
	d.Worker = nil
	return e.Store.Save(ctx, d)
}

func (e *Engine) workerSession(ctx context.Context, d *Draft) (Session, error) {
	s, err := e.API.Login(ctx, d.ChatID, "")
	if err == nil && s.User.ID != d.Worker.UserID {
		return Session{}, &APIError{Status: 403, Code: "identity_changed", Message: "Hisob o'zgargan. /menu orqali qayta boshlang."}
	}
	return s, err
}
func (e *Engine) workerAuthenticated(ctx context.Context, d *Draft, update int, s Session) error {
	f := d.Worker
	f.UserID, f.Profile = s.User.ID, s.User
	if f.Purpose == "apply" || f.Purpose == "register" {
		for _, field := range []struct{ step, text string }{{"first_name", f.Profile.FirstName}, {"region", f.Profile.Region}, {"district", f.Profile.District}} {
			if strings.TrimSpace(field.text) == "" {
				f.Step = field.step
				if err := e.saveWorker(ctx, d, update); err != nil {
					return err
				}
				return e.workerPrompt(d)
			}
		}
	}
	return e.prepareWorker(ctx, d, update, s)
}
func (e *Engine) prepareWorker(ctx context.Context, d *Draft, update int, s Session) error {
	f := d.Worker
	switch f.Purpose {
	case "apps", "inbox":
		apps, err := e.API.Applications(ctx, s, f.Filter, f.Page, f.Employer)
		if err != nil {
			return e.say(d.ChatID, workerError(err))
		}
		d.Worker = nil
		if err := e.saveWorker(ctx, d, update); err != nil {
			return err
		}
		return e.showApplications(d.ChatID, f, apps)
	case "alert":
		// Signal bir qadamlik amal: tasdiq bosqichi yo'q, shuning uchun oqim
		// darhol yopiladi va keyingi xabar mustaqil bo'ladi.
		d.Worker = nil
		if err := e.saveWorker(ctx, d, update); err != nil {
			return err
		}
		return e.handleAlert(ctx, d, s, f.ID)
	case "register":
		// Bu yerga yetib kelindi — demak hisob bor va workerAuthenticated
		// profilni to'liq qilib bo'ldi. Tasdiq bosqichi yo'q: ro'yxatdan
		// o'tish hech narsani boshqa odamga yubormaydi.
		d.Worker = nil
		if err := e.saveWorker(ctx, d, update); err != nil {
			return err
		}
		// ATAYLAB s.User EMAS: sessiya profil qadamlari boshlanishidan oldin
		// olingan, ya'ni unda hali yangi kiritilgan ism/hudud yo'q. Oqimning
		// o'z nusxasi — SaveProfile'ga yuborilgan ayni ma'lumot.
		return e.registrationDone(d.ChatID, f.Profile, f.Filled)
	case "apply":
		job, err := e.API.Job(ctx, s, f.ID)
		if err != nil {
			return e.say(d.ChatID, workerError(err))
		}
		if job.OwnerID == s.User.ID {
			return e.say(d.ChatID, "O'zingizning e'loningizga ariza yubora olmaysiz.")
		}
		if !jobOpen(job, e.now()) {
			return e.say(d.ChatID, "Bu ish hozir ariza qabul qilmayapti. /jobs orqali boshqa ish tanlang.")
		}
		f.Job, f.Step = job, "people"
	default:
		a, err := e.API.Application(ctx, s, f.ID)
		if err != nil {
			return e.say(d.ChatID, workerError(err))
		}
		if a.WorkerID != s.User.ID && a.EmployerID != s.User.ID {
			return e.say(d.ChatID, "Bu arizani boshqarish huquqingiz yo'q.")
		}
		if f.Purpose == "app" {
			d.Worker = nil
			if err := e.saveWorker(ctx, d, update); err != nil {
				return err
			}
			return e.showApplication(ctx, d.ChatID, s, a)
		}
		f.App = a
		if !workerActionAllowed(f.Purpose, a, s.User.ID) {
			return e.say(d.ChatID, "Ariza holati o'zgargan yoki bu amal siz uchun mavjud emas. /applications orqali yangilang.")
		}
		f.Step = "confirm"
		if f.Purpose == "cancel" {
			f.Step = "reason"
		}
	}
	if err := e.saveWorker(ctx, d, update); err != nil {
		return err
	}
	return e.workerPrompt(d)
}
func workerActionAllowed(action string, a Application, uid string) bool {
	worker, employer := a.WorkerID == uid, a.EmployerID == uid
	switch action {
	case "cancel":
		return (worker || employer) && (a.Status == "pending" || a.Status == "accepted")
	case "done":
		return a.Status == "accepted" && ((worker && !a.WorkerConfirmedDone) || (employer && !a.EmployerConfirmedDone))
	case "accept", "reject":
		return employer && a.Status == "pending"
	}
	return false
}
func (e *Engine) continueWorker(ctx context.Context, d *Draft, update int, m *tg.Message, action string) error {
	f := d.Worker
	text := strings.TrimSpace(m.Text)
	switch f.Step {
	case "consent":
		if action != "agree" {
			return e.workerPrompt(d)
		}
		f.Step = "contact"
	case "contact":
		if m.Contact == nil || m.Contact.UserID != d.ChatID {
			return e.say(d.ChatID, "Faqat o'zingizning raqamingizni «Telefonni ulashish» tugmasi bilan yuboring.")
		}
		phone := normalizePhone(m.Contact.PhoneNumber)
		if !phonePattern.MatchString(phone) {
			return e.say(d.ChatID, "+998 bilan boshlanadigan telefon raqamingizni yuboring.")
		}
		s, err := e.API.Login(ctx, d.ChatID, phone)
		if err != nil {
			return e.say(d.ChatID, workerError(err))
		}
		f.Filled = true
		if err := e.workerMessage(d.ChatID, "✅ Hisobingiz bog'landi."); err != nil {
			return err
		}
		return e.workerAuthenticated(ctx, d, update, s)
	case "first_name", "region", "district":
		limit := 100
		if f.Step == "first_name" {
			limit = 80
		}
		if !validText(text, limit) {
			return e.say(d.ChatID, fmt.Sprintf("1–%d belgidan iborat ma'lumot kiriting.", limit))
		}
		s, err := e.workerSession(ctx, d)
		if err != nil {
			return e.say(d.ChatID, workerError(err))
		}
		p := f.Profile
		switch f.Step {
		case "first_name":
			p.FirstName = text
		case "region":
			p.Region = text
		case "district":
			p.District = text
		}
		f.Profile, f.Filled = p, true
		for _, field := range []struct{ step, text string }{{"first_name", p.FirstName}, {"region", p.Region}, {"district", p.District}} {
			if strings.TrimSpace(field.text) == "" {
				f.Step = field.step
				if err := e.saveWorker(ctx, d, update); err != nil {
					return err
				}
				return e.workerPrompt(d)
			}
		}
		if err := e.API.SaveProfile(ctx, s, p); err != nil {
			return e.say(d.ChatID, workerError(err))
		}
		return e.prepareWorker(ctx, d, update, s)
	case "people":
		if action == "alone" {
			text = "1"
		}
		n, err := number(text, 100)
		available := f.Job.WorkersNeeded - f.Job.AcceptedCount
		if err != nil || n < 1 || int(n) > available {
			return e.say(d.ChatID, fmt.Sprintf("O'zingizni ham qo'shib, 1 dan %d gacha kishi sonini kiriting.", available))
		}
		f.People, f.Step = int(n), "confirm"
	case "reason":
		if !validText(text, 500) {
			return e.say(d.ChatID, "Bekor qilish sababini yozing (1–500 belgi).")
		}
		f.Reason, f.Step = text, "confirm"
	case "confirm":
		if action != "confirm" {
			return e.workerPrompt(d)
		}
		return e.commitWorker(ctx, d, update)
	case "submitted":
		return e.say(d.ChatID, "Oxirgi so'rov natijasini /applications yoki /candidates orqali tekshiring. Bir amal qayta yuborilmaydi.")
	default:
		s, err := e.API.Login(ctx, d.ChatID, "")
		if err != nil {
			return e.say(d.ChatID, workerError(err))
		}
		return e.workerAuthenticated(ctx, d, update, s)
	}
	if err := e.saveWorker(ctx, d, update); err != nil {
		return err
	}
	return e.workerPrompt(d)
}
func (e *Engine) workerPrompt(d *Draft) error {
	f := d.Worker
	rows := [][]tg.InlineKeyboardButton{}
	text := ""
	switch f.Step {
	case "consent":
		text = "Hisobingizni botga bog'lash uchun shartlar bilan tanishing. Ariza yuborsangiz, ism va tasdiqlangan telefon raqamingiz ish beruvchiga ko'rsatiladi."
		rows = append(rows, tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonURL("Shartlar", strings.TrimRight(e.WebURL, "/")+"/foydalanish-shartlari"), tg.NewInlineKeyboardButtonURL("Maxfiylik", strings.TrimRight(e.WebURL, "/")+"/maxfiylik-siyosati")), tg.NewInlineKeyboardRow(flowButton(f, "Roziman, davom etish", "agree")))
	case "contact":
		m := tg.NewMessage(d.ChatID, "Hisobingizni bog'lash uchun o'z telefon raqamingizni yuboring.\n/cancel — ortga qaytish.")
		kb := tg.NewReplyKeyboard(tg.NewKeyboardButtonRow(tg.NewKeyboardButtonContact("📞 Telefonni ulashish")))
		kb.ResizeKeyboard, kb.OneTimeKeyboard = true, true
		m.ReplyMarkup = kb
		_, err := e.Bot.Send(m)
		return err
	case "first_name":
		text = "Ish beruvchi sizni tanishi uchun ismingizni kiriting."
	case "region":
		text = "Yashash viloyatingizni kiriting. Masalan: Toshkent."
	case "district":
		text = "Yashash tumaningiz yoki shahringizni kiriting."
	case "people":
		text = fmt.Sprintf("%s\n\nNecha kishi borasiz? O'zingizni ham hisoblang.\nBo'sh o'rinlar: %d. Guruh bo'lib borsangiz, kishi sonini yozing.", f.Job.Title, f.Job.WorkersNeeded-f.Job.AcceptedCount)
		rows = append(rows, tg.NewInlineKeyboardRow(flowButton(f, "🙋 Faqat o'zim", "alone")))
	case "reason":
		text = "Arizani nima sababdan bekor qilmoqchisiz? Sababni yozing, keyingi bosqichda tasdiqlaysiz. Ish beruvchi yoki ishchiga sabab yetkaziladi."
	case "confirm":
		switch f.Purpose {
		case "apply":
			text = fmt.Sprintf("Arizani tekshiring\n\n%s\n📅 %s · %s (Toshkent)\n💰 %s\n👥 %d kishi\n👤 %s\n📞 %s\n\nIsmingiz va telefoningiz ish beruvchiga yuboriladi. Qabul qilinganingiz haqida javob kelguncha kuting.", f.Job.Title, f.Job.StartDate, f.Job.WorkTimeFrom, jobPay(f.Job.NearbyJob), f.People, f.Profile.FirstName, f.Profile.Phone)
		case "cancel":
			text = "Arizani bekor qilish\n\nIsh: " + f.App.ElonTitle + "\nSabab: " + f.Reason + "\n\nTasdiqlasangiz, ariza bekor qilinadi va ikkinchi tomon xabardor bo'ladi."
		case "done":
			text = "Ish: " + f.App.ElonTitle + "\n\nIsh haqiqatan tugadimi? Tasdiqlasangiz, ikkinchi tomondan ham tasdiq so'raladi. Bu tugma pul to'langanini tasdiqlamaydi."
		case "accept":
			text = fmt.Sprintf("%s\nNomzod: %s\nKishi soni: %d\n\nNomzodni ishga qabul qilishni tasdiqlang.", f.App.ElonTitle, f.App.WorkerName, f.App.PeopleCount)
		case "reject":
			text = "Ish: " + f.App.ElonTitle + "\n\nNomzod arizasini rad etishni tasdiqlang."
		}
		rows = append(rows, tg.NewInlineKeyboardRow(flowButton(f, "✅ Tasdiqlash", "confirm")))
	default:
		text = "Joriy holatni /applications orqali tekshiring."
	}
	rows = append(rows, tg.NewInlineKeyboardRow(workerButton("⬅️ Bosh menyu", "home", "")))
	return e.workerMessage(d.ChatID, text, rows...)
}

func (e *Engine) commitWorker(ctx context.Context, d *Draft, update int) error {
	f := d.Worker
	s, err := e.workerSession(ctx, d)
	if err != nil {
		return e.say(d.ChatID, workerError(err))
	}
	if f.Purpose != "apply" {
		a, err := e.API.Application(ctx, s, f.ID)
		if err != nil {
			return e.say(d.ChatID, workerError(err))
		}
		if !workerActionAllowed(f.Purpose, a, s.User.ID) {
			return e.say(d.ChatID, "Ariza holati o'zgargan. /applications yoki /candidates orqali yangilang.")
		}
	}
	// Save before the write: a crash/timeout must never replay an uncertain
	// submission, cancellation or decision from an old confirmation button.
	f.Step = "submitted"
	if err := e.saveWorker(ctx, d, update); err != nil {
		return err
	}
	message := ""
	appID := f.ID
	if f.Purpose == "apply" {
		var a Application
		a, err = e.API.Apply(ctx, s, f.ID, f.People)
		appID = a.ID
		message = "✅ Arizangiz yuborildi. Ish beruvchining javobini kuting. Holatini «Arizani ochish» orqali tekshiring."
	} else {
		action := f.Purpose
		if action == "done" {
			action = "confirm-done"
		}
		var status string
		status, err = e.API.ApplicationAction(ctx, s, f.ID, action, f.Reason)
		message = map[string]string{"cancel": "Ariza bekor qilindi. Sabab ikkinchi tomonga yuborildi.", "accept": "✅ Nomzod qabul qilindi va xabardor qilindi.", "reject": "Ariza rad etildi. Nomzod xabardor qilindi.", "done": "✅ Tasdig'ingiz saqlandi. Ikkinchi tomon tasdig'ini kuting."}[f.Purpose]
		if status == "completed" {
			message = "🏁 Ish yakunlandi va tarixga o'tdi."
		}
	}
	if err != nil {
		if definiteRejection(err) {
			d.Worker = nil
			if saveErr := e.saveWorker(ctx, d, update); saveErr != nil {
				return saveErr
			}
			return e.say(d.ChatID, workerError(err))
		}
		return e.say(d.ChatID, "So'rov natijasini tasdiqlab bo'lmadi. /applications yoki /candidates orqali holatni tekshiring. Xuddi shu amal avtomatik takrorlanmaydi.")
	}
	d.Worker = nil
	if err := e.saveWorker(ctx, d, update); err != nil {
		return err
	}
	rows := [][]tg.InlineKeyboardButton{tg.NewInlineKeyboardRow(workerButton("Bosh menyu", "home", ""))}
	if validWorkerID(appID) {
		rows = append([][]tg.InlineKeyboardButton{tg.NewInlineKeyboardRow(workerButton("Arizani ochish", "app", appID))}, rows...)
	}
	return e.workerMessage(d.ChatID, message, rows...)
}

func jobPay(j NearbyJob) string {
	if j.PricingType == "negotiable" || j.PerWorkerAmount <= 0 {
		return "Kelishiladi"
	}
	return fmt.Sprintf("%d so'm / ishchi", j.PerWorkerAmount)
}
func applicationStatus(status string) string {
	label := map[string]string{"pending": "⏳ Javob kutilmoqda", "accepted": "✅ Qabul qilingan", "rejected": "Ariza rad etilgan", "cancelled": "Ariza bekor qilingan", "completed": "🏁 Ish yakunlangan"}[status]
	if label == "" {
		return "Holatni yangilang"
	}
	return label
}
func workerJobSummary(j Job, now time.Time) string {
	date, clock := jobDateTime(j)
	if parsed, err := time.Parse("2006-01-02", date); err == nil {
		date = parsed.Format("02.01.2006")
	}
	if date == "" {
		date = "Sana ko'rsatilmagan"
	}
	if clock == "" {
		clock = "Vaqt kelishiladi"
	}
	if j.WorkTimeTo != "" {
		clock += "–" + j.WorkTimeTo
	}
	phone := strings.TrimSpace(j.ContactPhone)
	if phone == "" {
		phone = "Ko'rsatilmagan"
	}
	text := fmt.Sprintf("%s\n\n💰 %s\n📅 %s · %s (Toshkent)\n👥 Kerak: %d kishi · Qabul qilingan: %d\n🟢 Bo'sh o'rin: %d\n🏷 %s\n📍 %s, %s\n%s\n👤 Ish beruvchi: %s · reyting %.1f\n📞 Telefon: %s", j.Title, jobPay(j.NearbyJob), date, clock, j.WorkersNeeded, j.AcceptedCount, max(0, j.WorkersNeeded-j.AcceptedCount), j.CategoryName, j.Region, j.District, j.LocationText, j.OwnerName, j.OwnerRating, phone)
	if !jobOpen(j, now) {
		text += "\n\nℹ️ Bu ish hozir ariza qabul qilmayapti. Joylar to'lgan yoki e'lon muddati tugagan."
	}
	return text
}
func workerJobText(j Job, now time.Time) string {
	return workerJobSummary(j, now) + "\n\n" + j.Description
}
func (e *Engine) showWorkerJob(ctx context.Context, chat int64, j Job) error {
	text := workerJobText(j, e.now())
	caption, shortened := workerJobCaption(j, e.now())
	photos, loadErr := e.jobPhotos(ctx, j.Images)
	if loadErr != nil {
		text = "Rasmlarni hozir yuklab bo'lmadi. «Joylarni yangilash» tugmasi orqali qayta urinib ko'ring.\n\n" + text
	}
	if len(photos) > 0 {
		if err := e.sendJobPhotos(chat, photos, caption); err != nil {
			return err
		}
	} else if err := e.sendText(chat, text); err != nil {
		return err
	}
	rows := [][]tg.InlineKeyboardButton{}
	if len(photos) > 0 && shortened {
		rows = append(rows, tg.NewInlineKeyboardRow(workerButton("📖 To'liq tavsif", "jobtext", j.ID)))
	}
	if jobOpen(j, e.now()) {
		rows = append(rows, tg.NewInlineKeyboardRow(workerButton("🙋 Ariza berish", "apply", j.ID)))
	}
	if validCoordinates(j.Lat, j.Lng) {
		if _, err := e.Bot.Send(tg.NewLocation(chat, j.Lat, j.Lng)); err != nil {
			return err
		}
		rows = append(rows, tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonURL("🗺 Xaritada ochish", fmt.Sprintf("https://www.google.com/maps?q=%f,%f", j.Lat, j.Lng))))
	}
	rows = append(rows, tg.NewInlineKeyboardRow(workerButton("🔄 Joylarni yangilash", "job", j.ID), workerButton("📋 Arizalarim", "apps", "all")))
	return e.workerMessage(chat, "Davom etish uchun tanlang:", rows...)
}

func jobDateTime(j Job) (string, string) {
	date, clock := j.StartDate, j.WorkTimeFrom
	if len(date) > 10 {
		clock = ""
		if len(date) >= 16 {
			clock = date[11:16]
		}
	}
	if len(date) >= 10 {
		date = date[:10]
	}
	return date, clock
}
func jobOpen(j Job, now time.Time) bool {
	if j.Status != "recruiting" || j.WorkersNeeded <= j.AcceptedCount {
		return false
	}
	date, clock := jobDateTime(j)
	if clock == "" && len(j.StartDate) <= 10 {
		clock = "23:59"
	}
	start, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, tashkent)
	// Match the API feed's six-hour grace and legacy malformed-date fallback.
	return err != nil || !start.Before(now.Add(-6*time.Hour))
}
func (e *Engine) showApplications(chat int64, f *WorkerFlow, apps []Application) error {
	label, action := "📋 Arizalarim", "apps"
	if f.Employer {
		label, action = "📨 Kelgan arizalar", "inbox"
	}
	if err := e.workerMessage(chat, fmt.Sprintf("%s · %d-sahifa", label, f.Page)); err != nil {
		return err
	}
	for _, a := range apps {
		name := a.OwnerName
		if f.Employer {
			name = a.WorkerName
		}
		text := fmt.Sprintf("%s\n%s\n👥 %d kishi · %s", a.ElonTitle, applicationStatus(a.Status), a.PeopleCount, name)
		if err := e.workerMessage(chat, text, tg.NewInlineKeyboardRow(workerButton("Arizani ochish", "app", a.ID))); err != nil {
			return err
		}
	}
	text := "Holat bo'yicha tanlang yoki ro'yxatni yangilang."
	if len(apps) == 0 {
		text = "Bu bo'limda ariza yo'q. Yaqin ishlarni /jobs orqali topishingiz mumkin."
	}
	rows := [][]tg.InlineKeyboardButton{tg.NewInlineKeyboardRow(workerButton("Barchasi", action, "all"), workerButton("Kutilmoqda", action, "pending")), tg.NewInlineKeyboardRow(workerButton("Qabul qilingan", action, "accepted"), workerButton("Tarix", action, "history"))}
	nav := []tg.InlineKeyboardButton{}
	if f.Page > 1 {
		nav = append(nav, workerButton("← Oldingi", action, fmt.Sprintf("%s:%d", f.Filter, f.Page-1)))
	}
	if len(apps) == 5 && f.Page < 1000 {
		nav = append(nav, workerButton("Keyingi →", action, fmt.Sprintf("%s:%d", f.Filter, f.Page+1)))
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	rows = append(rows, tg.NewInlineKeyboardRow(workerButton("🔄 Yangilash", action, fmt.Sprintf("%s:%d", f.Filter, f.Page)), workerButton("Bosh menyu", "home", "")))
	return e.workerMessage(chat, text, rows...)
}
func (e *Engine) showApplication(ctx context.Context, chat int64, s Session, a Application) error {
	pay := "Kelishiladi"
	if !a.IsNegotiable && a.Amount > 0 {
		pay = fmt.Sprintf("%d so'm / ishchi", a.Amount)
	}
	text := fmt.Sprintf("%s\n\n%s\n💰 %s\n👥 %d kishi\n👤 Ish beruvchi: %s\n👤 Ishchi: %s", a.ElonTitle, applicationStatus(a.Status), pay, a.PeopleCount, a.OwnerName, a.WorkerName)
	if a.CancelReason != "" {
		text += "\nSabab: " + a.CancelReason
	}
	rows := [][]tg.InlineKeyboardButton{}
	if a.Status == "accepted" {
		job, err := e.API.Job(ctx, s, a.ElonID)
		if err == nil {
			text += fmt.Sprintf("\n\n📅 %s · %s (Toshkent)\n📍 %s, %s\n%s\n📞 Ish beruvchi: %s", job.StartDate, job.WorkTimeFrom, job.Region, job.District, job.LocationText, job.ContactPhone)
			if validCoordinates(job.Lat, job.Lng) {
				rows = append(rows, tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonURL("🗺 Ishga borish yo'li", fmt.Sprintf("https://www.google.com/maps/dir/?api=1&destination=%f,%f", job.Lat, job.Lng))))
			}
		} else {
			text += "\n\nIsh manzilini hozir yuklab bo'lmadi. Yangilash tugmasi orqali qayta tekshiring."
		}
		if a.WorkerConfirmedDone {
			text += "\n✅ Ishchi tugaganini tasdiqlagan."
		}
		if a.EmployerConfirmedDone {
			text += "\n✅ Ish beruvchi tugaganini tasdiqlagan."
		}
	}
	if a.EmployerID == s.User.ID {
		text += "\n📞 Nomzod: " + a.WorkerPhone
	}
	for _, item := range []struct{ action, label string }{{"accept", "✅ Qabul qilish"}, {"reject", "Rad etish"}, {"done", "🏁 Ishni tugatdim"}, {"cancel", "Arizani bekor qilish"}} {
		if workerActionAllowed(item.action, a, s.User.ID) {
			rows = append(rows, tg.NewInlineKeyboardRow(workerButton(item.label, item.action, a.ID)))
		}
	}
	rows = append(rows, tg.NewInlineKeyboardRow(workerButton("🔄 Yangilash", "app", a.ID), workerButton("Bosh menyu", "home", "")))
	return e.workerMessage(chat, text, rows...)
}
func workerError(err error) string {
	switch errorCode(err) {
	case "duplicate":
		return "Siz bu ishga allaqachon ariza yuborgansiz. /applications orqali holatini ko'ring."
	case "self_apply":
		return "O'zingizning e'loningizga ariza yubora olmaysiz."
	case "already_done":
		return "Bu ish allaqachon yakunlangan. /jobs orqali boshqa ish toping."
	case "not_found":
		return "E'lon yoki ariza topilmadi, yoxud uni ko'rish huquqingiz yo'q. /menu orqali yangilang."
	case "forbidden":
		return "Bu amalni bajarish huquqingiz yo'q."
	case "bad_state", "state_changed":
		return "Ariza holati o'zgargan. /applications yoki /candidates orqali yangilang."
	}
	return userError(err)
}
