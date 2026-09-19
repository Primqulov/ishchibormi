package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User -- platform user (both employer and worker)
type User struct {
	ID                  primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	TelegramID          int64               `bson:"telegramId,omitempty" json:"telegramId"`
	Phone               string              `bson:"phone" json:"phone"`
	FirstName           string              `bson:"firstName" json:"firstName"`
	LastName            string              `bson:"lastName" json:"lastName"`
	AvatarURL           string              `bson:"avatarUrl,omitempty" json:"avatarUrl"`
	AvatarMetadata      *AvatarMetadata     `bson:"avatarMetadata,omitempty" json:"-"`
	AvatarRevision      int64               `bson:"avatarRevision,omitempty" json:"-"`
	AvatarDeletedAt     *time.Time          `bson:"avatarDeletedAt,omitempty" json:"avatarDeletedAt,omitempty"`
	AvatarDeletedBy     primitive.ObjectID  `bson:"avatarDeletedBy,omitempty" json:"-"`
	AvatarDeletedReason string              `bson:"avatarDeletedReason,omitempty" json:"-"`
	AvatarDeletionJobs  []AvatarDeletionJob `bson:"avatarDeletionJobs,omitempty" json:"-"`
	Region              string              `bson:"region,omitempty" json:"region"`
	District            string              `bson:"district,omitempty" json:"district"`
	Bio                 string              `bson:"bio,omitempty" json:"bio"`
	Skills              []string            `bson:"skills,omitempty" json:"skills"`
	Rating              float64             `bson:"rating" json:"rating"`
	ReviewsCount        int                 `bson:"reviewsCount" json:"reviewsCount"`
	// Ikki tomonlama reyting: ishchi sifatida va ish beruvchi sifatida alohida.
	WorkerRating         float64 `bson:"workerRating" json:"workerRating"`
	WorkerReviewsCount   int     `bson:"workerReviewsCount" json:"workerReviewsCount"`
	EmployerRating       float64 `bson:"employerRating" json:"employerRating"`
	EmployerReviewsCount int     `bson:"employerReviewsCount" json:"employerReviewsCount"`
	CompletedJobsCount   int     `bson:"completedJobsCount" json:"completedJobsCount"`
	IsPhoneVerified      bool    `bson:"isPhoneVerified" json:"isPhoneVerified"`
	IsBlocked            bool    `bson:"isBlocked" json:"isBlocked"`
	// ModerationBannedUntil — avtomatik moderatsiya bloki tugash vaqti.
	//
	// IsBlocked dan ATAYLAB alohida: u admin qo'lidagi bayroq. Ikkalasini
	// bitta maydonda birlashtirsak, 2 yildan keyin blokni kim (admin yoki
	// tizim) qo'yganini bilib bo'lmasdi. Bu maydon esa vaqt o'tishi bilan
	// o'z-o'zidan kuchini yo'qotadi.
	//
	// Haqiqiy manba `moderation_strikes` (telefon bo'yicha) — bu nusxa faqat
	// mavjud seansni darhol to'xtatish uchun.
	// JSON'da ochiq (omitempty): admin paneli bloklangan foydalanuvchini
	// va blok tugash sanasini ko'rsatishi kerak. Bloklanmaganlar uchun
	// maydon umuman chiqmaydi.
	ModerationBannedUntil *time.Time `bson:"moderationBannedUntil,omitempty" json:"moderationBannedUntil,omitempty"`

	// ── Blok sababi ───────────────────────────────────────────────────────
	//
	// Admin paneli uchun: "ertaga bu foydalanuvchi nega bloklangan?" degan
	// savolga javob. Ilgari javob yo'q edi — `isBlocked` faqat ha/yo'q
	// bayrog'i, `moderationBannedUntil` esa faqat sana. Blokni kim, qachon
	// va NEGA qo'ygani hech qayerda saqlanmasdi.
	//
	// Uchala maydon ham `isBlocked` yoki `moderationBannedUntil` bilan
	// BIRGA yoziladi va blok ochilganda birga tozalanadi.
	//
	// Nega foydalanuvchining o'ziga oqib ketmaydi: bu maydonlar faqat
	// bloklangan hujjatda to'ladi, bloklangan foydalanuvchi esa /me ni
	// umuman chaqira olmaydi — auth.RequireActiveUser uni 403 bilan
	// to'xtatadi.

	// BlockReason — inson o'qiy oladigan sabab. Admin bloklaganda uning
	// o'zi yozgan matn; avtomatik blokda tayyor jumla.
	BlockReason string `bson:"blockReason,omitempty" json:"blockReason,omitempty"`
	// BlockSource — blokni kim qo'ygan: "admin" yoki "moderation".
	//
	// Bitta bayroq o'rniga alohida maydon, chunki ikkalasining oqibati
	// boshqacha: admin bloki qo'lda ochilguncha turadi, moderatsiya bloki
	// esa muddati tugagach o'z-o'zidan kuchini yo'qotadi.
	BlockSource string `bson:"blockSource,omitempty" json:"blockSource,omitempty"`
	// BlockedAt — qachon bloklangan.
	BlockedAt *time.Time `bson:"blockedAt,omitempty" json:"blockedAt,omitempty"`
	// BlockedBy — bloklagan admin id'si (faqat qo'lda blokda).
	BlockedBy string `bson:"blockedBy,omitempty" json:"blockedBy,omitempty"`

	// ── Platforma (qaysi klientdan) ───────────────────────────────────────
	//
	// Ikkita alohida maydon, chunki ikkita boshqa savolga javob beradi:
	// "qayerdan kelgan" (marketing/o'sish) va "hozir nimadan foydalanadi"
	// (qaysi klientni rivojlantirish kerak). Bitta maydonda birlashtirsak,
	// vebdan ro'yxatdan o'tib keyin ilovaga ko'chgan foydalanuvchi ikkala
	// javobni ham buzardi.
	//
	// Qiymatlar — httpx.Platform* yopiq ro'yxati ("web"|"android"|"ios").
	// Maydon BO'SH bo'lishi mumkin va bu normal: bu funksiya qo'shilishidan
	// oldin ro'yxatdan o'tganlar va sarlavha yubormaydigan eski klientlar.
	// omitempty ataylab — "noma'lum" ni bo'sh satr sifatida saqlaymiz, aks
	// holda uni keyinchalik haqiqiy qiymatdan ajratib bo'lmasdi.

	// SignupPlatform — ro'yxatdan o'tgan payt qaysi klient ishlatilgan.
	// FAQAT hujjat yaratilganda yoziladi ($setOnInsert) va hech qachon
	// o'zgarmaydi.
	SignupPlatform string `bson:"signupPlatform,omitempty" json:"signupPlatform,omitempty"`
	// LastPlatform — oxirgi so'rov qaysi klientdan kelgan.
	LastPlatform string `bson:"lastPlatform,omitempty" json:"lastPlatform,omitempty"`

	// ── Qurilma (veb klient qaysi OS'da ochilgan) ─────────────────────────
	//
	// Platformadan ALOHIDA o'lchov: "web" bizga brauzer ekanini aytadi,
	// qaysi qurilmada ochilganini — aytmaydi. Panelda ikkalasi qo'shilib
	// "Veb Android", "Veb iOS", "Veb Windows", "Veb Linux" bo'lib ko'rinadi.
	//
	// Qiymatlar — httpx.Device* yopiq ro'yxati ("android" | "ios" |
	// "windows" | "macos" | "linux" | "chromeos", hamda eski yozuvlardagi
	// "desktop"). FAQAT platforma "web" bo'lganda yoziladi: mobil ilovada
	// platformaning o'zi allaqachon qurilma OS'i, takrorlash chalkashtirardi.
	//
	// Bo'sh bo'lishi normal va ko'p uchraydi: bu maydondan oldin ro'yxatdan
	// o'tganlar, mobil ilova hisoblari, va UA'si brauzernikiga o'xshamagan
	// so'rovlar.
	//
	// XAVFSIZLIK: qiymat User-Agent'dan olinadi, ya'ni klient uni
	// o'zgartira oladi. Shuning uchun u FAQAT ko'rsatish va statistika
	// uchun — hech qanday ruxsat yoki cheklov tekshiruviga kirmaydi. Xom UA
	// satri saqlanmaydi (barmoq izi darajasidagi ma'lumot), faqat yuqoridagi
	// yopiq ro'yxatdan bitta qiymat.

	// SignupDevice — ro'yxatdan o'tgan paytdagi qurilma. $setOnInsert,
	// hech qachon o'zgarmaydi (SignupPlatform bilan bir xil qoida).
	SignupDevice string `bson:"signupDevice,omitempty" json:"signupDevice,omitempty"`
	// LastDevice — oxirgi so'rov paytidagi qurilma.
	LastDevice string `bson:"lastDevice,omitempty" json:"lastDevice,omitempty"`

	// LastSeenAt — LastPlatform qachon yozilgan. Ikki vazifasi bor: "faol
	// foydalanuvchi" hisobini oxirgi 30 kun bo'yicha kesish, va yozuvni
	// qayta-qayta yangilamaslik uchun eskirganini bilish
	// (auth.RequireActiveUser dagi throttle).
	LastSeenAt *time.Time `bson:"lastSeenAt,omitempty" json:"lastSeenAt,omitempty"`

	IsDeleted           bool                 `bson:"isDeleted" json:"isDeleted"`
	LangPref            string               `bson:"langPref" json:"langPref"`
	ThemePref           string               `bson:"themePref" json:"themePref"`
	BlockedUserIDs      []primitive.ObjectID `bson:"blockedUserIds,omitempty" json:"blockedUserIds"`
	OnboardingCompleted bool                 `bson:"onboardingCompleted" json:"onboardingCompleted"`
	CreatedAt           time.Time            `bson:"createdAt" json:"createdAt"`
	UpdatedAt           time.Time            `bson:"updatedAt" json:"updatedAt"`

	// Self-deletion releases the account's identity: phone and telegramId are
	// unset (freeing the unique indexes so the number can register again as a
	// fresh account) and archived here for support/audit. Only ever populated
	// on soft-deleted records — see internal/account.softDelete.
	DeletedPhone      string     `bson:"deletedPhone,omitempty" json:"deletedPhone,omitempty"`
	DeletedTelegramID int64      `bson:"deletedTelegramId,omitempty" json:"deletedTelegramId,omitempty"`
	DeletedAt         *time.Time `bson:"deletedAt,omitempty" json:"deletedAt,omitempty"`

	// IsReviewAccount marks the sandboxed Google Play review account (and the
	// demo counterparties seeded alongside it). It gates the write restrictions
	// in auth.DenyReviewAccount and keeps the account out of public listings,
	// admin analytics and the retention sweep.
	//
	// json:"-" is deliberate: serialising it would advertise to every client
	// that a review mode exists. Nothing outside the server ever sees it.
	IsReviewAccount bool `bson:"isReviewAccount,omitempty" json:"-"`
}

