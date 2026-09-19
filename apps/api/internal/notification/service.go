package notification

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Pusher pushes a serialized notification to any online WS clients for a user.
type Pusher interface {
	PushUser(userID primitive.ObjectID, kind string, payload any)
}

type Service struct {
	Col *mongo.Collection
	// Users backs the review-sandbox check in Push. Optional: when nil, Push
	// simply drops anything the review account would have triggered.
	Users    *mongo.Collection
	Pusher   Pusher
	Telegram *Telegram
}

func New(db *mongo.Database) *Service {
	return &Service{
		Col:   db.Collection("notifications"),
		Users: db.Collection("users"),
	}
}

func (s *Service) AttachPusher(p Pusher) { s.Pusher = p }

func (s *Service) AttachTelegram(t *Telegram) { s.Telegram = t }

func (s *Service) Push(ctx context.Context, userID primitive.ObjectID, typ, title, body string, rel *models.RelatedEntity) {
	s.push(ctx, models.Notification{UserID: userID, Type: typ, Title: title, Body: body, RelatedEntity: rel})
}

func (s *Service) PushAcceptedJob(ctx context.Context, workerID, applicationID primitive.ObjectID, jobTitle string, details *models.AcceptedJobDetails) {
	s.push(ctx, models.Notification{
		UserID: workerID, Type: "application_accepted", Title: "Arizangiz qabul qilindi", Body: jobTitle,
		RelatedEntity: &models.RelatedEntity{Type: "application", ID: applicationID}, AcceptedJob: details,
	})
}

// PushFromAdmin — admin AYNAN SHU foydalanuvchiga qo'lda yozgan xabar
// (internal/admin.NotifyUser).
//
// [Push] dan farqi bitta: yozuvda yuborgan admin qayd etiladi. Shu belgi
// tufayli admin panelida "men bu odamga nima yozganman?" degan savolga
// javob berish mumkin — broadcast bilan bir xil `type: "system"` bo'lsa
// ham ular endi ajratiladi.
//
// Alohida metod, [Push] ga qo'shimcha parametr emas: Push o'nlab joydan
// chaqiriladi va ularning hech biri adminga aloqador emas — hammasiga
// bo'sh qiymat uzatish shovqin bo'lardi.
func (s *Service) PushFromAdmin(ctx context.Context, userID, adminID primitive.ObjectID, title, body string) {
	s.push(ctx, models.Notification{UserID: userID, Type: "system", Title: title, Body: body, SentByAdminID: adminID})
}

func (s *Service) push(ctx context.Context, n models.Notification) {
	// Sandbox choke point for the Google Play review account.
	//
	// Every notification in the app funnels through here, so this single check
	// is what guarantees requirement "a real user must never notice that a
	// review account exists": whatever the reviewer does — applying to a real
	// elon, accepting, cancelling — nothing is ever delivered to a real person.
	// Traffic between the review account and its seeded demo counterparties
	// still flows, so the reviewer sees notifications working.
	//
	// Fails closed: if the recipient cannot be resolved, the notification is
	// dropped rather than delivered.
	if httpx.IsReviewActor(ctx) && !s.recipientIsReviewAccount(ctx, n.UserID) {
		return
	}
	n.CreatedAt = time.Now()
	res, err := s.Col.InsertOne(ctx, s.document(n))
	if err == nil {
		if oid, ok := res.InsertedID.(primitive.ObjectID); ok {
			n.ID = oid
		}
		if s.Telegram != nil {
			s.Telegram.wake()
		}
	} else {
		// Best-effort bo'lib qoladi (chaqiruvchini yiqitmaymiz), lekin jim
		// yutilmasin: insert yiqilsa foydalanuvchi bildirishnomani umuman
		// ko'rmaydi — buni logsiz payqash iloji yo'q edi.
		slog.Error("notification insert failed", "type", n.Type, "user", n.UserID.Hex(), "err", err)
	}
	if s.Pusher != nil {
		s.Pusher.PushUser(n.UserID, "notification", n)
	}
}

