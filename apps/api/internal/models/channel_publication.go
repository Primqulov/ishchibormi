package models

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

// Embedded in the listing so creation and enqueueing commit atomically.
type ChannelPublication struct {
	Reference     string             `bson:"reference"`
	BotUsername   string             `bson:"botUsername"`
	Status        string             `bson:"status"` // pending|sending|sent|skipped|failed|uncertain
	Attempts      int                `bson:"attempts"`
	NextAttemptAt time.Time          `bson:"nextAttemptAt,omitempty"`
	LeaseID       primitive.ObjectID `bson:"leaseId,omitempty"`
	LeaseUntil    time.Time          `bson:"leaseUntil,omitempty"`
	ChatID        int64              `bson:"chatId,omitempty"`
	MessageID     int64              `bson:"messageId,omitempty"`
	Reason        string             `bson:"reason,omitempty"`
	FinishedAt    time.Time          `bson:"finishedAt,omitempty"`
}
