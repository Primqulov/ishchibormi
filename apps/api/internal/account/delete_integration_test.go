package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ishchibormi/backend/config"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// These exercise the real handlers against a real MongoDB. They skip when no
// local Mongo is reachable, so `go test ./...` still passes on a bare checkout.
func testDB(t *testing.T) *mongo.Database {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cl, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Skipf("no local mongo: %v", err)
	}
	if err := cl.Ping(ctx, nil); err != nil {
		t.Skipf("no local mongo: %v", err)
	}
	db := cl.Database("ishchibormi_accttest")
	t.Cleanup(func() {
		bg := context.Background()
		_ = db.Drop(bg)
		_ = cl.Disconnect(bg)
	})
	return db
}

// asUser returns a request carrying uid the way httpx.UserAuth would.
func asUser(method, target string, body string, uid primitive.ObjectID) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := context.WithValue(r.Context(), httpx.CtxUserID, uid.Hex())
	return r.WithContext(ctx)
}

// seedUser inserts a user plus one live elon and two live applications (one on
// each side of the marketplace) so the wind-down can be asserted.
func seedUser(t *testing.T, db *mongo.Database, tgID int64) primitive.ObjectID {
	t.Helper()
	ctx := context.Background()
	uid := primitive.NewObjectID()

	if _, err := db.Collection("users").InsertOne(ctx, bson.M{
		"_id": uid, "phone": "+998900000001", "telegramId": tgID,
		"firstName": "Test", "isDeleted": false, "createdAt": time.Now(),
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Collection("elons").InsertOne(ctx, bson.M{
		"ownerId": uid, "title": "Test elon", "status": "recruiting", "isDeleted": false,
	}); err != nil {
		t.Fatalf("seed elon: %v", err)
	}
	if _, err := db.Collection("applications").InsertMany(ctx, []any{
		bson.M{"workerId": uid, "employerId": primitive.NewObjectID(), "status": "pending"},
		bson.M{"employerId": uid, "workerId": primitive.NewObjectID(), "status": "accepted"},
	}); err != nil {
		t.Fatalf("seed applications: %v", err)
	}
	return uid
}

func newHandler(db *mongo.Database) *Handler {
	// No bot token: every send fails as ErrUnreachable, which is exactly the
	// "user hasn't pressed /start" branch we want to assert.
	return NewHandler(config.Config{TelegramBotUsername: "Ishchibormibot"}, db, nil)
}

// With no reachable Telegram chat the request must still succeed, reporting
// sent:false plus the bot link the client shows as "press /start there".
func TestRequestDeleteFallsBackToBotLink(t *testing.T) {
	db := testDB(t)
	h := newHandler(db)
	uid := seedUser(t, db, 0)

	w := httptest.NewRecorder()
	h.RequestDelete(w, asUser(http.MethodPost, "/api/me/delete/request", "", uid))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var got requestResp
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Sent {
		t.Fatal("sent = true, want false when the chat is unreachable")
	}
	if got.BotURL != "https://t.me/Ishchibormibot" {
		t.Fatalf("botUrl = %q, want the bot deep link", got.BotURL)
	}
	if got.CodeLength != CodeLength {
		t.Fatalf("codeLength = %d, want %d", got.CodeLength, CodeLength)
	}

	// A code must be stored regardless, so pressing delete again after /start
	// finds a usable record.
	var doc struct {
		Code string `bson:"code"`
	}
	if err := db.Collection("delete_codes").FindOne(context.Background(),
		bson.M{"userId": uid}).Decode(&doc); err != nil {
		t.Fatalf("no code stored: %v", err)
	}
	if len(doc.Code) != CodeLength {
		t.Fatalf("stored code %q has length %d, want %d", doc.Code, len(doc.Code), CodeLength)
	}
}

func TestConfirmDeleteHappyPath(t *testing.T) {
	db := testDB(t)
	h := newHandler(db)
	ctx := context.Background()
	uid := seedUser(t, db, 0)

	h.RequestDelete(httptest.NewRecorder(), asUser(http.MethodPost, "/x", "", uid))
	code := storedCode(t, db, uid)

	w := httptest.NewRecorder()
	h.ConfirmDelete(w, asUser(http.MethodPost, "/x", `{"code":"`+code+`"}`, uid))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}

	// Account flagged...
	var u struct {
		IsDeleted bool `bson:"isDeleted"`
	}
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&u); err != nil {
		t.Fatalf("load user: %v", err)
	}
	if !u.IsDeleted {
		t.Fatal("user.isDeleted = false, want true")
	}

	// ...listings pulled from the feed...
	var e struct {
		Status    string `bson:"status"`
		IsDeleted bool   `bson:"isDeleted"`
	}
	if err := db.Collection("elons").FindOne(ctx, bson.M{"ownerId": uid}).Decode(&e); err != nil {
		t.Fatalf("load elon: %v", err)
	}
	if !e.IsDeleted || e.Status != "cancelled" {
		t.Fatalf("elon = {isDeleted:%v status:%q}, want {true cancelled}", e.IsDeleted, e.Status)
	}

	// ...and nothing left in flight on either side.
	live, err := db.Collection("applications").CountDocuments(ctx, bson.M{
		"$or":    []bson.M{{"workerId": uid}, {"employerId": uid}},
		"status": bson.M{"$in": []string{"pending", "accepted"}},
	})
	if err != nil {
		t.Fatalf("count applications: %v", err)
	}
	if live != 0 {
		t.Fatalf("%d applications still live, want 0", live)
	}
}

