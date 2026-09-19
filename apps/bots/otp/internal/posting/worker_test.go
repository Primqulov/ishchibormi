package posting

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const workerJobID = "123456789012345678901234"
const workerAppID = "223456789012345678901234"

func (a *fakeAPI) Job(_ context.Context, _ Session, id string) (Job, error) {
	a.lastJobID = id
	a.jobCalls++
	return a.job, a.workerErr
}
func (a *fakeAPI) Apply(_ context.Context, _ Session, _ string, people int) (Application, error) {
	a.applyCalls++
	a.appliedPeople = people
	return Application{ID: workerAppID, Status: "pending"}, a.workerErr
}
func (a *fakeAPI) Applications(_ context.Context, _ Session, filter string, page int, employer bool) ([]Application, error) {
	a.appFilter, a.appPage, a.appEmployer = filter, page, employer
	return a.applications, a.workerErr
}
func (a *fakeAPI) Application(context.Context, Session, string) (Application, error) {
	return a.application, a.workerErr
}
func (a *fakeAPI) ApplicationAction(_ context.Context, _ Session, _ string, action, reason string) (string, error) {
	a.actionCalls++
	a.lastAction, a.lastReason = action, reason
	return "awaiting_other", a.workerErr
}
func workerSetup(t *testing.T) *harness {
	h := setup(t)
	h.a.job = Job{NearbyJob: NearbyJob{ID: workerJobID, Title: "Kunlik ish", WorkersNeeded: 4, AcceptedCount: 1, StartDate: "2026-09-17", WorkTimeFrom: "09:00", PerWorkerAmount: 150000, Lat: 41.3, Lng: 69.2}, Status: "recruiting", OwnerID: "employer", ContactPhone: "+998901112233"}
	h.a.application = Application{ID: workerAppID, ElonID: workerJobID, ElonTitle: "Kunlik ish", WorkerID: "owner", EmployerID: "employer", PeopleCount: 2, Status: "accepted"}
	return h
}
func (h *harness) workerClick(action, id string) {
	h.t.Helper()
	h.update++
	h.rawClick("w:" + action + ":" + id)
}
func (h *harness) flowClick(action string) {
	h.t.Helper()
	if h.s.draft.Worker == nil {
		h.t.Fatal("missing worker flow")
	}
	h.update++
	h.rawClick("w:flow:" + h.s.draft.Worker.Key + ":" + action)
}
func (h *harness) hasText(fragment string) bool {
	for _, raw := range h.b.messages {
		if m, ok := raw.(tg.MessageConfig); ok && strings.Contains(m.Text, fragment) {
			return true
		}
	}
	return false
}

