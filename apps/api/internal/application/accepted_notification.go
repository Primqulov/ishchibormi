package application

import (
	"context"
	"strings"

	"github.com/ishchibormi/backend/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (h *Handler) notifyAccepted(ctx context.Context, app models.Application, listing models.Elon) {
	details := &models.AcceptedJobDetails{
		Work: *listing.WorkDetails(), EmployerName: strings.TrimSpace(listing.OwnerName),
		ContactPhone: strings.TrimSpace(listing.ContactPhone),
	}
	// The listing's chosen contact takes precedence over the account phone.
	if details.ContactPhone == "" || details.EmployerName == "" {
		var owner models.User
		if err := h.Users.FindOne(ctx, bson.M{"_id": listing.OwnerID, "isDeleted": bson.M{"$ne": true}},
			options.FindOne().SetProjection(bson.M{"phone": 1, "firstName": 1, "lastName": 1})).Decode(&owner); err == nil {
			if details.ContactPhone == "" {
				details.ContactPhone = strings.TrimSpace(owner.Phone)
			}
			if details.EmployerName == "" {
				details.EmployerName = strings.TrimSpace(owner.FirstName + " " + owner.LastName)
			}
		}
	}
	h.Notify.PushAcceptedJob(ctx, app.WorkerID, app.ID, listing.Title, details)
}
