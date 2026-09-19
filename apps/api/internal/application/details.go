package application

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Get opens a specific application from a notification, including archived
// applications outside the first page of either participant's inbox.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	uid, err := primitive.ObjectIDFromHex(httpx.UserID(r))
	if err != nil || uid.IsZero() {
		httpx.Err(w, httpx.NewError(401, "unauthorized", "Hisobingizga kiring."))
		return
	}
	id, err := primitive.ObjectIDFromHex(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Err(w, httpx.NewError(400, "bad_id", "Ariza manzili noto'g'ri."))
		return
	}
	filter := bson.M{"_id": id, "$or": bson.A{bson.M{"workerId": uid}, bson.M{"employerId": uid}}}
	if !httpx.IsReviewActor(r.Context()) {
		filter["isReviewData"] = bson.M{"$ne": true}
	}
	var a models.Application
	if err := h.Apps.FindOne(r.Context(), filter).Decode(&a); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			httpx.Err(w, httpx.NewError(404, "not_found", "Ariza topilmadi yoki uni ko'rish huquqingiz yo'q."))
		} else {
			httpx.Err(w, err)
		}
		return
	}
	apps := []models.Application{a}
	h.liveAppAvatars(r.Context(), apps)
	httpx.JSON(w, 200, apps[0])
}
