package application

import (
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"net/http"
)

// Filter before pagination so accepted jobs are not lost behind older pages.
func applicationStatusFilter(w http.ResponseWriter, r *http.Request, filter bson.M) bool {
	switch status := r.URL.Query().Get("status"); status {
	case "", "all":
	case "pending", "accepted", "completed", "cancelled", "rejected":
		filter["status"] = status
	case "history":
		filter["status"] = bson.M{"$in": []string{"completed", "cancelled", "rejected"}}
	default:
		httpx.Err(w, httpx.NewError(400, "bad_status", "Ariza holati noto'g'ri."))
		return false
	}
	return true
}
