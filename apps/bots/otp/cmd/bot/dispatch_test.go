package main

import (
	"context"
	"sync"
	"testing"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestDispatchPreservesChatOrderWithoutBlockingOtherUsers(t *testing.T) {
	updates := make(chan tg.Update, 4)
	for i, id := range []int64{42, 42, 43} {
		updates <- tg.Update{UpdateID: i + 1, Message: &tg.Message{From: &tg.User{ID: id}, Chat: &tg.Chat{ID: id, Type: "private"}}}
	}
	close(updates)
	blocked := make(chan struct{})
	other := make(chan struct{})
	done := make(chan struct{})
	var mu sync.Mutex
	var order []int
	go func() {
		dispatchUpdates(context.Background(), updates, func(q <-chan tg.Update) {
			for u := range q {
				if u.UpdateID == 1 {
					<-blocked
				}
				if u.Message.Chat.ID == 43 {
					close(other)
				}
				mu.Lock()
				order = append(order, u.UpdateID)
				mu.Unlock()
			}
		}, nil)
		close(done)
	}()
	select {
	case <-other:
	case <-time.After(2 * time.Second):
		close(blocked)
		t.Fatal("slow posting blocked another user's login")
	}
	close(blocked)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("workers did not drain")
	}
	mu.Lock()
	defer mu.Unlock()
	first, second := -1, -1
	for i, id := range order {
		if id == 1 {
			first = i
		}
		if id == 2 {
			second = i
		}
	}
	if first < 0 || second < first {
		t.Fatal("chat update order changed")
	}
}

// Kanal a'zoligi update'i shaxsiy chat navbatlariga tushmaydi (privateSender
// uni 0 deb hisoblaydi), shuning uchun u alohida yo'naltirilishi kerak —
// aks holda botni kanalga qo'shish hodisasi jimgina yo'qolardi.
func TestChannelMembershipUpdatesBypassPrivateQueues(t *testing.T) {
	updates := make(chan tg.Update, 2)
	seen := make(chan int64, 1)
	worker := make(chan int, 1)
	updates <- tg.Update{UpdateID: 1, MyChatMember: &tg.ChatMemberUpdated{Chat: tg.Chat{ID: -1001234567890, Type: "channel"}}}
	updates <- tg.Update{UpdateID: 2, Message: &tg.Message{Chat: &tg.Chat{ID: 42, Type: "private"}, From: &tg.User{ID: 42}}}
	close(updates)

	done := make(chan struct{})
	go func() {
		dispatchUpdates(context.Background(), updates, func(q <-chan tg.Update) {
			for u := range q {
				worker <- u.UpdateID
			}
		}, func(_ context.Context, u tg.Update) {
			seen <- u.MyChatMember.Chat.ID
		})
		close(done)
	}()
	select {
	case id := <-seen:
		if id != -1001234567890 {
			t.Fatalf("wrong channel routed: %d", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel membership update never reached its handler")
	}
	select {
	case id := <-worker:
		if id != 2 {
			t.Fatalf("membership update leaked into a private queue: %d", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("private update was not dispatched")
	}
	<-done
}