// PublicUser is a safe projection.
type PublicUser struct {
	ID                   primitive.ObjectID `json:"id"`
	FirstName            string             `json:"firstName"`
	LastName             string             `json:"lastName"`
	AvatarURL            string             `json:"avatarUrl"`
	Region               string             `json:"region"`
	District             string             `json:"district"`
	Bio                  string             `json:"bio"`
	Skills               []string           `json:"skills"`
	Rating               float64            `json:"rating"`
	ReviewsCount         int                `json:"reviewsCount"`
	WorkerRating         float64            `json:"workerRating"`
	WorkerReviewsCount   int                `json:"workerReviewsCount"`
	EmployerRating       float64            `json:"employerRating"`
	EmployerReviewsCount int                `json:"employerReviewsCount"`
	CompletedJobsCount   int                `json:"completedJobsCount"`
	IsPhoneVerified      bool               `json:"isPhoneVerified"`
}

func (u *User) Public() PublicUser {
	return PublicUser{
		ID: u.ID, FirstName: u.FirstName, LastName: u.LastName, AvatarURL: u.AvatarURL,
		Region: u.Region, District: u.District, Bio: u.Bio, Skills: u.Skills,
		Rating: u.Rating, ReviewsCount: u.ReviewsCount,
		WorkerRating: u.WorkerRating, WorkerReviewsCount: u.WorkerReviewsCount,
		EmployerRating: u.EmployerRating, EmployerReviewsCount: u.EmployerReviewsCount,
		CompletedJobsCount: u.CompletedJobsCount, IsPhoneVerified: u.IsPhoneVerified,
	}
}

// Category
type Category struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name            string             `bson:"name" json:"name"`
	Slug            string             `bson:"slug" json:"slug"`
	Icon            string             `bson:"icon,omitempty" json:"icon"`
	CreatedBy       primitive.ObjectID `bson:"createdBy,omitempty" json:"createdBy"`
	IsSystemDefault bool               `bson:"isSystemDefault" json:"isSystemDefault"`
	IsActive        bool               `bson:"isActive" json:"isActive"`
	// UsageCount — kategoriyada TARIXAN joylangan e'lonlar soni (faqat o'sadi).
	// Admin panel uchun; ommaviy `/api/categories` javobida u ActiveCount bilan
	// bir xil qiymatga almashtiriladi — category.Handler.List'ga qarang.
	UsageCount int `bson:"usageCount" json:"usageCount"`
	// ActiveCount — hozir feedda ko'rinib turgan (recruiting, o'chirilmagan,
	// vaqti o'tmagan) e'lonlar soni. Bazada saqlanmaydi: e'lon vaqt o'tishi
	// bilan o'z-o'zidan "faol emas"ga aylanadi, shuning uchun har so'rovda
	// `elons` ustidan hisoblanadi.
	ActiveCount int       `bson:"-" json:"activeCount"`
	CreatedAt   time.Time `bson:"createdAt" json:"createdAt"`
}