// recipientIsReviewAccount reports whether a notification target is part of the
// review sandbox. Only consulted for review-actor traffic, so it adds no query
// to the normal path.
func (s *Service) recipientIsReviewAccount(ctx context.Context, userID primitive.ObjectID) bool {
	if s.Users == nil {
		return false
	}
	var u models.User
	err := s.Users.FindOne(ctx,
		bson.M{"_id": userID},
		options.FindOne().SetProjection(bson.M{"isReviewAccount": 1}),
	).Decode(&u)
	return err == nil && u.IsReviewAccount
}

func (s *Service) List(w http.ResponseWriter, r *http.Request) {
	uid, _ := primitive.ObjectIDFromHex(httpx.UserID(r))
	// Ixtiyoriy limit/page. Paramsiz eski klientlar avvalgidek eng yangi 200
	// tasini oladi (javob shakli o'zgarmagan — oddiy massiv).
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	cur, err := s.Col.Find(r.Context(),
		bson.M{"userId": uid},
		options.Find().
			SetSort(bson.D{{Key: "createdAt", Value: -1}}).
			SetSkip(int64((page-1)*limit)).
			SetLimit(int64(limit)))
	if err != nil {
		httpx.Err(w, err)
		return
	}
	defer cur.Close(r.Context())
	out := []models.Notification{}
	for cur.Next(r.Context()) {
		var n models.Notification
		if err := cur.Decode(&n); err == nil {
			out = append(out, n)
		}
	}
	httpx.JSON(w, 200, out)
}

func (s *Service) ReadAll(w http.ResponseWriter, r *http.Request) {
	uid, _ := primitive.ObjectIDFromHex(httpx.UserID(r))
	if _, err := s.Col.UpdateMany(r.Context(), bson.M{"userId": uid, "isRead": false}, bson.M{"$set": bson.M{"isRead": true}}); err != nil {
		slog.Error("notification readAll failed", "user", uid.Hex(), "err", err)
	}
	httpx.JSON(w, 200, map[string]bool{"ok": true})
}

type readReq struct {
	NotificationIDs []string `json:"notificationIds"`
	RelatedIDs      []string `json:"relatedIds"`
	RelatedType     string   `json:"relatedType"`
}

// Read marks a targeted subset of the user's unread notifications read — either
// a notification's own id (notificationIds), those tied to specific related
// entities (relatedIds), or a whole related type (relatedType, e.g.
// "application"). Used by both the notification detail screen and related
// application/elon screens.
func (s *Service) Read(w http.ResponseWriter, r *http.Request) {
	uid, _ := primitive.ObjectIDFromHex(httpx.UserID(r))
	var req readReq
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Err(w, err)
		return
	}
	if len(req.NotificationIDs) > 200 || len(req.RelatedIDs) > 200 || len(req.RelatedType) > 64 {
		httpx.Err(w, httpx.NewError(400, "too_many_ids", "notification selection is too large"))
		return
	}
	filter := bson.M{"userId": uid, "isRead": false}
	switch {
	case len(req.NotificationIDs) > 0:
		oids := objectIDs(req.NotificationIDs)
		if len(oids) == 0 {
			httpx.JSON(w, 200, map[string]bool{"ok": true})
			return
		}
		// userId remains in the filter, so a user can never mark another
		// user's notification read even if they know its ObjectID.
		filter["_id"] = bson.M{"$in": oids}
	case len(req.RelatedIDs) > 0:
		oids := objectIDs(req.RelatedIDs)
		if len(oids) == 0 {
			httpx.JSON(w, 200, map[string]bool{"ok": true})
			return
		}
		filter["relatedEntity.id"] = bson.M{"$in": oids}
	case req.RelatedType != "":
		filter["relatedEntity.type"] = req.RelatedType
	default:
		httpx.Err(w, httpx.NewError(400, "bad_request", "notificationIds, relatedIds yoki relatedType kerak"))
		return
	}
	if _, err := s.Col.UpdateMany(r.Context(), filter, bson.M{"$set": bson.M{"isRead": true}}); err != nil {
		slog.Error("notification read failed", "user", uid.Hex(), "err", err)
	}
	httpx.JSON(w, 200, map[string]bool{"ok": true})
}

func objectIDs(hexIDs []string) []primitive.ObjectID {
	oids := make([]primitive.ObjectID, 0, len(hexIDs))
	for _, h := range hexIDs {
		if oid, err := primitive.ObjectIDFromHex(h); err == nil {
			oids = append(oids, oid)
		}
	}
	return oids
}
