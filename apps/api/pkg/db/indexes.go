package db

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EnsureIndexes creates required indexes on boot (idempotent).
func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	type spec struct {
		coll string
		idx  mongo.IndexModel
	}
	specs := []spec{
		{"users", mongo.IndexModel{Keys: bson.D{{Key: "telegramId", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)}},
		{"users", mongo.IndexModel{Keys: bson.D{{Key: "phone", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)}},
		{"users", mongo.IndexModel{Keys: bson.D{{Key: "firstName", Value: 1}, {Key: "lastName", Value: 1}}}},
		{"users", mongo.IndexModel{Keys: bson.D{{Key: "avatarDeletionJobs.nextAttemptAt", Value: 1}}, Options: options.Index().SetSparse(true)}},
		{"avatar_uploads", mongo.IndexModel{Keys: bson.D{{Key: "userId", Value: 1}}}},
		// Retention sweep (internal/account.Purger) scans for soft-deleted
		// accounts past their grace period every 6h. Without this it is a full
		// collection scan over every user on the platform.
		{"users", mongo.IndexModel{Keys: bson.D{{Key: "isDeleted", Value: 1}, {Key: "deletedAt", Value: 1}}}},
		// Admin panelining platforma kesimi: ro'yxat filtri (lastPlatform) va
		// "oxirgi 30 kunda faol" agregatsiyasi (lastSeenAt) — ikkalasi ham shu
		// bitta indeksdan foydalanadi. Usiz har ochilishda butun users
		// kolleksiyasi to'liq skanerlanardi.
		{"users", mongo.IndexModel{Keys: bson.D{{Key: "lastPlatform", Value: 1}, {Key: "lastSeenAt", Value: -1}}}},

		{"elons", mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}, {Key: "publishedAt", Value: -1}}}},
		{"elons", mongo.IndexModel{Keys: bson.D{{Key: "telegramChannel.reference", Value: 1}, {Key: "telegramChannel.status", Value: 1}, {Key: "telegramChannel.nextAttemptAt", Value: 1}}, Options: options.Index().SetSparse(true)}},
		{"elons", mongo.IndexModel{Keys: bson.D{{Key: "ownerId", Value: 1}, {Key: "status", Value: 1}}}},
		{"elons", mongo.IndexModel{Keys: bson.D{{Key: "ownerFollowupPending", Value: 1}}, Options: options.Index().SetSparse(true)}},
		{"elons", mongo.IndexModel{Keys: bson.D{{Key: "adminModerationJobs.nextAttemptAt", Value: 1}}, Options: options.Index().SetSparse(true)}},
		{"elons", mongo.IndexModel{Keys: bson.D{{Key: "adminModerationJobs.images", Value: 1}}, Options: options.Index().SetSparse(true)}},
		{"elons", mongo.IndexModel{Keys: bson.D{{Key: "images", Value: 1}}, Options: options.Index().SetSparse(true)}},
		{"elon_image_assets", mongo.IndexModel{Keys: bson.D{{Key: "ownerId", Value: 1}}}},
		{"admin_elon_purge_events", mongo.IndexModel{Keys: bson.D{{Key: "nextAttemptAt", Value: 1}}}},
		{"elons", mongo.IndexModel{Keys: bson.D{{Key: "categoryId", Value: 1}}}},
		{"elons", mongo.IndexModel{Keys: bson.D{{Key: "title", Value: "text"}, {Key: "description", Value: "text"}}}},

		// Ish signali (internal/jobalert): har yangi e'lon shu kolleksiyadan
		// qo'pol to'rtburchak bo'yicha nomzodlarni tanlaydi. Usiz har e'lon
		// butun kolleksiyani skanerlardi.
		{"job_alerts", mongo.IndexModel{Keys: bson.D{{Key: "lat", Value: 1}, {Key: "lng", Value: 1}}}},

		{"applications", mongo.IndexModel{Keys: bson.D{{Key: "workerId", Value: 1}, {Key: "status", Value: 1}}}},
		{"applications", mongo.IndexModel{Keys: bson.D{{Key: "elonId", Value: 1}, {Key: "status", Value: 1}}}},
		{"applications", mongo.IndexModel{Keys: bson.D{{Key: "listingRecheckPending", Value: 1}}, Options: options.Index().SetSparse(true)}},
		{"applications", mongo.IndexModel{Keys: bson.D{{Key: "employerId", Value: 1}, {Key: "status", Value: 1}}}},
		{"applications", mongo.IndexModel{Keys: bson.D{{Key: "elonId", Value: 1}, {Key: "workerId", Value: 1}}, Options: options.Index().SetUnique(true)}},
		// MyApplications/MyElonsApplications/History appliedAt bo'yicha sortlaydi;
		// History'ning $or filtri uchun Mongo har bir tarmoqqa alohida indeks
		// ishlatadi — appliedAt shu tarmoqlarda bo'lmasa sort xotirada bajariladi.
		{"applications", mongo.IndexModel{Keys: bson.D{{Key: "workerId", Value: 1}, {Key: "appliedAt", Value: -1}}}},
		{"applications", mongo.IndexModel{Keys: bson.D{{Key: "employerId", Value: 1}, {Key: "appliedAt", Value: -1}}}},
		// E'lonning batafsil sahifasi (internal/admin/elon_detail.go) arizalarni
		// elonId bo'yicha oladi va appliedAt bo'yicha sortlaydi. Yuqoridagi
		// {elonId, status} indeksi sortga yaramaydi — usiz sort xotirada
		// bajarilardi, xuddi workerId/employerId juftliklari kabi.
		{"applications", mongo.IndexModel{Keys: bson.D{{Key: "elonId", Value: 1}, {Key: "appliedAt", Value: -1}}}},

		// "Bir kunga bitta ish" qulflari (internal/application/daylock.go).
		// Unikallikni _id ({ishchi}:{kun}) o'zi beradi; bu faqat o'tgan
		// kunlarning qulflarini yig'ishtiradigan TTL.
		{"worker_day_locks", mongo.IndexModel{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}},

		{"categories", mongo.IndexModel{Keys: bson.D{{Key: "slug", Value: 1}}, Options: options.Index().SetUnique(true)}},

		{"notifications", mongo.IndexModel{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "createdAt", Value: -1}}}},
		// Telegram outbox shares the inbox document, so persistence is atomic.
		{"notifications", mongo.IndexModel{Keys: bson.D{{Key: "telegram.nextAttemptAt", Value: 1}}, Options: options.Index().SetSparse(true)}},

		// FCM qurilma tokenlari: bitta token — bitta hujjat (upsert kaliti);
		// push yuborishda foydalanuvchining hamma qurilmasi userId bo'yicha olinadi.
		{"device_tokens", mongo.IndexModel{Keys: bson.D{{Key: "token", Value: 1}}, Options: options.Index().SetUnique(true)}},
		{"device_tokens", mongo.IndexModel{Keys: bson.D{{Key: "userId", Value: 1}}}},
		{"reports", mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}, {Key: "createdAt", Value: -1}}}},
		// Batafsil sahifalar shikoyatlarni NISHON bo'yicha oladi
		// (internal/admin: GetUser, GetElon). Bu indekssiz har ochilish
		// butun kolleksiyani skanerlardi — ya'ni sahifani qayta-qayta
		// yangilash bazani cho'ktiradigan arzon yo'lga aylanardi.
		{"reports", mongo.IndexModel{Keys: bson.D{{Key: "targetType", Value: 1}, {Key: "targetId", Value: 1}, {Key: "createdAt", Value: -1}}}},

		{"admins", mongo.IndexModel{Keys: bson.D{{Key: "username", Value: 1}}, Options: options.Index().SetUnique(true)}},

		// Admin jurnali. `target` HEX SATR sifatida saqlanadi
		// (internal/admin.Handler.audit), va batafsil sahifalar aynan shu
		// bo'yicha qidiradi: statusFromAudit (foydalanuvchi) va
		// elonAdminActions (e'lon). Jurnal faqat o'sadi — indekssiz bu
		// so'rovlar vaqt o'tgani sayin sekinlashib boradi.
		{"admin_audit", mongo.IndexModel{Keys: bson.D{{Key: "target", Value: 1}, {Key: "action", Value: 1}, {Key: "createdAt", Value: -1}}}},
		// Audit log ro'yxati (internal/admin.Handler.Audit) BOSHQA so'rov:
		// filtr `action`/`createdAt` bo'yicha, sort esa doim `createdAt: -1`.
		// Yuqoridagi indeks `target` dan boshlanadi, ya'ni bu so'rovga
		// xizmat qila olmaydi — har ochilish butun kolleksiyani skanerlab,
		// natijani XOTIRADA sortlaydi. Jurnal faqat o'sgani uchun bu ertami-
		// kechmi Mongo'ning 32 MB sort chegarasiga urilib, 96-xato bilan
		// YIQILADI: "kim nima qildi" degan savolga javob beradigan yagona
		// ekran jimgina ishdan chiqadi. Shuning uchun ikki indeks:
		{"admin_audit", mongo.IndexModel{Keys: bson.D{{Key: "createdAt", Value: -1}}}},
		{"admin_audit", mongo.IndexModel{Keys: bson.D{{Key: "action", Value: 1}, {Key: "createdAt", Value: -1}}}},
		// ...va admin bo'yicha kesim (?adminId=) — bitta xodimning butun izi.
		{"admin_audit", mongo.IndexModel{Keys: bson.D{{Key: "adminId", Value: 1}, {Key: "createdAt", Value: -1}}}},

		// Admin sessiyalari (refresh tokenlar, internal/admin/refresh.go).
		// Qidiruv har doim xesh bo'yicha ketadi — ikkalasi ham indekslangan,
		// chunki aylantirishdan keyingi imtiyoz oynasida eski xesh so'raladi.
		{"admin_sessions", mongo.IndexModel{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: options.Index().SetUnique(true)}},
		{"admin_sessions", mongo.IndexModel{Keys: bson.D{{Key: "prevTokenHash", Value: 1}}, Options: options.Index().SetSparse(true)}},
		{"admin_sessions", mongo.IndexModel{Keys: bson.D{{Key: "adminId", Value: 1}}}},
		// TTL: qat'iy muddati o'tgan sessiyalarni Mongo o'zi yig'ishtiradi.
		{"admin_sessions", mongo.IndexModel{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}},

		// OTP collection: TTL on expiresAt
		{"otp_codes", mongo.IndexModel{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}},
		{"otp_codes", mongo.IndexModel{Keys: bson.D{{Key: "tgToken", Value: 1}}}},

		// Account-deletion codes: one live code per user, TTL-reaped on expiry.
		{"delete_codes", mongo.IndexModel{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}},
		{"delete_codes", mongo.IndexModel{Keys: bson.D{{Key: "userId", Value: 1}}, Options: options.Index().SetUnique(true)}},

		// Ilova ichidagi fikr-mulohaza: foydalanuvchi o'z tarixini, admin butun
		// ro'yxatni createdAt bo'yicha teskari sortda oladi.
		{"feedback", mongo.IndexModel{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "createdAt", Value: -1}}}},
		{"feedback", mongo.IndexModel{Keys: bson.D{{Key: "createdAt", Value: -1}}}},

		// Dastur xatoliklari (internal/errlog, "3.12 · Xatoliklar").
		//
		// fingerprint UNIKAL: guruhlash butunlay shunga tayanadi va yozuv
		// upsert bilan ketadi. Indekssiz (yoki unikalsiz) ikki parallel
		// upsert bir xil xatolik uchun ikkita guruh yasab, sanoqni ikkiga
		// bo'lib yuborardi.
		{"error_groups", mongo.IndexModel{Keys: bson.D{{Key: "fingerprint", Value: 1}}, Options: options.Index().SetUnique(true)}},
		// Ro'yxatning sukut bo'yicha ko'rinishi: Holat = "Ochiq", sort =
		// oxirgi hodisa. Filtrlarning qolgan uchtasi ham shu shaklda.
		{"error_groups", mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}, {Key: "lastSeenAt", Value: -1}}}},
		{"error_groups", mongo.IndexModel{Keys: bson.D{{Key: "severity", Value: 1}, {Key: "lastSeenAt", Value: -1}}}},
		{"error_groups", mongo.IndexModel{Keys: bson.D{{Key: "module", Value: 1}, {Key: "lastSeenAt", Value: -1}}}},
		{"error_groups", mongo.IndexModel{Keys: bson.D{{Key: "lastSeenAt", Value: -1}}}},
		// Qidiruv kod/yorliq bo'yicha aniq mos kelishni ham qo'llaydi.
		{"error_groups", mongo.IndexModel{Keys: bson.D{{Key: "code", Value: 1}}}},
		{"error_groups", mongo.IndexModel{Keys: bson.D{{Key: "ref", Value: 1}}}},
		// TTL — 180 kun (Figma 3.12.2 · G · "Saqlash muddati"). Jurnal
		// cheksiz o'sadigan yagona kolleksiya bo'lib qolmasligi kerak:
		// xatolik yozuvi diagnostika uchun, arxiv uchun emas.
		{"error_groups", mongo.IndexModel{Keys: bson.D{{Key: "lastSeenAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(180 * 24 * 3600).SetName("lastSeenAt_ttl")}},

		// Hodisalar — 30 kun. Guruhdan qisqaroq, chunki ular ancha ko'p va
		// faqat "24 soatdagi hodisalar" hisobiga kerak.
		{"error_events", mongo.IndexModel{Keys: bson.D{{Key: "at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(30 * 24 * 3600).SetName("at_ttl")}},
		{"error_events", mongo.IndexModel{Keys: bson.D{{Key: "fingerprint", Value: 1}, {Key: "at", Value: -1}}}},
		{"error_events", mongo.IndexModel{Keys: bson.D{{Key: "at", Value: -1}}}},

		// Namunalar — stek, qadamlar va qurilma bilan ("3.12.1 · Xatolik —
		// batafsil"). Hodisalar bilan bir xil 30 kunlik TTL. HAJM bo'yicha
		// chegara alohida qo'yiladi (internal/errlog.pruneSamples: har
		// guruhda ≤ 20 ta) — TTL faqat vaqtni cheklaydi, tinimsiz
		// takrorlanadigan bitta xatolik esa 30 kun ichida ham o'n minglab
		// stek izini qoldira olardi.
		{"error_samples", mongo.IndexModel{Keys: bson.D{{Key: "at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(30 * 24 * 3600).SetName("at_ttl")}},
		{"error_samples", mongo.IndexModel{Keys: bson.D{{Key: "fingerprint", Value: 1}, {Key: "at", Value: -1}}}},

		// Kanalga chiqarish (internal/channelpost). Bot qaysi kanallarda
		// administrator ekani reyestrda; fan-out har e'londa FAQAT faol
		// yozuvlarni o'qiydi.
		{"telegram_channels", mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}}}},
		// (e'lon, kanal) juftligi UNIKAL: uzilgan fan-out qayta ishlaganda
		// ham bitta kanalga ikkinchi post yaratilmaydi. Takroriy postning
		// oldini olish aynan shu indeksga tayanadi.
		{"channel_posts", mongo.IndexModel{Keys: bson.D{{Key: "elonId", Value: 1}, {Key: "chatId", Value: 1}}, Options: options.Index().SetUnique(true)}},
		// Navbat tanlovi: status + navbatdagi vaqt bo'yicha eng eskisi.
		{"channel_posts", mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}, {Key: "nextAttemptAt", Value: 1}}}},
		// Uzilib qolgan lizinglarni tozalash (status=sending, leaseUntil<now).
		{"channel_posts", mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}, {Key: "leaseUntil", Value: 1}}}},
		// Fan-out kutayotgan e'lonlar. Sparse: belgisi bor e'lonlar oz.
		{"elons", mongo.IndexModel{Keys: bson.D{{Key: "telegramBroadcast.status", Value: 1}, {Key: "telegramBroadcast.queuedAt", Value: 1}}, Options: options.Index().SetSparse(true)}},
	}
	for _, s := range specs {
		if _, err := db.Collection(s.coll).Indexes().CreateOne(ctx, s.idx); err != nil {
			return err
		}
	}
	return nil
}