// Elon (job listing)
type Elon struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	OwnerID      primitive.ObjectID `bson:"ownerId" json:"ownerId"`
	Title        string             `bson:"title" json:"title"`
	CategoryID   primitive.ObjectID `bson:"categoryId" json:"categoryId"`
	CategoryName string             `bson:"categoryName" json:"categoryName"`
	Description  string             `bson:"description" json:"description"`
	LocationURL  string             `bson:"locationUrl,omitempty" json:"locationUrl"`
	LocationText string             `bson:"locationText,omitempty" json:"locationText"`
	// Aniq ish joyi koordinatalari (xaritadan tanlanadi).
	Lat             float64 `bson:"lat,omitempty" json:"lat"`
	Lng             float64 `bson:"lng,omitempty" json:"lng"`
	Region          string  `bson:"region,omitempty" json:"region"`
	District        string  `bson:"district,omitempty" json:"district"`
	WorkersNeeded   int     `bson:"workersNeeded" json:"workersNeeded"`
	PricingType     string  `bson:"pricingType" json:"pricingType"` // per_worker|total|negotiable
	PriceAmount     int64   `bson:"priceAmount" json:"priceAmount"`
	PerWorkerAmount int64   `bson:"perWorkerAmount" json:"perWorkerAmount"`
	StartDate       string  `bson:"startDate,omitempty" json:"startDate"`
	WorkTimeFrom    string  `bson:"workTimeFrom,omitempty" json:"workTimeFrom"`
	WorkTimeTo      string  `bson:"workTimeTo,omitempty" json:"workTimeTo"`
	ContactPhone    string  `bson:"contactPhone,omitempty" json:"contactPhone"`
	// Ishga kimlar kerak: male (erkaklar) | female (ayollar) | mixed (aralash).
	// Bo'sh/eski e'lonlar "aralash" deb hisoblanadi (feed filtriga qarang).
	Gender        string `bson:"gender,omitempty" json:"gender"`
	Status        string `bson:"status" json:"status"` // draft|recruiting|filled|in_progress|completed|cancelled|hidden
	AcceptedCount int    `bson:"acceptedCount" json:"acceptedCount"`
	ViewsCount    int    `bson:"viewsCount" json:"viewsCount"`
	// Owner edits use a revision fence so concurrent saves cannot overwrite a
	// newer edit or remove images which another request has just retained.
	OwnerRevision int64      `bson:"ownerRevision,omitempty" json:"-"`
	CancelReason  string     `bson:"cancelReason,omitempty" json:"cancelReason,omitempty"`
	CancelledAt   *time.Time `bson:"cancelledAt,omitempty" json:"cancelledAt,omitempty"`
	// Durable owner follow-up survives an HTTP disconnect or process restart.
	OwnerFollowupPending bool  `bson:"ownerFollowupPending,omitempty" json:"-"`
	OwnerFollowupVersion int64 `bson:"ownerFollowupVersion,omitempty" json:"-"`
	// The last schedule/address edit remains visible to legacy applications
	// which predate detailed snapshots, even after a later title-only save.
	OwnerWorkDetailsRevision int64 `bson:"ownerWorkDetailsRevision,omitempty" json:"-"`
	// HiddenFromStatus — admin e'lonni yashirishdan OLDIN qaysi holatda
	// bo'lgani. Faqat `status == "hidden"` paytda yoziladi.
	//
	// Yashirish `status` ni "hidden" bilan almashtiradi, ya'ni oldingi holat
	// yo'qoladi. Busiz «Tiklash» faqat "recruiting" ga qaytara olardi va bu
	// ma'lumotni buzardi: allaqachon boshlanib ketgan (in_progress) yoki
	// yakunlangan ish qaytadan ommaviy feedga chiqib, yangi ariza qabul
	// qila boshlardi.
	//
	// json:"-" — bu ichki moderatsiya tafsiloti; na foydalanuvchiga, na
	// panelga uzatilmaydi (panelda tugma shunchaki «Tiklash» deb turadi).
	HiddenFromStatus string `bson:"hiddenFromStatus,omitempty" json:"-"`
	IsDeleted        bool   `bson:"isDeleted" json:"isDeleted"`
	// DeletedAt — qachon yashirilgani. Admin panelida ko'rsatiladi:
	// yashirilgan e'lon ro'yxatda qolgani uchun "qachon?" degan savolga
	// javob kerak bo'ladi. Yashirilmaganda umuman yozilmaydi.
	DeletedAt *time.Time `bson:"deletedAt,omitempty" json:"deletedAt,omitempty"`
	// ModerationPending — e'lon AI tekshiruvidan O'TMASDAN chop etilgan
	// (kvota tugagan yoki xizmat uzilgan paytda). Keyinchalik qo'lda ko'rib
	// chiqish uchun belgi.
	//
	// json:"-" ataylab: foydalanuvchi tekshiruv o'tkazib yuborilganini
	// bilmasligi kerak.
	ModerationPending bool                `bson:"moderationPending,omitempty" json:"-"`
	TelegramChannel   *ChannelPublication `bson:"telegramChannel,omitempty" json:"-"`
	// Denormalized moderation flag so public feed/sitemap queries can hide all
	// listings immediately when an owner is blocked without an expensive join.
	OwnerBlocked bool       `bson:"ownerBlocked,omitempty" json:"-"`
	PublishedAt  *time.Time `bson:"publishedAt,omitempty" json:"publishedAt,omitempty"`
	CreatedAt    time.Time  `bson:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time  `bson:"updatedAt" json:"updatedAt"`
	// Denormalized owner info for fast feed
	OwnerName         string  `bson:"ownerName,omitempty" json:"ownerName"`
	OwnerRating       float64 `bson:"ownerRating,omitempty" json:"ownerRating"`
	OwnerReviewsCount int     `bson:"ownerReviewsCount,omitempty" json:"ownerReviewsCount"`
	OwnerAvatarURL    string  `bson:"ownerAvatarUrl,omitempty" json:"ownerAvatarUrl"`
	// Image URLs (stored on S3).
	Images          []string            `bson:"images,omitempty" json:"images"`
	OwnerSnapshot   *ElonOwnerSnapshot  `bson:"ownerSnapshot,omitempty" json:"-"`
	ImagesRemovedAt *time.Time          `bson:"imagesRemovedAt,omitempty" json:"-"`
	ModerationJobs  []ElonModerationJob `bson:"adminModerationJobs,omitempty" json:"-"`
	PurgeEvent      *ElonPurgeEvent     `bson:"adminPurgeEvent,omitempty" json:"-"`

	// IsReviewData marks an elon created by the review account. Such elons are
	// filtered out of the public feed, search and sitemap, so a real user never
	// sees one. See internal/auth/review.go. Never serialised — see
	// User.IsReviewAccount.
	IsReviewData bool `bson:"isReviewData,omitempty" json:"-"`
}

// Application
type Application struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ElonID      primitive.ObjectID `bson:"elonId" json:"elonId"`
	ElonTitle   string             `bson:"elonTitle" json:"elonTitle"`
	WorkerID    primitive.ObjectID `bson:"workerId" json:"workerId"`
	EmployerID  primitive.ObjectID `bson:"employerId" json:"employerId"`
	WorkerPhone string             `bson:"workerPhone" json:"workerPhone"`
	// Ushbu ariza bilan nechta kishi ishga kelmoqchi (guruh bo'lib ariza).
	// Kamida 1. Ish beruvchi qabul qilganda e'lonning acceptedCount shu songa
	// oshadi.
	PeopleCount int `bson:"peopleCount" json:"peopleCount"`
	// Denormalized worker snapshot (ariza tushgan paytdagi holat) — ish beruvchi
	// nomzodlar ro'yxatini ko'rsatishi uchun.
	WorkerName         string  `bson:"workerName,omitempty" json:"workerName"`
	WorkerRating       float64 `bson:"workerRating,omitempty" json:"workerRating"`
	WorkerReviewsCount int     `bson:"workerReviewsCount,omitempty" json:"workerReviewsCount"`
	WorkerAvatarURL    string  `bson:"workerAvatarUrl,omitempty" json:"workerAvatarUrl"`
	WorkerVerified     bool    `bson:"workerVerified,omitempty" json:"workerVerified"`
	// Denormalized elon snapshot — ishchi o'z arizalari ro'yxatini ko'rsatishi uchun.
	ElonCategoryName      string              `bson:"elonCategoryName,omitempty" json:"elonCategoryName"`
	ElonCategoryID        primitive.ObjectID  `bson:"elonCategoryId,omitempty" json:"-"`
	ElonRegion            string              `bson:"elonRegion,omitempty" json:"elonRegion"`
	ElonDistrict          string              `bson:"elonDistrict,omitempty" json:"elonDistrict"`
	OwnerName             string              `bson:"ownerName,omitempty" json:"ownerName"`
	OwnerRating           float64             `bson:"ownerRating,omitempty" json:"ownerRating"`
	OwnerAvatarURL        string              `bson:"ownerAvatarUrl,omitempty" json:"ownerAvatarUrl"`
	Amount                int64               `bson:"amount" json:"amount"`
	ElonOwnerRevision     int64               `bson:"elonOwnerRevision,omitempty" json:"-"`
	ElonWorkDetails       *ListingWorkDetails `bson:"elonWorkDetails,omitempty" json:"-"`
	ListingRecheckPending bool                `bson:"listingRecheckPending,omitempty" json:"-"`
	IsNegotiable          bool                `bson:"isNegotiable" json:"isNegotiable"`
	Status                string              `bson:"status" json:"status"` // pending|accepted|rejected|cancelled|completed
	EmployerConfirmedDone bool                `bson:"employerConfirmedDone" json:"employerConfirmedDone"`
	WorkerConfirmedDone   bool                `bson:"workerConfirmedDone" json:"workerConfirmedDone"`
	// AutoCompleted — ish ikki tomon tasdig'isiz, belgilangan vaqtdan 18 soat
	// o'tgach avtomatik yakunlangan bo'lsa true. Tarix (arxiv) yozuvi qanday
	// yopilganini ajratish uchun.
	AutoCompleted bool       `bson:"autoCompleted" json:"autoCompleted"`
	CancelledBy   string     `bson:"cancelledBy,omitempty" json:"cancelledBy"`
	CancelReason  string     `bson:"cancelReason,omitempty" json:"cancelReason,omitempty"`
	AppliedAt     time.Time  `bson:"appliedAt" json:"appliedAt"`
	DecidedAt     *time.Time `bson:"decidedAt,omitempty" json:"decidedAt,omitempty"`
	CompletedAt   *time.Time `bson:"completedAt,omitempty" json:"completedAt,omitempty"`

	// IsReviewData marks an application submitted by the review account. The
	// employer is never notified about it and never sees it in their candidate
	// list, so a reviewer can exercise the full apply flow against a real elon
	// without any real user noticing. Never serialised — see
	// User.IsReviewAccount.
	IsReviewData bool `bson:"isReviewData,omitempty" json:"-"`
}

// Eslatma: User'dagi WorkerRating/EmployerRating/ReviewsCount maydonlari
// kelajakdagi "sharh va baho" funksiyasi uchun ajratilgan — hozircha ularni
// to'ldiradigan endpoint YO'Q (hech qayerda yozilmaydi, doim 0 turadi).

// Notification
type Notification struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID        primitive.ObjectID `bson:"userId" json:"userId"`
	Type          string             `bson:"type" json:"type"`
	Title         string             `bson:"title" json:"title"`
	Body          string             `bson:"body" json:"body"`
	RelatedEntity *RelatedEntity     `bson:"relatedEntity,omitempty" json:"relatedEntity,omitempty"`
	IsRead        bool               `bson:"isRead" json:"isRead"`
	CreatedAt     time.Time          `bson:"createdAt" json:"createdAt"`
	// Contact and exact location are used only for the accepted worker's
	// Telegram message; they are not added to the shared inbox/FCM payload.
	AcceptedJob *AcceptedJobDetails `bson:"acceptedJob,omitempty" json:"-"`

	// SentByAdminID — bu xabarni AYNAN SHU foydalanuvchiga qo'lda yuborgan
	// admin (internal/admin.NotifyUser).
	//
	// NEGA KERAK: qo'lda yuborilgan xabar ham, hammaga ketgan broadcast ham
	// bir xil `type: "system"` bilan saqlanadi va boshqa hech qanday farqi
	// yo'q edi. Admin panelida "shu foydalanuvchiga nima yozganman?" degan
	// savolga javob berish uchun ikkalasini ajratish shart — aks holda
	// ro'yxatga hamma olgan e'lonlar ham tushib, javob noto'g'ri bo'lardi.
	//
	// Broadcast'da ATAYLAB to'ldirilmaydi: shu maydonning yo'qligi
	// "bu shaxsiy xabar emas" degani.
	//
	// omitempty — eski yozuvlarda maydon umuman bo'lmaydi. Ular
	// ro'yxatga tushmaydi va bu to'g'ri: ular haqiqatan qaysi yo'l bilan
	// yuborilgani endi bilib bo'lmaydi, taxmin qilib ko'rsatgandan ko'ra
	// ko'rsatmagan ma'qul.
	SentByAdminID primitive.ObjectID `bson:"sentByAdminId,omitempty" json:"sentByAdminId,omitempty"`
}
type RelatedEntity struct {
	Type string             `bson:"type" json:"type"`
	ID   primitive.ObjectID `bson:"id" json:"id"`
}

// DeviceToken — mobil qurilmaning FCM push tokeni. Bitta token har doim bitta
// foydalanuvchiga tegishli: qurilmada akkaunt almashsa, token yangi egasiga
// ko'chiriladi (internal/push.Handler.Register upsert'i).
type DeviceToken struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID `bson:"userId" json:"userId"`
	Token     string             `bson:"token" json:"token"`
	Platform  string             `bson:"platform" json:"platform"` // android|ios
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// Report
type Report struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ReporterID  primitive.ObjectID `bson:"reporterId" json:"reporterId"`
	TargetType  string             `bson:"targetType" json:"targetType"` // user|elon|message
	TargetID    primitive.ObjectID `bson:"targetId" json:"targetId"`
	Reason      string             `bson:"reason" json:"reason"`
	Description string             `bson:"description,omitempty" json:"description"`
	Status      string             `bson:"status" json:"status"` // open|resolved|dismissed
	ReviewedBy  primitive.ObjectID `bson:"reviewedBy,omitempty" json:"reviewedBy,omitempty"`
	ReviewedAt  *time.Time         `bson:"reviewedAt,omitempty" json:"reviewedAt,omitempty"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
}