func TestWorkerApplyOwnContactProfileGroupAndConfirmation(t *testing.T) {
	h := workerSetup(t)
	h.a.newUser = true
	h.a.profile = Profile{ID: "worker"}
	h.workerClick("apply", workerJobID)
	if h.s.draft.Worker.Step != "consent" {
		t.Fatal("new account skipped consent")
	}
	h.flowClick("agree")
	m := h.message("")
	m.Contact = &tg.Contact{UserID: 43, PhoneNumber: "998901234567"}
	h.send(m)
	if h.s.draft.Worker.Step != "contact" {
		t.Fatal("foreign contact accepted")
	}
	m.Contact.UserID = 42
	h.send(m)
	h.text("Ali")
	h.text("Toshkent")
	h.text("Chilonzor")
	if h.s.draft.Worker.Step != "people" || h.a.profile.FirstName != "Ali" {
		t.Fatal("profile not saved")
	}
	h.text("4")
	if h.s.draft.Worker.Step != "people" {
		t.Fatal("exceeded remaining capacity")
	}
	h.text("2")
	if h.a.applyCalls != 0 || h.s.draft.Worker.Step != "confirm" {
		t.Fatal("apply lacked confirmation")
	}
	old := "w:flow:" + h.s.draft.Worker.Key + ":confirm"
	h.flowClick("confirm")
	if h.a.applyCalls != 1 || h.a.appliedPeople != 2 || h.s.draft.Worker != nil {
		t.Fatal("incorrect apply")
	}
	h.update++
	h.rawClick(old)
	if h.a.applyCalls != 1 {
		t.Fatal("old confirmation replayed")
	}
}
func TestWorkerApplyPreservesDraftAndReturnsToPost(t *testing.T) {
	h := workerSetup(t)
	h.formToPreview("total")
	title := h.s.draft.Form.Title
	h.workerClick("apply", workerJobID)
	h.flowClick("alone")
	h.text("/post")
	if h.s.draft.Form.Title != title || h.s.draft.Step != "menu" || h.s.draft.Worker != nil || h.a.applyCalls != 0 {
		t.Fatal("worker flow damaged posting draft")
	}
	h.text("/menu")
	if !h.hasText("Kerakli bo'limni") {
		t.Fatal("menu did not exit posting prompt")
	}
}
func TestWorkerConfirmationChecksIdentityAndPersistsBeforeWrite(t *testing.T) {
	for _, mode := range []string{"identity", "store", "timeout", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			h := workerSetup(t)
			h.workerClick("apply", workerJobID)
			h.flowClick("alone")
			key := h.s.draft.Worker.Key
			switch mode {
			case "identity":
				h.a.profile.ID = "different"
			case "store":
				h.s.fail = true
			case "timeout":
				h.a.workerErr = errors.New("timeout")
			case "duplicate":
				h.a.workerErr = &APIError{Status: 409, Code: "duplicate"}
			}
			h.update++
			err := h.e.Handle(context.Background(), tg.Update{UpdateID: h.update, CallbackQuery: &tg.CallbackQuery{ID: "cb", From: &tg.User{ID: 42}, Message: h.message(""), Data: "w:flow:" + key + ":confirm"}})
			if mode == "store" {
				if err == nil || h.a.applyCalls != 0 {
					t.Fatal("write proceeded without durable guard")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "identity" && h.a.applyCalls != 0 {
				t.Fatal("changed account submitted")
			}
			if mode == "timeout" {
				if h.s.draft.Worker.Step != "submitted" {
					t.Fatal("uncertainty lost")
				}
				h.text("retry")
				if h.a.applyCalls != 1 {
					t.Fatal("uncertain write replayed")
				}
			}
			if mode == "duplicate" && !h.hasText("allaqachon ariza") {
				t.Fatal("duplicate not explained")
			}
		})
	}
}
func TestWorkerApplicationLifecyclePermissionsAndFreshState(t *testing.T) {
	h := workerSetup(t)
	h.workerClick("app", workerAppID)
	if !h.hasText("+998901112233") {
		t.Fatal("accepted worker missing contact")
	}
	h.workerClick("done", workerAppID)
	if h.a.actionCalls != 0 {
		t.Fatal("completion not confirmed")
	}
	h.flowClick("confirm")
	if h.a.lastAction != "confirm-done" || !h.hasText("Ikkinchi tomon") {
		t.Fatal("completion flow failed")
	}
	h.workerClick("cancel", workerAppID)
	h.text("Bora olmayman")
	if h.a.actionCalls != 1 {
		t.Fatal("cancel sent without confirmation")
	}
	h.flowClick("confirm")
	if h.a.lastAction != "cancel" || h.a.lastReason != "Bora olmayman" {
		t.Fatal("reason not delivered")
	}
	h.a.application.WorkerID = "someone_else"
	h.workerClick("done", workerAppID)
	if h.a.actionCalls != 2 {
		t.Fatal("unrelated user mutated application")
	}
	h.a.application.WorkerID = "owner"
	h.workerClick("done", workerAppID)
	h.a.application.Status = "cancelled"
	h.flowClick("confirm")
	if h.a.actionCalls != 2 {
		t.Fatal("stale state completed")
	}
}
func TestWorkerEmployerCanDecideOnlyAfterConfirmation(t *testing.T) {
	h := workerSetup(t)
	h.a.application.EmployerID = "owner"
	h.a.application.WorkerID = "worker"
	h.a.application.Status = "pending"
	h.workerClick("accept", workerAppID)
	if h.a.actionCalls != 0 {
		t.Fatal("early accept")
	}
	h.flowClick("confirm")
	if h.a.lastAction != "accept" {
		t.Fatal("accept missing")
	}
	h.workerClick("reject", workerAppID)
	h.flowClick("confirm")
	if h.a.lastAction != "reject" {
		t.Fatal("reject missing")
	}
}
func TestWorkerListsUseServerFiltersAndPagination(t *testing.T) {
	h := workerSetup(t)
	h.a.applications = []Application{h.a.application}
	h.text("/myjobs")
	if h.a.appFilter != "accepted" || h.a.appEmployer {
		t.Fatal("wrong worker list")
	}
	h.workerClick("apps", "history:2")
	if h.a.appPage != 2 || h.a.appFilter != "history" {
		t.Fatal("pagination dropped filter")
	}
	h.text("/candidates")
	if !h.a.appEmployer || h.a.appFilter != "pending" {
		t.Fatal("employer inbox not filtered")
	}
}
func TestWorkerRestartExpiryAndForgedUpdates(t *testing.T) {
	h := workerSetup(t)
	h.workerClick("apply", workerJobID)
	h.flowClick("alone")
	// The flow survives loading from storage, without storing a bearer token.
	key := h.s.draft.Worker.Key
	h.e.Now = func() time.Time { return time.Date(2026, 9, 18, 10, 0, 0, 0, tashkent) }
	h.update++
	h.rawClick("w:flow:" + key + ":confirm")
	if h.a.applyCalls != 0 {
		t.Fatal("expired confirmation submitted")
	}
	h = workerSetup(t)
	m := h.message("")
	m.Chat.Type = "group"
	if err := h.e.Handle(context.Background(), tg.Update{UpdateID: 1, CallbackQuery: &tg.CallbackQuery{From: &tg.User{ID: 42}, Message: m, Data: "w:apply:" + workerJobID}}); err != nil {
		t.Fatal(err)
	}
	if h.a.logins != 0 {
		t.Fatal("group accepted worker identity")
	}
}
func TestSearchDateCategoryPersistThroughPagesAndResetOnLocation(t *testing.T) {
	h := workerSetup(t)
	h.a.nearbyResult = searchResult()
	h.location(41.3, 69.2)
	s, _ := h.e.Searches.get(42, h.e.now())
	h.update++
	h.rawClick("jobs:" + s.Key + ":day:tomorrow")
	if h.a.searchFilter.Day != "2026-09-17" {
		t.Fatal("Tashkent date missing")
	}
	s, _ = h.e.Searches.get(42, h.e.now())
	h.update++
	h.rawClick("jobs:" + s.Key + ":category:" + workerJobID)
	h.searchClick("page", 2)
	if h.a.searchFilter.Day != "2026-09-17" || h.a.searchFilter.CategoryID != workerJobID {
		t.Fatal("filters lost on page")
	}
	h.location(41.4, 69.3)
	if h.a.nearbyPage != 1 || h.a.searchFilter.CategoryID != workerJobID {
		t.Fatal("location must reset page and preserve filters")
	}
}

