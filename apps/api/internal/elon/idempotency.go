package elon

import (
	"crypto/sha256"
	"net/http"

	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Namespace the client-generated key by its authenticated owner. The Mongo
// _id unique index makes even concurrent retries create at most one listing.
func creationID(r *http.Request, owner primitive.ObjectID) (primitive.ObjectID, bool, error) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		return primitive.NewObjectID(), false, nil
	}
	if _, err := primitive.ObjectIDFromHex(key); err != nil {
		return primitive.NilObjectID, false, httpx.NewError(400, "bad_idempotency_key", "invalid Idempotency-Key")
	}
	hash := sha256.Sum256([]byte("elon-create:" + owner.Hex() + ":" + key))
	var id primitive.ObjectID
	copy(id[:], hash[:12])
	return id, true, nil
}