// Feedback — foydalanuvchilardan kelgan taklif va shikoyatlar.
type Feedback struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID     primitive.ObjectID `bson:"userId" json:"userId"`
	UserName   string             `bson:"userName,omitempty" json:"userName"`
	UserPhone  string             `bson:"userPhone,omitempty" json:"userPhone"`
	Type       string             `bson:"type" json:"type"` // suggestion|complaint
	Subject    string             `bson:"subject,omitempty" json:"subject"`
	Message    string             `bson:"message" json:"message"`
	Status     string             `bson:"status" json:"status"` // open|resolved
	ReviewedBy primitive.ObjectID `bson:"reviewedBy,omitempty" json:"reviewedBy,omitempty"`
	ReviewedAt *time.Time         `bson:"reviewedAt,omitempty" json:"reviewedAt,omitempty"`
	CreatedAt  time.Time          `bson:"createdAt" json:"createdAt"`
}

// Admin
type Admin struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Username     string             `bson:"username" json:"username"`
	Name         string             `bson:"name,omitempty" json:"name"`
	PasswordHash string             `bson:"passwordHash" json:"-"`
	Role         string             `bson:"role" json:"role"`
	IsActive     bool               `bson:"isActive" json:"isActive"`
	// Two-factor (TOTP). TOTPSecret is never serialized to clients. TOTPEnabled
	// is true only after the admin verifies a code during enrollment.
	TOTPSecret  string `bson:"totpSecret,omitempty" json:"-"`
	TOTPEnabled bool   `bson:"totpEnabled" json:"totpEnabled"`
	// TOTPLastCounter is the 30-second counter of the last code accepted for
	// this admin. Codes at or below it are refused, making every code single-use
	// (RFC 6238 §5.2) — a shoulder-surfed or phished code cannot be replayed
	// within its skew window. Never serialized.
	TOTPLastCounter uint64 `bson:"totpLastCounter,omitempty" json:"-"`
	// Incremented whenever credentials, role, active state, or logout changes
	// the session. Admin JWTs carry the matching value and are rejected when it
	// differs, providing immediate revocation for privileged sessions.
	TokenVersion int `bson:"tokenVersion,omitempty" json:"-"`
	// LastActivityAt — bu hisob OXIRGI marta qachon ishlatilgani, klientdan
	// qat'i nazar. Veb panel ham, mobil admin ilovasi ham SHU maydonni
	// yangilaydi, chunki "foydalanilmasa chiqarib yuborish" oynasi hisob
	// bo'yicha yagona: bir joyda ishlash ikkinchisini ham tirik saqlashi
	// kerak (config.AdminIdleTTL, internal/admin/refresh.go).
	//
	// Har so'rovda emas, daqiqada bir marta yoziladi — aniqligi 3 kunlik oyna
	// uchun yetarli, yozuv yuki esa nolga yaqin.
	LastActivityAt time.Time `bson:"lastActivityAt,omitempty" json:"lastActivityAt,omitempty"`
	CreatedAt      time.Time `bson:"createdAt" json:"createdAt"`
}

