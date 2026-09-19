package notification

import (
	"context"
	"errors"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// PushOnce persists one notification for a durable business event. A stable
// notification ID prevents worker retries from creating duplicate inbox rows.
// The existing pusher queues WS/FCM delivery; acceptance is not a delivery receipt.
func (s *Service) PushOnce(ctx context.Context, n models.Notification) error {
	if s == nil || s.Col == nil {
		return errors.New("notifications unavailable")
	}
	if n.ID.IsZero() {
		return errors.New("notification id is required")
	}
	if httpx.IsReviewActor(ctx) && !s.recipientIsReviewAccount(ctx, n.UserID) {
		return nil
	}
	_, err := s.Col.UpdateOne(ctx, bson.M{"_id": n.ID}, bson.M{"$setOnInsert": s.document(n)}, options.Update().SetUpsert(true))
	if err != nil {
		return err
	}
	if s.Telegram != nil {
		s.Telegram.wake()
	}
	if s.Pusher != nil {
		var state struct {
			PushQueued bool `bson:"pushQueued"`
		}
		if err := s.Col.FindOne(ctx, bson.M{"_id": n.ID}).Decode(&state); err != nil {
			return err
		}
		if !state.PushQueued {
			// At-least-once transport, with a stable event ID. A restart after
			// inbox insertion must not silently lose its push notification.
			s.Pusher.PushUser(n.UserID, "notification", n)
			_, err = s.Col.UpdateOne(ctx, bson.M{"_id": n.ID}, bson.M{"$set": bson.M{"pushQueued": true}})
			if err != nil {
				return err
			}
		}
	}
	return nil
}
