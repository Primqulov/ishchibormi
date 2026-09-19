package posting

import (
	"testing"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func hasCallback(messages []tg.Chattable, data string) bool {
	for _, raw := range messages {
		if m, ok := raw.(tg.MessageConfig); ok {
			if kb, ok := m.ReplyMarkup.(tg.InlineKeyboardMarkup); ok {
				for _, row := range kb.InlineKeyboard {
					for _, b := range row {
						if b.CallbackData != nil && *b.CallbackData == data {
							return true
						}
					}
				}
			}
		}
	}
	return false
}
func TestChannelLinkOpensExactLiveJobWithoutLoginAndOffersApplication(t *testing.T) {
	h := workerSetup(t)
	h.a.newUser = true
	h.text("/start job_" + workerJobID)
	if h.a.jobCalls != 1 || h.a.lastJobID != workerJobID || h.a.logins != 0 || h.a.applyCalls != 0 {
		t.Fatal("channel link did not open exact public job")
	}
	for _, part := range []string{"Kunlik ish", "Bo'sh o'rin: 3", "Qabul qilingan: 1", "+998901112233", "17.09.2026"} {
		if !h.hasText(part) {
			t.Fatal("missing live detail", part)
		}
	}
	mapShown := false
	for _, raw := range h.b.messages {
		if loc, ok := raw.(tg.LocationConfig); ok {
			mapShown = loc.Latitude == 41.3 && loc.Longitude == 69.2
		}
	}
	if !mapShown || !hasCallback(h.b.messages, "w:apply:"+workerJobID) {
		t.Fatal("native map/apply missing")
	}
	count := len(h.b.messages)
	h.a.job.AcceptedCount = 4
	h.a.job.Status = "filled"
	h.workerClick("job", workerJobID)
	if hasCallback(h.b.messages[count:], "w:apply:"+workerJobID) || !h.hasText("Bo'sh o'rin: 0") {
		t.Fatal("stale vacancy count")
	}
	// An old apply button still checks live availability before offering a form.
	h.a.newUser = false
	h.workerClick("apply", workerJobID)
	if h.a.applyCalls != 0 || !h.hasText("hozir ariza qabul qilmayapti") {
		t.Fatal("filled job accepted application")
	}
}
func TestChannelLinkPreservesDraftAndRejectsMissingMalformedOrExpiredJobs(t *testing.T) {
	h := workerSetup(t)
	h.formToPreview("total")
	title := h.s.draft.Form.Title
	h.text("/start job_" + workerJobID)
	if h.s.draft.Form.Title != title || h.s.draft.Step != "preview" {
		t.Fatal("deep link erased employer draft")
	}
	calls := h.a.jobCalls
	h.text("/start job_not-an-id")
	if h.a.jobCalls != calls {
		t.Fatal("malformed deep link reached API")
	}
	h.a.workerErr = &APIError{Status: 404, Code: "not_found"}
	h.text("/start job_" + workerJobID)
	if !h.hasText("yopilgan yoki endi mavjud emas") {
		t.Fatal("closed job not explained")
	}
	h.a.workerErr = nil
	h.a.job.StartDate = "2020-01-01"
	count := len(h.b.messages)
	h.text("/start job_" + workerJobID)
	if hasCallback(h.b.messages[count:], "w:apply:"+workerJobID) {
		t.Fatal("expired job offered apply")
	}
}
func TestJobOpenMatchesFeedGraceAndLegacySchedule(t *testing.T) {
	now := time.Date(2026, 9, 17, 15, 0, 0, 0, tashkent)
	for _, tc := range []struct {
		date, clock string
		open        bool
	}{{"2026-09-17", "09:00", true}, {"2026-09-17", "08:59", false}, {"2026-09-17T08:59:00", "18:00", false}, {"2026-09-17", "", true}, {"legacy", "", true}} {
		j := Job{NearbyJob: NearbyJob{WorkersNeeded: 1, StartDate: tc.date, WorkTimeFrom: tc.clock}, Status: "recruiting"}
		if jobOpen(j, now) != tc.open {
			t.Fatalf("schedule mismatch: %+v", tc)
		}
	}
}