// AdminSession — bitta qurilmadagi admin sessiyasi (veb brauzer yoki mobil
// ilova). Access token qisqa umr ko'radi; uni yangilab turadigan refresh
// token shu yerda yashaydi.
//
// Xom token SAQLANMAYDI — faqat SHA-256 xeshi. Bazani o'qiy olgan hujum
// shundan tokenni tiklay olmaydi. Butun mexanizm va nega aynan shunday
// qilingani: internal/admin/refresh.go.
type AdminSession struct {
	ID      primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	AdminID primitive.ObjectID `bson:"adminId" json:"adminId"`
	// TokenHash — hozir kuchda bo'lgan refresh tokenning xeshi.
	TokenHash string `bson:"tokenHash" json:"-"`
	// PrevTokenHash/PrevValidUntil — aylantirishdan keyingi qisqa imtiyoz
	// oynasi: javob mijozga yetib bormasa (tarmoq uzilishi) eski token
	// PrevValidUntil gacha qabul qilinaveradi va admin bekorga chiqarib
	// yuborilmaydi.
	PrevTokenHash  string    `bson:"prevTokenHash,omitempty" json:"-"`
	PrevValidUntil time.Time `bson:"prevValidUntil,omitempty" json:"-"`
	// TokenVersion — sessiya ochilgan paytdagi Admin.TokenVersion. Farq qilsa
	// (logout, parol yoki rol o'zgarishi) sessiya kuchdan qoladi.
	TokenVersion int    `bson:"tokenVersion" json:"-"`
	Platform     string `bson:"platform,omitempty" json:"platform,omitempty"`
	// IP — sessiya ochilgan manzil. Faqat audit uchun; klientga chiqmaydi.
	IP         string    `bson:"ip,omitempty" json:"-"`
	CreatedAt  time.Time `bson:"createdAt" json:"createdAt"`
	LastUsedAt time.Time `bson:"lastUsedAt" json:"lastUsedAt"`
	// ExpiresAt — QAT'IY yuqori chegara (faollikdan qat'i nazar) va TTL
	// indeksi uchun belgi. 3 kunlik "foydalanilmasa" oynasi bu yerda emas,
	// Admin.LastActivityAt da hisoblanadi.
	ExpiresAt time.Time `bson:"expiresAt" json:"expiresAt"`
}