// Deleting must release the phone/telegramId so the same number can sign up
// again as a fresh account. The values are unset (not blanked): phone has no
// omitempty, so a "" would stay in the unique index and the next deletion of
// another account would collide with this one.
func TestConfirmDeleteReleasesIdentity(t *testing.T) {
	db := testDB(t)
	h := newHandler(db)
	ctx := context.Background()
	uid := seedUser(t, db, 555001)

	h.RequestDelete(httptest.NewRecorder(), asUser(http.MethodPost, "/x", "", uid))
	w := httptest.NewRecorder()
	h.ConfirmDelete(w, asUser(http.MethodPost, "/x", `{"code":"`+storedCode(t, db, uid)+`"}`, uid))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var raw bson.M
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&raw); err != nil {
		t.Fatalf("load user: %v", err)
	}
	if _, ok := raw["phone"]; ok {
		t.Fatalf("phone still present (%v) — the number cannot be reused", raw["phone"])
	}
	if _, ok := raw["telegramId"]; ok {
		t.Fatalf("telegramId still present (%v)", raw["telegramId"])
	}
	// Archived for support rather than discarded.
	if raw["deletedPhone"] != "+998900000001" {
		t.Fatalf("deletedPhone = %v, want the original number", raw["deletedPhone"])
	}
	if raw["deletedTelegramId"] != int64(555001) {
		t.Fatalf("deletedTelegramId = %v, want 555001", raw["deletedTelegramId"])
	}
}

// Two deleted accounts must be able to coexist — proves the unique-sparse index
// really is freed rather than holding a "" that the second delete collides with.
func TestTwoDeletedAccountsCoexist(t *testing.T) {
	db := testDB(t)
	h := newHandler(db)
	ctx := context.Background()

	if err := db.Collection("users").Drop(ctx); err != nil {
		t.Fatalf("drop users: %v", err)
	}
	// The production indexes are what make this test meaningful.
	if _, err := db.Collection("users").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "phone", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
		{Keys: bson.D{{Key: "telegramId", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
	}); err != nil {
		t.Fatalf("create indexes: %v", err)
	}

	for i, tgID := range []int64{555010, 555011} {
		uid := primitive.NewObjectID()
		if _, err := db.Collection("users").InsertOne(ctx, bson.M{
			"_id": uid, "phone": "+99890000010" + string(rune('0'+i)),
			"telegramId": tgID, "isDeleted": false, "createdAt": time.Now(),
		}); err != nil {
			t.Fatalf("insert user %d: %v", i, err)
		}
		h.RequestDelete(httptest.NewRecorder(), asUser(http.MethodPost, "/x", "", uid))
		w := httptest.NewRecorder()
		h.ConfirmDelete(w, asUser(http.MethodPost, "/x", `{"code":"`+storedCode(t, db, uid)+`"}`, uid))
		if w.Code != http.StatusOK {
			t.Fatalf("deleting account %d: status %d (%s)", i, w.Code, w.Body.String())
		}
	}

	n, err := db.Collection("users").CountDocuments(ctx, bson.M{"isDeleted": true})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("%d deleted accounts stored, want 2", n)
	}
}

