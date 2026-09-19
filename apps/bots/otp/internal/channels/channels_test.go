package channels

import (
	"context"
	"testing"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const channelID = int64(-1001234567890)

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://127.0.0.1:27017"))
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Ping(ctx, nil); err != nil {
		_ = c.Disconnect(context.Background())
		t.Skip("local Mongo unavailable")
	}
	db := c.Database("ib_channels_test_" + primitive.NewObjectID().Hex())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = db.Drop(ctx)
		_ = c.Disconnect(ctx)
	})
	return New(db.Collection("telegram_channels"))
}

func stored(t *testing.T, r *Registry) bson.M {
	t.Helper()
	var out bson.M
	if err := r.col.FindOne(context.Background(), bson.M{"_id": channelID}).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func update(chatType, status string, canPost bool) tg.Update {
	return tg.Update{MyChatMember: &tg.ChatMemberUpdated{
		Chat: tg.Chat{ID: channelID, Type: chatType, Title: "Ish e'lonlari", UserName: "ishlar"},
		From: tg.User{ID: 42},
		NewChatMember: tg.ChatMember{
			Status:          status,
			CanPostMessages: canPost,
		},
	}}
}

// Guruh va shaxsiy chat e'lon oqimi uchun mo'ljallanmagan: u yerda e'lonlar
// suhbatni bosib ketardi.
func TestOnlyChannelMembershipIsRegistered(t *testing.T) {
	for _, tc := range []struct {
		name string
		u    tg.Update
		want bool
	}{
		{"channel", update("channel", "administrator", true), true},
		{"group", update("group", "administrator", true), false},
		{"supergroup", update("supergroup", "administrator", true), false},
		{"private", update("private", "member", false), false},
		{"other update", tg.Update{Message: &tg.Message{}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := FromUpdate(tc.u); ok != tc.want {
				t.Fatalf("accepted=%t, want %t", ok, tc.want)
			}
		})
	}
}

func TestChannelBecomesActiveOnlyWithPostPermission(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       string
		canPost      bool
		wantActive   bool
		wantedReason string
	}{
		{"admin with permission", "administrator", true, true, ""},
		{"admin without permission", "administrator", false, false, "no_post_permission"},
		{"plain member", "member", false, false, "member_status:member"},
		{"removed", "kicked", false, false, "member_status:kicked"},
		{"left", "left", false, false, "member_status:left"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := testRegistry(t)
			event, ok := FromUpdate(update("channel", tc.status, tc.canPost))
			if !ok {
				t.Fatal("channel event rejected")
			}
			active, err := r.Apply(context.Background(), event)
			if err != nil {
				t.Fatal(err)
			}
			if active != tc.wantActive {
				t.Fatalf("active=%t, want %t", active, tc.wantActive)
			}
			doc := stored(t, r)
			wantStatus := "inactive"
			if tc.wantActive {
				wantStatus = "active"
			}
			if doc["status"] != wantStatus {
				t.Fatalf("stored status %v, want %s", doc["status"], wantStatus)
			}
			if tc.wantedReason != "" && doc["reason"] != tc.wantedReason {
				t.Fatalf("stored reason %v, want %s", doc["reason"], tc.wantedReason)
			}
			if doc["title"] != "Ish e'lonlari" || doc["username"] != "ishlar" {
				t.Fatalf("channel identity not stored: %v", doc)
			}
		})
	}
}

// Qo'shish → chiqarish → qayta qo'shish bitta yozuvda ishlashi kerak: kanal
// ID si o'zgarmaydi, ya'ni ikkinchi hujjat paydo bo'lmasin.
func TestRejoiningReusesTheSameRecord(t *testing.T) {
	r := testRegistry(t)
	ctx := context.Background()
	for _, step := range []struct {
		status  string
		canPost bool
	}{{"administrator", true}, {"kicked", false}, {"administrator", true}} {
		event, _ := FromUpdate(update("channel", step.status, step.canPost))
		if _, err := r.Apply(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	n, err := r.col.CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("got %d records for one channel", n)
	}
	if stored(t, r)["status"] != "active" {
		t.Fatal("rejoined channel did not become active again")
	}
}

// Operator to'xtatgan kanal bot qayta qo'shilsa ham tiklanmaydi — bu
// suiiste'mol qilgan kanalni uzishning yagona ishonchli usuli.
func TestBlockedChannelIsNeverReactivated(t *testing.T) {
	r := testRegistry(t)
	ctx := context.Background()
	if _, err := r.col.InsertOne(ctx, bson.M{"_id": channelID, "status": "blocked", "reason": "abuse"}); err != nil {
		t.Fatal(err)
	}
	event, _ := FromUpdate(update("channel", "administrator", true))
	active, err := r.Apply(ctx, event)
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("blocked channel was reported as receiving listings")
	}
	if doc := stored(t, r); doc["status"] != "blocked" || doc["reason"] != "abuse" {
		t.Fatalf("blocked record was modified: %v", doc)
	}
}