// Broadcast — admin ommaviy bildirishnomasi tarixi. Yuborish fon jarayonida
// bajariladi; SentCount va Status yuborish tugagach yangilanadi.
type Broadcast struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title       string             `bson:"title" json:"title"`
	Body        string             `bson:"body" json:"body"`
	Region      string             `bson:"region,omitempty" json:"region"`
	ActiveOnly  bool               `bson:"activeOnly" json:"activeOnly"`
	SentCount   int                `bson:"sentCount" json:"sentCount"`
	Status      string             `bson:"status" json:"status"` // scheduled|sending|done
	ScheduledAt *time.Time         `bson:"scheduledAt,omitempty" json:"scheduledAt,omitempty"`
	CreatedBy   primitive.ObjectID `bson:"createdBy,omitempty" json:"createdBy"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
}

// AdminAuditLog
type AdminAudit struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	AdminID   primitive.ObjectID `bson:"adminId" json:"adminId"`
	Action    string             `bson:"action" json:"action"`
	Target    string             `bson:"target,omitempty" json:"target"`
	Detail    string             `bson:"detail,omitempty" json:"detail"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	// Structured listing moderation fields preserve reasons without parsing
	// human-readable detail text. Older audit rows keep their legacy fallback.
	Kind        string `bson:"kind,omitempty" json:"kind,omitempty"`
	FromStatus  string `bson:"fromStatus,omitempty" json:"fromStatus,omitempty"`
	Status      string `bson:"status,omitempty" json:"status,omitempty"`
	Reason      string `bson:"reason,omitempty" json:"reason,omitempty"`
	NotifyOwner *bool  `bson:"notifyOwner,omitempty" json:"notifyOwner,omitempty"`
	// Until is the original deadline of a moderation_ban. Keeping it in
	// the audit record preserves the ban after current user fields clear.
	Until *time.Time `bson:"until,omitempty" json:"until,omitempty"`
}

// ErrorGroup — bir xil DASTUR xatoligining yig'ma yozuvi ("3.12 · Xatoliklar"
// sahifasidagi bitta qator). Guruhlash kaliti — Fingerprint: kod + qayerda
// (fayl/funksiya) + normallashtirilgan yo'l. Bir xil nosozlik minglab marta
// takrorlansa ham bitta hujjat bo'lib qoladi, faqat Count/UsersCount o'sadi.
//
// Hech qanday erkin matn xom holda kirmaydi: Title katalogdan (errlog),
// Message va Where esa errlog.Text() dan o'tadi.
type ErrorGroup struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Fingerprint string             `bson:"fingerprint" json:"fingerprint"`
	// Ref — panelda ko'rinadigan qisqa yorliq: ERR-2F91C4.
	Ref      string `bson:"ref" json:"ref"`
	Code     string `bson:"code" json:"code"`
	Module   string `bson:"module" json:"module"`     // backend|db|external|jobs|admin_app|client_app|security
	Severity string `bson:"severity" json:"severity"` // critical|high|medium|low
	// SevRank — Severity ning raqamli tartibi. Mongo satrni alifbo bo'yicha
	// saralaydi ("critical < high < low < medium"), shuning uchun saralash
	// uchun alohida maydon kerak.
	SevRank int    `bson:"sevRank" json:"-"`
	Runtime string `bson:"runtime" json:"runtime"` // Backend | Admin ilova | OTP bot | …
	Title   string `bson:"title" json:"title"`
	Where   string `bson:"where,omitempty" json:"where,omitempty"`
	Message string `bson:"message,omitempty" json:"message,omitempty"`
	Path    string `bson:"path,omitempty" json:"path,omitempty"`
	// LastDevice / LastAppVersion — oxirgi namunadagi qurilma yorlig'i
	// ("Xiaomi Redmi Note 12 · Android 14") va ilova versiyasi. Ro'yxatdagi
	// "Qurilma" va "Ilova versiyasi" ustunlari uchun (Figma 3.12.3 · N).
	//
	// Guruhda ATAYLAB nusxa qilib saqlanadi: ro'yxat bir sahifada 9 qator
	// chizadi, har biri uchun namunalar kolleksiyasiga borish 9 qo'shimcha
	// so'rov bo'lardi. Qiymat "oxirgi ma'lum" degan ma'noda — taqsimot
	// emas; taqsimot batafsil ekranda, hodisalar bo'yicha hisoblanadi.
	LastDevice     string `bson:"lastDevice,omitempty" json:"lastDevice,omitempty"`
	LastAppVersion string `bson:"lastAppVersion,omitempty" json:"lastAppVersion,omitempty"`
	Count          int64  `bson:"count" json:"count"`
	// UsersCount — ta'sirlangan noyob foydalanuvchilar soni. Foydalanuvchi
	// ID'lari SAQLANMAYDI: faqat sanoq va oxirgi 64 tasining hash'i (Users)
	// yuriladi, ya'ni "kim" emas, "qancha" javobi qoladi.
	UsersCount  int64      `bson:"usersCount" json:"usersCount"`
	UserHashes  []string   `bson:"userHashes,omitempty" json:"-"`
	Status      string     `bson:"status" json:"status"` // new|watching|fixing|resolved|regressed|ignored
	Note        string     `bson:"note,omitempty" json:"note,omitempty"`
	FirstSeenAt time.Time  `bson:"firstSeenAt" json:"firstSeenAt"`
	LastSeenAt  time.Time  `bson:"lastSeenAt" json:"lastSeenAt"`
	ResolvedAt  *time.Time `bson:"resolvedAt,omitempty" json:"resolvedAt,omitempty"`
	ResolvedBy  string     `bson:"resolvedBy,omitempty" json:"resolvedBy,omitempty"`

	// ── Hayot sikli (Figma 3.12.3 · J) ──────────────────────────────────
	//
	// BaseSeverity — KATALOGDAGI asl daraja. Regressiya darajani bir
	// pog'ona ko'taradi (errlog.BumpSeverity); asl qiymat saqlanmasa, har
	// takrorlanishda daraja yana ko'tarilib, oxiri hamma narsa "Kritik"
	// bo'lib qolardi.
	BaseSeverity string `bson:"baseSeverity,omitempty" json:"baseSeverity,omitempty"`
	// AssigneeID/Assignee — mas'ul admin. Yorliq ("@login") ham saqlanadi:
	// admin o'chirilsa ham tarix o'qiladigan bo'lib qoladi.
	AssigneeID primitive.ObjectID `bson:"assigneeId,omitempty" json:"-"`
	Assignee   string             `bson:"assignee,omitempty" json:"assignee,omitempty"`
	// StartedAt/PlannedVersion/FixNote — "Bartaraf etilmoqda" paneli.
	StartedAt      *time.Time `bson:"startedAt,omitempty" json:"startedAt,omitempty"`
	PlannedVersion string     `bson:"plannedVersion,omitempty" json:"plannedVersion,omitempty"`
	FixNote        string     `bson:"fixNote,omitempty" json:"fixNote,omitempty"`
	// FixedVersion — tuzatish chiqarilgan versiya ("1.4.3 (121)").
	FixedVersion string `bson:"fixedVersion,omitempty" json:"fixedVersion,omitempty"`
	// ClosedVersion — guruh YOPILGAN paytdagi versiya. Regressiyada u
	// joyida qoladi: "1.4.3 da yopilgan edi, keyin qaytdi".
	ClosedVersion string     `bson:"closedVersion,omitempty" json:"closedVersion,omitempty"`
	ReopenedAt    *time.Time `bson:"reopenedAt,omitempty" json:"reopenedAt,omitempty"`
	// IgnoreReason — "E'tiborsiz qoldirish" uchun MAJBURIY sabab
	// (Figma 3.12.3 · J). Nosozlikni ko'rinmas qiladigan yagona tugma
	// izohsiz bosilmasligi kerak.
	IgnoreReason string `bson:"ignoreReason,omitempty" json:"ignoreReason,omitempty"`
	// Activity — "Amallar tarixi va izohlar" tasmasi (oxirgi 50 ta,
	// $slice bilan cheklangan). Audit jurnalining o'rnini bosmaydi: bu —
	// guruh yonidagi qisqa tasma, audit esa o'zgarmas manba.
	Activity []ErrorActivity `bson:"activity,omitempty" json:"activity,omitempty"`

	// AlertedAt — oxirgi Telegram ogohlantirishi. Bitta xatolik sikli
	// tunda yuzta xabar yubormasligi uchun (recorder.go dagi throttle).
	AlertedAt *time.Time `bson:"alertedAt,omitempty" json:"-"`
	// AlertedCount — ogohlantirish yuborilgan paytdagi Count. "Yuqori"
	// darajali xatolik har 10 hodisada bir marta eslatiladi.
	AlertedCount int64 `bson:"alertedCount,omitempty" json:"-"`
	// TgSentAt — admin QO'LDA yuborgan oxirgi Telegram xabari. Avtomatik
	// ogohlantirishdan alohida hisoblanadi, o'z sovish oynasi bilan.
	TgSentAt *time.Time `bson:"tgSentAt,omitempty" json:"tgSentAt,omitempty"`

	// AI — oxirgi AI tahlili (internal/admin/errai.go). Guruh ichida
	// saqlanadi, chunki u guruhga tegishli va guruh bilan birga
	// o'chadi; alohida kolleksiya bo'lsa batafsil ekran yana bitta
	// so'rov qilardi. Tarixi yuritilmaydi — oxirgisi qoladi, avvalgilari
	// esa `activity` tasmasida qator bo'lib turadi.
	AI *ErrorAI `bson:"ai,omitempty" json:"ai,omitempty"`
}

