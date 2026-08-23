package tgbot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Action
	}{
		{"429 ждём названное сервером время", &APIError{Code: 429, RetryAfter: 5 * time.Second}, ActionWait},
		{"403 адресат недоступен навсегда", &APIError{Code: 403, Description: "bot was blocked by the user"}, ActionDisable},
		{"401 чинить конфигурацию", &APIError{Code: 401}, ActionFail},
		{"400 повторять бессмысленно", &APIError{Code: 400, Description: "chat not found"}, ActionFail},
		{"400 с migrate_to_chat_id — миграция", &APIError{Code: 400, MigrateTo: -1001}, ActionMigrate},
		{"500 временный отказ", &APIError{Code: 500}, ActionRetry},
		{"незнакомый код — ограниченные повторы", &APIError{Code: 418}, ActionRetry},
		{"сетевая ошибка — исход неизвестен", errors.New("connection reset"), ActionRetry},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.err); got != tt.want {
				t.Fatalf("Classify() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQueuePrefersInteractive(t *testing.T) {
	ctx := context.Background()
	q := NewQueue(4, 4)

	for i := range 3 {
		if err := q.Put(ctx, Outgoing{ChatID: int64(i), Priority: Background}); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.Put(ctx, Outgoing{ChatID: 100, Priority: Interactive}); err != nil {
		t.Fatal(err)
	}

	// Интерактивное сообщение поставлено последним, но выходит первым.
	m, ok := q.Next(ctx)
	if !ok || m.ChatID != 100 {
		t.Fatalf("Next() = %+v, %v; want chat 100", m, ok)
	}
}

func TestPollerAdvancesOffset(t *testing.T) {
	var (
		mu      sync.Mutex
		offsets []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		mu.Lock()
		offsets = append(offsets, r.FormValue("offset"))
		n := len(offsets)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			// Пачка намеренно не по порядку: смещение считается по максимуму.
			fmt.Fprint(w, `{"ok":true,"result":[{"update_id":103},{"update_id":105},{"update_id":104}]}`)
			return
		}
		// Дальше апдейтов нет: имитация длинного опроса без событий.
		<-r.Context().Done()
	}))
	defer srv.Close()

	old := APIBase
	APIBase = srv.URL
	defer func() { APIBase = old }()

	p := NewPoller("test-token")
	p.Timeout = 0
	p.Backoff = time.Millisecond
	p.Client = srv.Client()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		seenMu sync.Mutex
		seen   int
	)
	done := make(chan error, 1)
	go func() {
		done <- p.Run(ctx, func(_ context.Context, updates []Update) error {
			seenMu.Lock()
			seen += len(updates)
			seenMu.Unlock()
			return nil
		})
	}()

	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(offsets) >= 2
	})
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("poller did not stop on context cancellation")
	}

	seenMu.Lock()
	defer seenMu.Unlock()
	if seen != 3 {
		t.Fatalf("handled %d updates, want 3", seen)
	}

	mu.Lock()
	defer mu.Unlock()
	if offsets[0] != "0" {
		t.Fatalf("first offset = %q, want 0", offsets[0])
	}
	// Смещение — максимальный полученный идентификатор плюс один,
	// независимо от порядка апдейтов в пачке.
	if offsets[1] != "106" {
		t.Fatalf("second offset = %q, want 106", offsets[1])
	}
}

// waitFor ждёт выполнения условия, чтобы тест не зависел от таймингов сети.
func waitFor(t *testing.T, limit time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}