// Ro'yxatdan o'tish ariza oqimidan MUSTAQIL bo'lishi kerak: foydalanuvchi
// hech qanday e'lonni tanlamasdan hisob ocha olsin.
func TestWorkerRegisterLinksAccountAndCompletesProfile(t *testing.T) {
	h := workerSetup(t)
	h.a.newUser = true
	h.a.profile = Profile{ID: "worker"}
	h.text("/register")
	if h.s.draft.Worker == nil || h.s.draft.Worker.Step != "consent" {
		t.Fatal("registration skipped consent")
	}
	h.flowClick("agree")
	foreign := h.message("")
	foreign.Contact = &tg.Contact{UserID: 43, PhoneNumber: "998901112233"}
	h.send(foreign)
	if h.s.draft.Worker.Step != "contact" || h.a.profile.Phone != "" {
		t.Fatal("foreign contact created an account")
	}
	own := h.message("")
	own.Contact = &tg.Contact{UserID: 42, PhoneNumber: "998901234567"}
	h.send(own)
	h.text("Ali")
	h.text("Toshkent")
	h.text("Chilonzor")
	if h.s.draft.Worker != nil {
		t.Fatal("registration flow left open")
	}
	if h.a.profile.FirstName != "Ali" || h.a.profile.Region != "Toshkent" || h.a.profile.District != "Chilonzor" {
		t.Fatalf("profile not saved: %+v", h.a.profile)
	}
	// Yakuniy xabar oqimning o'z nusxasidan quriladi: sessiya profil
	// qadamlaridan OLDIN olingani uchun s.User hali bo'sh bo'lardi.
	if !h.hasText("Ro'yxatdan o'tdingiz") || !h.hasText("Ali") || !h.hasText("+998901234567") {
		t.Fatal("registration summary missing name or phone")
	}
}

func TestWorkerRegisterDoesNotPresentExistingAccountAsNewSignup(t *testing.T) {
	h := workerSetup(t) // setup() profili to'liq: Ali / Toshkent / Yunusobod
	h.text("/register")
	if h.s.draft.Worker != nil {
		t.Fatal("complete profile should not open a flow")
	}
	if !h.hasText("allaqachon ro'yxatdan o'tgansiz") {
		t.Fatal("missing already-registered notice")
	}
	if h.hasText("Ro'yxatdan o'tdingiz") {
		t.Fatal("claimed a new signup for an existing account")
	}
}