// ErrorAI — Gemini qaytargan ildiz-sabab xulosasi (Figma 3.12.1 · AI
// tahlili; kontekst manbai 3.12.3 · L).
//
// # Nega natija saqlanadi
//
// Chaqiruv pullik va sekin (~4 s). Bir xil guruhni har ochganda qayta
// so'rash kvotani yeydi va har safar boshqacha matn berardi — ya'ni
// adminlar bir-birining ko'rgan xulosasini ko'rmasdi. Saqlangan natija
// esa jamoaviy: kim so'ragani (`by`) va qachon (`at`) ko'rinib turadi.
//
// # Nega "eskirgan" belgisi bor
//
// CountAt — tahlil paytidagi hodisalar soni. Keyin xatolik yana ming
// marta takrorlansa, xulosa hamon eski hisobotga tayangan bo'ladi;
// panel shuni ochiq aytadi va qayta so'rashni taklif qiladi.
type ErrorAI struct {
	Sarlavha   string    `bson:"sarlavha" json:"sarlavha"`
	Sabab      string    `bson:"sabab" json:"sabab"`
	Qayerda    string    `bson:"qayerda,omitempty" json:"qayerda,omitempty"`
	Tuzatish   []string  `bson:"tuzatish,omitempty" json:"tuzatish,omitempty"`
	Tekshirish []string  `bson:"tekshirish,omitempty" json:"tekshirish,omitempty"`
	Ishonch    string    `bson:"ishonch,omitempty" json:"ishonch,omitempty"` // past|o'rta|yuqori
	Model      string    `bson:"model,omitempty" json:"model,omitempty"`
	Tokens     int       `bson:"tokens,omitempty" json:"tokens,omitempty"`
	Include    []string  `bson:"include,omitempty" json:"include,omitempty"`
	CountAt    int64     `bson:"countAt,omitempty" json:"countAt,omitempty"`
	At         time.Time `bson:"at" json:"at"`
	By         string    `bson:"by,omitempty" json:"by,omitempty"`
}

// ErrorActivity — guruh tasmasidagi bitta yozuv (holat, izoh, Telegram).
type ErrorActivity struct {
	Kind  string    `bson:"kind" json:"kind"` // status|note|assign|telegram|regressed|export|ai
	Text  string    `bson:"text" json:"text"`
	Actor string    `bson:"actor,omitempty" json:"actor,omitempty"` // bo'sh => tizim
	At    time.Time `bson:"at" json:"at"`
}

// ErrorDevice — hodisa yuz bergan qurilma (Figma 3.12.3 · H).
//
// Manba: `X-Client-Device` sarlavhasi (mobil ilova uni device_info_plus /
// package_info_plus orqali to'ldiradi) yoki veb uchun `User-Agent` va
// `Sec-CH-UA-*`. Sarlavha TASHQARIDAN keladi — internal/errlog/device.go
// uni kalitlar ro'yxati, belgilar to'plami va uzunlik bo'yicha qat'iy
// tozalaydi. Bo'sh maydon panelda "aniqlanmagan" bo'lib chiqadi.
type ErrorDevice struct {
	Platform    string `bson:"platform,omitempty" json:"platform,omitempty"` // android|ios|web
	Brand       string `bson:"brand,omitempty" json:"brand,omitempty"`
	Model       string `bson:"model,omitempty" json:"model,omitempty"`
	ModelCode   string `bson:"modelCode,omitempty" json:"modelCode,omitempty"`
	OS          string `bson:"os,omitempty" json:"os,omitempty"`
	OSVersion   string `bson:"osVersion,omitempty" json:"osVersion,omitempty"`
	APILevel    string `bson:"apiLevel,omitempty" json:"apiLevel,omitempty"`
	AppVersion  string `bson:"appVersion,omitempty" json:"appVersion,omitempty"`
	Build       string `bson:"build,omitempty" json:"build,omitempty"`
	Flutter     string `bson:"flutter,omitempty" json:"flutter,omitempty"`
	Dart        string `bson:"dart,omitempty" json:"dart,omitempty"`
	Screen      string `bson:"screen,omitempty" json:"screen,omitempty"`
	RAM         string `bson:"ram,omitempty" json:"ram,omitempty"`
	Storage     string `bson:"storage,omitempty" json:"storage,omitempty"`
	Locale      string `bson:"locale,omitempty" json:"locale,omitempty"`
	Network     string `bson:"network,omitempty" json:"network,omitempty"`
	Battery     string `bson:"battery,omitempty" json:"battery,omitempty"`
	Emulator    string `bson:"emulator,omitempty" json:"emulator,omitempty"`
	Orientation string `bson:"orientation,omitempty" json:"orientation,omitempty"`
	Browser     string `bson:"browser,omitempty" json:"browser,omitempty"`
	Engine      string `bson:"engine,omitempty" json:"engine,omitempty"`
}

