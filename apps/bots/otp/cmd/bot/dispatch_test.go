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
		})
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
