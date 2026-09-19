package main

import (
	"context"
	"sync"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bound concurrency while preserving per-chat order. A photo upload or
// moderation request must not pause every user's OTP/login conversation.
func dispatchUpdates(ctx context.Context, updates <-chan tg.Update, run func(<-chan tg.Update), membership func(context.Context, tg.Update)) {
	const workers = 16
	queues := make([]chan tg.Update, workers)
	var wg sync.WaitGroup
	for i := range queues {
		queues[i] = make(chan tg.Update, 32)
		wg.Add(1)
		go func(q <-chan tg.Update) { defer wg.Done(); run(q) }(queues[i])
	}
	defer func() {
		for _, q := range queues {
			close(q)
		}
		wg.Wait()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case u, ok := <-updates:
			if !ok {
				return
			}
			// Kanalga qo'shilish/chiqarilish ATAYLAB navbatlardan tashqarida
			// ishlanadi: privateSender bunday update uchun 0 qaytaradi va u
			// quyida jimgina tashlab yuborilardi. Hodisa kamdan-kam, ya'ni
			// dispatch tsiklini sezilarli ushlab turmaydi.
			if u.MyChatMember != nil {
				if membership != nil {
					membership(ctx, u)
				}
				continue
			}
			id := privateSender(u)
			if id <= 0 {
				continue
			}
			select {
			case queues[id%workers] <- u:
			case <-ctx.Done():
				return
			}
		}
	}
}

func privateSender(u tg.Update) int64 {
	m, from := u.Message, (*tg.User)(nil)
	if u.CallbackQuery != nil {
		m, from = u.CallbackQuery.Message, u.CallbackQuery.From
	} else if m != nil {
		from = m.From
	}
	if m == nil || m.Chat == nil || from == nil || m.Chat.Type != "private" || m.Chat.ID != from.ID {
		return 0
	}
	return from.ID
}