// Ro'yxatdan o'tish e'lon qoralamasini buzmasligi kerak — ariza oqimi bilan
// bir xil kafolat.
func TestWorkerRegisterPreservesPostingDraft(t *testing.T) {
	h := workerSetup(t)
	h.formToPreview("total")
	title := h.s.draft.Form.Title
	h.text("/register")
	// Profil to'liq, ya'ni oqim darhol yopiladi. Qoralama tegilmasdan qoladi;
	// Step ATAYLAB tekshirilmaydi — ro'yxatdan o'tish e'lon bosqichini
	// o'zgartirmaydi, uni faqat /post qayta boshlaydi.
	if h.s.draft.Form.Title != title || h.s.draft.Worker != nil {
		t.Fatal("registration damaged the posting draft")
	}
}

func TestRegisterButtonAndIDGuard(t *testing.T) {
	if KeyboardCommand(btnRegister) != "register" {
		t.Fatal("menu button does not open registration")
	}
	h := workerSetup(t)
	h.workerClick("register", workerJobID)
	if h.hasText("Ro'yxatdan") {
		t.Fatal("registration accepted a listing ID instead of the fixed marker")
	}
}

// Taklif ichidagi havola tugmasini matni bo'yicha topadi.
func offerURL(h *harness, label string) string {
	for _, raw := range h.b.messages {
		m, ok := raw.(tg.MessageConfig)
		if !ok {
			continue
		}
		kb, ok := m.ReplyMarkup.(tg.InlineKeyboardMarkup)
		if !ok {
			continue
		}
		for _, row := range kb.InlineKeyboard {
			for _, b := range row {
				if strings.Contains(b.Text, label) && b.URL != nil {
					return *b.URL
				}
			}
		}
	}
	return ""
}

// Taklif hisobi YO'Q odamga chiqadi va uchala yo'lni — bot, Android ilova,
// sayt — birga ko'rsatadi.
func TestRegistrationOfferOnlyForAccountlessUserAndListsAllThreeWays(t *testing.T) {
	h := workerSetup(t)
	h.a.newUser = true
	h.a.profile = Profile{ID: "worker"}
	h.text("/start")
	if !h.hasText("Hisobingiz hali ochilmagan") {
		t.Fatal("accountless user was not offered registration")
	}
	if !hasCallback(h.b.messages, "w:register:new") {
		t.Fatal("offer has no in-bot registration button")
	}
	if url := offerURL(h, "Android ilova"); !strings.Contains(url, "play.google.com") {
		t.Fatalf("offer does not point to the mobile app: %q", url)
	}
	if url := offerURL(h, "Sayt"); !strings.HasPrefix(url, "https://ishchibormi.uz") {
		t.Fatalf("offer does not point to the website: %q", url)
	}
	// Taklif ish qidirishni to'smaydi.
	if !h.hasText("Kerakli bo'limni") {
		t.Fatal("offer replaced the main menu")
	}
	if h.s.draft.Registered {
		t.Fatal("accountless user cached as registered")
	}
}

func TestRegistrationOfferSkippedAndCachedForExistingAccount(t *testing.T) {
	h := workerSetup(t) // setup() hisobi bor
	h.text("/start")
	if h.hasText("Hisobingiz hali ochilmagan") {
		t.Fatal("registered user was offered registration")
	}
	if !h.s.draft.Registered || h.a.logins != 1 {
		t.Fatalf("registration state not cached: registered=%v logins=%d", h.s.draft.Registered, h.a.logins)
	}
	h.text("/menu")
	if h.a.logins != 1 {
		t.Fatal("cached registration state still triggered a session request")
	}
}

// Sessiya tekshiruvi yiqilsa bosh menyu baribir ishlashi kerak: taklif
// ikkinchi darajali, uning xatosi menyuni buzmaydi.
func TestRegistrationOfferSkippedWhenSessionCheckFails(t *testing.T) {
	h := workerSetup(t)
	h.a.loginErr = errors.New("api unavailable")
	h.text("/start")
	if !h.hasText("Kerakli bo'limni") {
		t.Fatal("main menu lost when the session check failed")
	}
	if h.hasText("Hisobingiz hali ochilmagan") || h.s.draft.Registered {
		t.Fatal("a failed check was treated as a definite answer")
	}
}
