package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ishchibormi/backend/pkg/elonquery"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Kalitning butun aylanmasi: standart holat → yoqish → restartdan keyin
// bazadan qayta o'qish → o'chirish. `expiredCount` har ikki holatda ham
// AYNAN muddati o'tgan e'lonlarni sanashi kerak — yashirilgan yoki yangi
// e'lon unga qo'shilmaydi.
func TestFeedSettingsRoundTrip(t *testing.T) {
	db := filterDB(t)
	h := &Handler{
		Admins:   db.Collection("admins"),
		Elons:    db.Collection("elons"),
		AuditCol: db.Collection("admin_audit"),
	}
	t.Cleanup(func() { elonquery.SetShowExpired(false) })
	ctx := context.Background()
	adminID := primitive.NewObjectID()
	if _, err := h.Admins.InsertOne(ctx, bson.M{"_id": adminID, "username": "boss", "role": "superadmin"}); err != nil {
		t.Fatal(err)
	}
	uz := time.FixedZone("UZT", 5*3600)
	past := time.Now().In(uz).AddDate(0, 0, -3).Format("2006-01-02")
	future := time.Now().In(uz).AddDate(0, 0, 1).Format("2006-01-02")
	if _, err := h.Elons.InsertMany(ctx, []any{
		bson.M{"_id": primitive.NewObjectID(), "status": "recruiting", "startDate": future},
		bson.M{"_id": primitive.NewObjectID(), "status": "recruiting", "startDate": past},
		bson.M{"_id": primitive.NewObjectID(), "status": "recruiting", "startDate": past + "T08:00:00"},
		bson.M{"_id": primitive.NewObjectID(), "status": "hidden", "startDate": past},
	}); err != nil {
		t.Fatal(err)
	}

	call := func(method, body string) (int, feedSettingsResponse) {
		t.Helper()
		r := httptest.NewRequest(method, "/", strings.NewReader(body))
		c := context.WithValue(r.Context(), httpx.CtxAdminID, adminID.Hex())
		c = context.WithValue(c, httpx.CtxAdminRole, "superadmin")
		w := httptest.NewRecorder()
		if method == http.MethodGet {
			h.GetFeedSettings(w, r.WithContext(c))
		} else {
			h.SetFeedSettings(w, r.WithContext(c))
		}
		var out feedSettingsResponse
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	if code, got := call(http.MethodGet, ""); code != 200 || got.ShowExpired || got.ExpiredCount != 2 || got.UpdatedAt != nil {
		t.Fatalf("standart holat: %d %+v", code, got)
	}
	// Maydonsiz so'rov kalitni jimgina o'chirib qo'ymasligi kerak.
	if code, _ := call(http.MethodPut, `{}`); code != 400 {
		t.Fatalf("bo'sh tana: %d, 400 kutilgan edi", code)
	}
	if code, got := call(http.MethodPut, `{"showExpired":true}`); code != 200 || !got.ShowExpired || got.ExpiredCount != 2 || got.UpdatedByName != "boss" {
		t.Fatalf("yoqish: %d %+v", code, got)
	}
	if !elonquery.ShowExpired() {
		t.Fatal("yoqilgan kalit filtrga qo'llanmadi")
	}
	// Restart: xotiradagi qiymat yo'qoladi, bazadagisi qaytishi kerak.
	elonquery.SetShowExpired(false)
	if err := h.LoadFeedSettings(ctx); err != nil || !elonquery.ShowExpired() {
		t.Fatalf("bazadan qayta o'qilmadi: err=%v", err)
	}
	if code, got := call(http.MethodPut, `{"showExpired":false}`); code != 200 || got.ShowExpired || elonquery.ShowExpired() {
		t.Fatalf("o'chirish: %d %+v", code, got)
	}
	if n, _ := h.AuditCol.CountDocuments(ctx, bson.M{"action": "feed_show_expired", "adminId": adminID}); n != 2 {
		t.Fatalf("audit yozuvlari: %d, 2 kutilgan edi", n)
	}
}
