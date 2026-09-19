package posting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type memoryStore struct {
	draft *Draft
	fail  bool
}

func clone(d *Draft) *Draft {
	if d == nil {
		return nil
	}
	b, _ := json.Marshal(d)
	var out Draft
	_ = json.Unmarshal(b, &out)
	return &out
}
func (s *memoryStore) Load(context.Context, int64) (*Draft, error) { return clone(s.draft), nil }
func (s *memoryStore) Save(_ context.Context, d *Draft) error {
	if s.fail {
		return errors.New("database unavailable")
	}
	s.draft = clone(d)
	return nil
}

type fakeBot struct {
	messages []tg.Chattable
	requests []tg.Chattable
}

func (b *fakeBot) Send(m tg.Chattable) (tg.Message, error) {
	b.messages = append(b.messages, m)
	return tg.Message{}, nil
}
func (b *fakeBot) Request(m tg.Chattable) (*tg.APIResponse, error) {
	b.requests = append(b.requests, m)
	return &tg.APIResponse{Ok: true}, nil
}

type fakeAPI struct {
	lastJobID                               string
	jobCalls                                int
	job                                     Job
	application                             Application
	applications                            []Application
	applyCalls, actionCalls, appliedPeople  int
	lastAction, lastReason, appFilter       string
	appPage                                 int
	appEmployer                             bool
	workerErr                               error
	searchFilter                            SearchFilter
	nearbyCalls                             int
	nearbyLat, nearbyLng                    float64
	nearbyPage                              int
	nearbyResult                            NearbyPage
	nearbyError                             error
	profile                                 Profile
	newUser                                 bool
	loginErr, errorPublish, errorCategories error
	logins, published, uploads, removed     int
	keys                                    []string
	forms                                   []Form

	alert                    *JobAlert
	alertErr                 error
	alertSaves, alertDeletes int
}

func (a *fakeAPI) Nearby(_ context.Context, lat, lng float64, page int, filters ...SearchFilter) (NearbyPage, error) {
	if len(filters) > 0 {
		a.searchFilter = filters[0]
	}
	a.nearbyCalls++
	a.nearbyLat, a.nearbyLng, a.nearbyPage = lat, lng, page
	return a.nearbyResult, a.nearbyError
}

func (a *fakeAPI) Login(_ context.Context, _ int64, phone string) (Session, error) {
	a.logins++
	if a.loginErr != nil {
		return Session{}, a.loginErr
	}
	if a.newUser && phone == "" {
		return Session{}, &APIError{Status: 401, Code: "contact_required"}
	}
	if phone != "" {
		a.newUser = false
		a.profile.Phone = phone
	}
	return Session{AccessToken: "test-token", User: a.profile}, nil
}
func (a *fakeAPI) Categories(context.Context) ([]Category, error) {
	return []Category{{ID: "123456789012345678901234", Name: "Qurilish"}}, a.errorCategories
}
func (a *fakeAPI) SaveProfile(_ context.Context, _ Session, p Profile) error {
	a.profile = p
	return nil
}
func (a *fakeAPI) Upload(context.Context, Session, []byte) (string, error) {
	a.uploads++
	return fmt.Sprintf("https://files.test/%d.jpg", a.uploads), nil
}
func (a *fakeAPI) RemoveUpload(context.Context, Session, string) error { a.removed++; return nil }
func (a *fakeAPI) Publish(_ context.Context, _ Session, key string, f Form) (Listing, error) {
	a.published++
	a.keys = append(a.keys, key)
	a.forms = append(a.forms, f)
	return Listing{ID: "posted-id"}, a.errorPublish
}

type harness struct {
	e      *Engine
	s      *memoryStore
	a      *fakeAPI
	b      *fakeBot
	update int
	t      *testing.T
}

func setup(t *testing.T) *harness {
	s := &memoryStore{}
	a := &fakeAPI{profile: Profile{ID: "owner", Phone: "+998901234567", FirstName: "Ali", LastName: "Karimov", Region: "Toshkent", District: "Yunusobod"}}
	b := &fakeBot{}
	e := &Engine{Store: s, API: a, Bot: b, WebURL: "https://ishchibormi.uz", Searches: NewSearchSessions(), Now: func() time.Time { return time.Date(2026, 9, 16, 10, 0, 0, 0, tashkent) }, MiniAppURL: "https://mini.test/miniapp/post"}
	return &harness{e: e, s: s, a: a, b: b, t: t}
}
func (h *harness) message(text string) *tg.Message {
	m := &tg.Message{Chat: &tg.Chat{ID: 42, Type: "private"}, From: &tg.User{ID: 42}, Text: text}
	if strings.HasPrefix(text, "/") {
		word := strings.Split(text, " ")[0]
		m.Entities = []tg.MessageEntity{{Type: "bot_command", Offset: 0, Length: len(word)}}
	}
	return m
}
func (h *harness) send(m *tg.Message) {
	h.t.Helper()
	h.update++
	if err := h.e.Handle(context.Background(), tg.Update{UpdateID: h.update, Message: m}); err != nil {
		h.t.Fatal(err)
	}
}
func (h *harness) text(s string) { h.t.Helper(); h.send(h.message(s)) }
func (h *harness) rawClick(data string) {
	h.t.Helper()
	if err := h.e.Handle(context.Background(), tg.Update{UpdateID: h.update, CallbackQuery: &tg.CallbackQuery{ID: "cb", From: &tg.User{ID: 42}, Message: h.message("bot prompt"), Data: data}}); err != nil {
		h.t.Fatal(err)
	}
}
func (h *harness) step(want string) {
	h.t.Helper()
	if h.s.draft == nil || h.s.draft.Step != want {
		h.t.Fatalf("want step %s, got %+v", want, h.s.draft)
	}
}

// Represents a pre-Mini-App draft already stored before migration.
func (h *harness) formToPreview(pricing string) {
	h.t.Helper()
	h.s.draft = freshDraft(42)
	h.s.draft.Step = "preview"
	h.s.draft.Form = Form{Title: "Hovli tozalash", Description: "Hovlini tozalash.", PricingType: pricing, Lat: 41.3, Lng: 69.2}
}

// Ish signali uchun soxta API. Saqlangan qiymat testda tekshiriladi.
func (a *fakeAPI) JobAlert(context.Context, Session) (*JobAlert, error) {
	return a.alert, a.alertErr
}
func (a *fakeAPI) SaveJobAlert(_ context.Context, _ Session, alert JobAlert) (*JobAlert, error) {
	a.alertSaves++
	if a.alertErr != nil {
		return nil, a.alertErr
	}
	saved := alert
	if saved.CategoryID != "" {
		saved.CategoryName = "Qurilish"
	}
	a.alert = &saved
	return a.alert, nil
}
func (a *fakeAPI) DeleteJobAlert(context.Context, Session) error {
	a.alertDeletes++
	a.alert = nil
	return a.alertErr
}