// A wrong guess must be rejected, charged against the attempt budget, and must
// not touch the account.
func TestConfirmDeleteWrongCode(t *testing.T) {
	db := testDB(t)
	h := newHandler(db)
	ctx := context.Background()
	uid := seedUser(t, db, 0)

	h.RequestDelete(httptest.NewRecorder(), asUser(http.MethodPost, "/x", "", uid))

	w := httptest.NewRecorder()
	h.ConfirmDelete(w, asUser(http.MethodPost, "/x", `{"code":"wr0nG!"}`, uid))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}

	var u struct {
		IsDeleted bool `bson:"isDeleted"`
	}
	_ = db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&u)
	if u.IsDeleted {
		t.Fatal("a wrong code deleted the account")
	}

	var rec struct {
		Attempts int  `bson:"attempts"`
		Used     bool `bson:"used"`
	}
	if err := db.Collection("delete_codes").FindOne(ctx, bson.M{"userId": uid}).Decode(&rec); err != nil {
		t.Fatalf("load code: %v", err)
	}
	if rec.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", rec.Attempts)
	}
	if rec.Used {
		t.Fatal("code marked used after a wrong guess")
	}
}

// The code is case-sensitive — that's the whole point of the mixed alphabet.
func TestConfirmDeleteIsCaseSensitive(t *testing.T) {
	db := testDB(t)
	h := newHandler(db)
	uid := seedUser(t, db, 0)

	h.RequestDelete(httptest.NewRecorder(), asUser(http.MethodPost, "/x", "", uid))
	code := storedCode(t, db, uid)
	flipped := strings.ToUpper(code)
	if flipped == code {
		flipped = strings.ToLower(code)
	}

	w := httptest.NewRecorder()
	h.ConfirmDelete(w, asUser(http.MethodPost, "/x", `{"code":"`+flipped+`"}`, uid))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d for case-flipped code, want 401", w.Code)
	}
}

// One code, one deletion: replaying it must fail.
func TestConfirmDeleteCodeIsSingleUse(t *testing.T) {
	db := testDB(t)
	h := newHandler(db)
	uid := seedUser(t, db, 0)

	h.RequestDelete(httptest.NewRecorder(), asUser(http.MethodPost, "/x", "", uid))
	code := storedCode(t, db, uid)

	first := httptest.NewRecorder()
	h.ConfirmDelete(first, asUser(http.MethodPost, "/x", `{"code":"`+code+`"}`, uid))
	if first.Code != http.StatusOK {
		t.Fatalf("first confirm status = %d, want 200", first.Code)
	}

	second := httptest.NewRecorder()
	h.ConfirmDelete(second, asUser(http.MethodPost, "/x", `{"code":"`+code+`"}`, uid))
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("replayed code status = %d, want 401", second.Code)
	}
}

// Re-requesting must invalidate the previous code rather than leaving two live.
func TestRequestDeleteReplacesPreviousCode(t *testing.T) {
	db := testDB(t)
	h := newHandler(db)
	uid := seedUser(t, db, 0)

	h.RequestDelete(httptest.NewRecorder(), asUser(http.MethodPost, "/x", "", uid))
	first := storedCode(t, db, uid)
	h.RequestDelete(httptest.NewRecorder(), asUser(http.MethodPost, "/x", "", uid))
	second := storedCode(t, db, uid)

	if first == second {
		t.Fatal("re-request returned the same code")
	}
	w := httptest.NewRecorder()
	h.ConfirmDelete(w, asUser(http.MethodPost, "/x", `{"code":"`+first+`"}`, uid))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("superseded code status = %d, want 401", w.Code)
	}
}

func storedCode(t *testing.T, db *mongo.Database, uid primitive.ObjectID) string {
	t.Helper()
	var doc struct {
		Code string `bson:"code"`
	}
	if err := db.Collection("delete_codes").FindOne(context.Background(),
		bson.M{"userId": uid}).Decode(&doc); err != nil {
		t.Fatalf("load stored code: %v", err)
	}
	return doc.Code
}
