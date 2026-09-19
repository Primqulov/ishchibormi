package admin

import (
	"context"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestUsersFilterByPlatform(t *testing.T) {
	db := filterDB(t)
	ctx := context.Background()
	users := db.Collection("users")

	if _, err := users.InsertMany(ctx, []any{
		bson.M{"_id": primitive.NewObjectID(), "firstName": "Veb", "lastPlatform": "web"},
		bson.M{"_id": primitive.NewObjectID(), "firstName": "Android", "lastPlatform": "android"},
		bson.M{"_id": primitive.NewObjectID(), "firstName": "Ios", "lastPlatform": "ios"},
		// Maydon umuman yo'q — eski hisob.
		bson.M{"_id": primitive.NewObjectID(), "firstName": "Eski"},
		// Bo'sh satr — "noma'lum" ning ikkinchi ko'rinishi.
		bson.M{"_id": primitive.NewObjectID(), "firstName": "Bosh", "lastPlatform": ""},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	names := func(q url.Values) []string {
		cur, err := users.Find(ctx, usersFilter(q))
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		defer cur.Close(ctx)
		out := []string{}
		for cur.Next(ctx) {
			var u struct {
				FirstName string `bson:"firstName"`
			}
			if cur.Decode(&u) == nil {
				out = append(out, u.FirstName)
			}
		}
		sort.Strings(out)
		return out
	}

	cases := []struct {
		platform string
		want     []string
	}{
		{"web", []string{"Veb"}},
		{"android", []string{"Android"}},
		{"ios", []string{"Ios"}},
		// Ikkala "noma'lum" ko'rinishi ham bitta filtrga tushadi.
		{"unknown", []string{"Bosh", "Eski"}},
	}
	for _, c := range cases {
		t.Run(c.platform, func(t *testing.T) {
			if got := names(url.Values{"platform": {c.platform}}); !equal(got, c.want) {
				t.Errorf("platform=%s -> %v, want %v", c.platform, got, c.want)
			}
		})
	}

	// Notanish qiymat filtrni O'CHIRIB qo'yishi kerak, hech kimni
	// qaytarmaslikni emas: aks holda admin bo'sh ro'yxatni ko'rib
	// "foydalanuvchi yo'q" deb o'ylardi.
	if got := names(url.Values{"platform": {"symbian"}}); len(got) != 5 {
		t.Errorf("notanish platforma -> %v (%d ta), hammasi kutilgan", got, len(got))
	}
}

func TestPlatformCountsAlwaysListsEveryPlatform(t *testing.T) {
	db := filterDB(t)
	ctx := context.Background()
	users := db.Collection("users")

	now := time.Now()
	if _, err := users.InsertMany(ctx, []any{
		bson.M{"_id": primitive.NewObjectID(), "signupPlatform": "web", "lastPlatform": "web", "lastSeenAt": now},
		bson.M{"_id": primitive.NewObjectID(), "signupPlatform": "web", "lastPlatform": "android", "lastSeenAt": now},
		bson.M{"_id": primitive.NewObjectID(), "signupPlatform": "android", "lastPlatform": "android", "lastSeenAt": now},
		// signupPlatform yo'q — bu funksiyadan oldin ro'yxatdan o'tgan.
		bson.M{"_id": primitive.NewObjectID(), "lastPlatform": "web", "lastSeenAt": now},
		// O'chirilgan hisob hech qaysi hisobotga kirmasligi kerak.
		bson.M{"_id": primitive.NewObjectID(), "signupPlatform": "web", "isDeleted": true},
		// Play review qumdoni ham.
		bson.M{"_id": primitive.NewObjectID(), "signupPlatform": "ios", "isReviewAccount": true},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	as := func(rows []nameCount) map[string]int {
		m := map[string]int{}
		for _, r := range rows {
			m[r.Name] = r.Count
		}
		return m
	}

	signup := platformCounts(ctx, users, "signupPlatform", nil)
	// Hech kim ishlatmagan platforma ham ustun sifatida BARIBIR bo'lishi
	// kerak — yo'qolgan ustun "ma'lumot kelmadi" bo'lib o'qiladi. Kutilgan
	// son ATAYLAB qotirilmagan: yangi platforma qo'shilganda test emas,
	// httpx.Platforms yagona manba bo'lib qolsin.
	wantRows := len(httpx.Platforms) + 1 // + "noma'lum"
	if len(signup) != wantRows {
		t.Fatalf("signup %d ta qator, %d kutilgan: %v", len(signup), wantRows, signup)
	}
	got := as(signup)
	for name, want := range map[string]int{"web": 2, "android": 1, "ios": 0, "telegram": 0, "unknown": 1} {
		if got[name] != want {
			t.Errorf("signup[%s] = %d, want %d (hammasi: %v)", name, got[name], want, got)
		}
	}

	// Yig'indi jami faol foydalanuvchiga teng bo'lishi shart: aks holda
	// panelda ustunlar yig'indisi KPI kartasiga mos kelmay qolardi.
	sum := 0
	for _, r := range signup {
		sum += r.Count
	}
	if sum != 4 {
		t.Errorf("signup yig'indisi %d, 4 kutilgan (o'chirilgan/review hisoblanmasin)", sum)
	}

	active := as(platformCounts(ctx, users, "lastPlatform", bson.M{
		"lastSeenAt": bson.M{"$gte": now.Add(-24 * time.Hour)},
	}))
	for name, want := range map[string]int{"web": 2, "android": 2, "ios": 0, "unknown": 0} {
		if active[name] != want {
			t.Errorf("active[%s] = %d, want %d (hammasi: %v)", name, active[name], want, active)
		}
	}
}

// Oyna tashqarisidagi foydalanuvchi "faol" emas.
func TestPlatformCountsRespectsActiveWindow(t *testing.T) {
	db := filterDB(t)
	ctx := context.Background()
	users := db.Collection("users")

	now := time.Now()
	if _, err := users.InsertMany(ctx, []any{
		bson.M{"_id": primitive.NewObjectID(), "lastPlatform": "web", "lastSeenAt": now},
		bson.M{"_id": primitive.NewObjectID(), "lastPlatform": "web", "lastSeenAt": now.AddDate(0, 0, -90)},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	since := now.AddDate(0, 0, -activeWindowDays)
	rows := platformCounts(ctx, users, "lastPlatform", bson.M{"lastSeenAt": bson.M{"$gte": since}})
	for _, r := range rows {
		if r.Name == "web" && r.Count != 1 {
			t.Errorf("web = %d, 1 kutilgan (90 kun oldingisi sanalmasin)", r.Count)
		}
	}
}
