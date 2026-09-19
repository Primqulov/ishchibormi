package posting

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func searchResult() NearbyPage {
	return NearbyPage{Page: 1, Limit: 5, Total: 9, Items: []NearbyJob{
		{ID: "123456789012345678901234", Title: "Hovli tozalash", Description: "Tozalash", Lat: 41.301, Lng: 69.2, DistanceMeters: 111.2, WorkersNeeded: 3, AcceptedCount: 1, PricingType: "per_worker", PerWorkerAmount: 150000, StartDate: "2026-09-17", WorkTimeFrom: "09:00", Region: "Toshkent", District: "Yunusobod", Gender: "mixed"},
		{ID: "123456789012345678901235", Title: "Yuk tushirish", DistanceMeters: 1250, WorkersNeeded: 2, PricingType: "negotiable", Lat: 41.31, Lng: 69.2},
	}}
}
func (h *harness) location(lat, lng float64) {
	h.t.Helper()
	m := h.message("")
	m.Location = &tg.Location{Latitude: lat, Longitude: lng}
	h.send(m)
}
func (h *harness) searchClick(action string, page int) {
	h.t.Helper()
	s, ok := h.e.Searches.get(42, h.e.now())
	if !ok {
		h.t.Fatal("search session missing")
	}
	h.update++
	h.rawClick(fmt.Sprintf("jobs:%s:%s:%d", s.Key, action, page))
}

func TestSearchLocationShowsNearbyJobsWithoutLogin(t *testing.T) {
	h := setup(t)
	h.a.nearbyResult = searchResult()
	h.text("/jobs")
	h.location(41.3, 69.2)
	if h.a.nearbyCalls != 1 || h.a.nearbyLat != 41.3 || h.a.nearbyLng != 69.2 || h.a.nearbyPage != 1 || h.a.logins != 0 {
		t.Fatal("location did not reach public nearby API")
	}
	var cards []tg.MessageConfig
	for _, raw := range h.b.messages {
		if m, ok := raw.(tg.MessageConfig); ok && m.ParseMode == "HTML" {
			cards = append(cards, m)
		}
	}
	if len(cards) != 2 || !strings.Contains(cards[0].Text, "111 m") || !strings.Contains(cards[0].Text, "150000") || !strings.Contains(cards[0].Text, "Bo'sh o'rin: 2") || !strings.Contains(cards[1].Text, "1.2 km") {
		t.Fatalf("incomplete search cards: %+v", cards)
	}
	buttons := cards[0].ReplyMarkup.(tg.InlineKeyboardMarkup).InlineKeyboard[0]
	if !strings.HasSuffix(*buttons[0].URL, "/elon/123456789012345678901234") || !strings.Contains(*buttons[1].URL, "41.301000,69.200000") {
		t.Fatal("listing/map links missing")
	}
	if h.s.draft.Form.Lat != 0 || h.s.draft.HasLocation {
		t.Fatal("worker coordinates were saved in employer draft")
	}
}

func TestSearchPaginationAndNewLocationInvalidateOldButtons(t *testing.T) {
	h := setup(t)
	h.a.nearbyResult = searchResult()
	h.location(41.3, 69.2)
	first, _ := h.e.Searches.get(42, h.e.now())
	h.searchClick("page", 2)
	if h.a.nearbyPage != 2 {
		t.Fatal("next page did not reach API")
	}
	calls := h.a.nearbyCalls
	h.update++
	h.rawClick("jobs:" + first.Key + ":page:2")
	if h.a.nearbyCalls != calls {
		t.Fatal("old button ran another request")
	}
	h.location(40.1, 68.2)
	if h.a.nearbyPage != 1 || h.a.nearbyLat != 40.1 {
		t.Fatal("new location did not reset pagination")
	}
	h.searchClick("refresh", 0)
	if h.a.nearbyPage != 1 {
		t.Fatal("refresh did not start at nearest result")
	}
}

func TestSearchKeepsLegacyDraftAndPostOpensMiniApp(t *testing.T) {
	h := setup(t)
	h.formToPreview("total")
	h.a.nearbyResult = searchResult()
	h.text("/jobs")
	h.location(40, 70)
	if h.s.draft.Form.Lat != 41.3 || h.s.draft.Form.Title != "Hovli tozalash" {
		t.Fatal("search changed legacy draft")
	}
	h.text("/post")
	h.step("menu")
	if _, ok := h.e.Searches.get(42, h.e.now()); ok {
		t.Fatal("Mini App launch did not exit search")
	}
}