// ErrorStep — xatolikdan OLDINGI qadam (breadcrumb, Figma 3.12.3 · I).
//
// Mijoz ularni halqali buferda (oxirgi 20 ta) yuritadi va FAQAT xatolik
// bilan birga yuboradi. Matn errlog.Text() dan o'tadi: token, telefon va
// IP shu yerda ham niqoblanadi.
type ErrorStep struct {
	At   time.Time `bson:"at" json:"at"`
	Kind string    `bson:"kind" json:"kind"` // nav|screen|action|request|response|crash
	Text string    `bson:"text" json:"text"`
}

// ErrorEvent — bitta hodisa. Guruh 180 kun yashaydi, hodisa esa 30 kun
// (TTL indeks, pkg/db/indexes.go) — grafik va "24 soatdagi hodisalar"
// hisoblagichi uchun shuncha yetarli.
type ErrorEvent struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Fingerprint string             `bson:"fingerprint" json:"fingerprint"`
	Code        string             `bson:"code" json:"code"`
	Severity    string             `bson:"severity" json:"severity"`
	Module      string             `bson:"module" json:"module"`
	Where       string             `bson:"where,omitempty" json:"where,omitempty"`
	Message     string             `bson:"message,omitempty" json:"message,omitempty"`
	Method      string             `bson:"method,omitempty" json:"method,omitempty"`
	Path        string             `bson:"path,omitempty" json:"path,omitempty"`
	Status      int                `bson:"status,omitempty" json:"status,omitempty"`
	// N — bu hujjat nechta takrorlanishni ifodalaydi. Recorder hodisalarni
	// bir necha soniyalik oynada yig'ib, oynaga bitta hujjat yozadi: shu
	// tariqa xatolik sikli (sekundiga minglab) bazani ko'chki bilan
	// to'ldirmaydi, lekin "24 soatdagi hodisalar" sanog'i aniq qoladi
	// ($sum: "$n").
	N int `bson:"n" json:"n"`
	// UserHash — foydalanuvchining ID'sidan olingan qaytarib bo'lmaydigan
	// hash. Xom ID emas: jurnal "kim nima qildi" tarixiga aylanmasligi kerak.
	UserHash   string `bson:"userHash,omitempty" json:"-"`
	Platform   string `bson:"platform,omitempty" json:"platform,omitempty"`
	AppVersion string `bson:"appVersion,omitempty" json:"appVersion,omitempty"`
	// Brand/OS — "Ta'sir taqsimoti" uchun (Figma 3.12.3 · K). Ular AYNAN
	// shu yerda turadi, namunada emas: namunalar guruhda 20 tadan oshmaydi,
	// ya'ni ular bo'yicha hisoblangan foiz butun hodisalar oqimini emas,
	// oxirgi yigirmatasini ko'rsatardi.
	Brand string    `bson:"brand,omitempty" json:"brand,omitempty"`
	OS    string    `bson:"os,omitempty" json:"os,omitempty"`
	At    time.Time `bson:"at" json:"at"`
}

// ErrorSample — bitta hodisaning TO'LIQ nusxasi (Figma 3.12.1 · "So'nggi
// hodisalar", 3.12.3 · H va I).
//
// # NEGA ALOHIDA KOLLEKSIYA
//
// `error_events` — sanoq uchun: u ko'p, kichik va faqat grafikni chizadi.
// Namuna esa og'ir (stek, qadamlar, qurilma) va har guruh uchun ATIGI bir
// nechtasi saqlanadi (errlog.maxSamples). Ikkalasini birlashtirsak, yo
// grafik og'irlashardi, yo batafsil ko'rinish bo'shab qolardi.
//
// # SHAXSIY MA'LUMOT
//
// Guruh va hodisa darajasida foydalanuvchi faqat qaytarib bo'lmaydigan
// hash bo'lib turadi. Namunada esa `userId` — ObjectID — saqlanadi, chunki
// "kim duch keldi" paneli usiz umuman ishlamaydi. Muvozanat quyidagicha:
// har guruhda ≤ maxSamples ta namuna, 30 kunlik TTL, moderator+ RBAC,
// ism va telefon SAQLANMAYDI (o'qish paytida `users` dan olinadi) va
// telefon javobda HAR DOIM niqoblangan holda ketadi.
type ErrorSample struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Fingerprint string             `bson:"fingerprint" json:"-"`
	Code        string             `bson:"code" json:"code"`
	At          time.Time          `bson:"at" json:"at"`

	Method     string `bson:"method,omitempty" json:"method,omitempty"`
	Path       string `bson:"path,omitempty" json:"path,omitempty"`
	Status     int    `bson:"status,omitempty" json:"status,omitempty"`
	DurationMs int    `bson:"durationMs,omitempty" json:"durationMs,omitempty"`
	RequestID  string `bson:"requestId,omitempty" json:"requestId,omitempty"`

	UserID   primitive.ObjectID `bson:"userId,omitempty" json:"-"`
	AdminID  primitive.ObjectID `bson:"adminId,omitempty" json:"-"`
	UserHash string             `bson:"userHash,omitempty" json:"-"`

	Device  ErrorDevice `bson:"device,omitempty" json:"device"`
	Message string      `bson:"message,omitempty" json:"message,omitempty"`
	Stack   []string    `bson:"stack,omitempty" json:"stack,omitempty"`
	Steps   []ErrorStep `bson:"steps,omitempty" json:"steps,omitempty"`
}

// OTPCode
type OTPCode struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	TGToken    string             `bson:"tgToken"`
	Phone      string             `bson:"phone,omitempty"`
	TelegramID int64              `bson:"telegramId,omitempty"`
	Code       string             `bson:"code,omitempty"`
	Attempts   int                `bson:"attempts"`
	ExpiresAt  time.Time          `bson:"expiresAt"`
	Used       bool               `bson:"used"`
	CreatedAt  time.Time          `bson:"createdAt"`
}