func TestCancelSearchKeepsPostingDraftAndRejectsOldResults(t *testing.T) {
	h := setup(t)
	h.formToPreview("total")
	h.text("/jobs")
	h.a.nearbyResult = searchResult()
	h.location(41.3, 69.2)
	s, _ := h.e.Searches.get(42, h.e.now())
	h.text("/cancel")
	h.step("preview")
	h.update++
	h.rawClick("jobs:" + s.Key + ":page:2")
	if h.a.nearbyCalls != 1 || h.a.removed != 0 {
		t.Fatal("cancelled search changed employer data")
	}
}

func TestSearchHandlesEmptyAndFailedRequestsWithRetry(t *testing.T) {
	h := setup(t)
	h.a.nearbyError = errors.New("offline")
	h.text("/jobs")
	h.location(41.3, 69.2)
	if _, ok := h.e.Searches.get(42, h.e.now()); !ok {
		t.Fatal("failed search lost coordinates")
	}
	h.a.nearbyError = nil
	h.a.nearbyResult = NearbyPage{Items: []NearbyJob{}, Page: 1, Limit: 5}
	h.searchClick("refresh", 0)
	found := false
	for _, raw := range h.b.messages {
		if m, ok := raw.(tg.MessageConfig); ok && strings.Contains(m.Text, "faol ish e'lonlari topilmadi") {
			found = true
		}
	}
	if !found || h.a.nearbyCalls != 2 {
		t.Fatal("empty state/retry missing")
	}
}

func TestSearchRejectsForeignUpdatesAndInvalidLocations(t *testing.T) {
	h := setup(t)
	h.text("/jobs")
	h.location(math.NaN(), 69)
	h.location(91, 69)
	m := h.message("")
	m.Location = &tg.Location{Latitude: 41.3, Longitude: 69.2}
	m.Chat.Type = "group"
	h.send(m)
	m.Chat.Type = "private"
	m.From.ID = 99
	h.send(m)
	if h.a.nearbyCalls != 0 {
		t.Fatal("invalid/foreign location reached API")
	}
}

func TestSearchExpiresAndDoesNotSurviveRestart(t *testing.T) {
	h := setup(t)
	h.a.nearbyResult = searchResult()
	h.location(41.3, 69.2)
	s, _ := h.e.Searches.get(42, h.e.now())
	if _, ok := h.e.Searches.get(42, h.e.now().Add(2*time.Hour)); ok {
		t.Fatal("location retained past expiry")
	}
	h.e.Searches = NewSearchSessions()
	h.update++
	h.rawClick("jobs:" + s.Key + ":page:2")
	if h.a.nearbyCalls != 1 {
		t.Fatal("stale coordinates survived restart")
	}
}

func TestSearchCardEscapesListingTextAndShowsLocalISOTime(t *testing.T) {
	j := searchResult().Items[0]
	j.Title = "<script>&"
	j.Description = strings.Repeat("😀", 1000) + "<b>"
	j.StartDate = "2026-09-17T13:45:00"
	j.WorkTimeFrom = "09:00"
	card := nearbyCard(j, 1)
	if strings.Contains(card, "<script>") || !strings.Contains(card, "&lt;script&gt;&amp;") || !strings.Contains(card, "13:45") || len([]rune(card)) > 2000 {
		t.Fatalf("unsafe/incorrect card %s", card)
	}
}

func TestSearchMenuCallbackAndDeepLink(t *testing.T) {
	for _, mode := range []string{"callback", "link"} {
		t.Run(mode, func(t *testing.T) {
			h := setup(t)
			h.text("/start")
			if mode == "callback" {
				h.update++
				h.rawClick("jobs:start")
			} else {
				h.text("/start jobs")
			}
			if _, ok := h.e.Searches.get(42, h.e.now()); !ok {
				t.Fatal("menu did not start location request")
			}
		})
	}
}

func TestDuplicateSearchUpdateDoesNotSendResultsTwice(t *testing.T) {
	h := setup(t)
	h.a.nearbyResult = searchResult()
	h.location(41.3, 69.2)
	m := h.message("")
	m.Location = &tg.Location{Latitude: 41.3, Longitude: 69.2}
	if err := h.e.Handle(context.Background(), tg.Update{UpdateID: h.update, Message: m}); err != nil {
		t.Fatal(err)
	}
	if h.a.nearbyCalls != 1 {
		t.Fatal("replayed location duplicated search")
	}
}
