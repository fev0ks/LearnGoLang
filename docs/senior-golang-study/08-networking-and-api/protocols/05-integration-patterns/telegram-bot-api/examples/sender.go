package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Пределы отправки взяты из Telegram Bot FAQ: около 30 сообщений в секунду
// на бота и не более одного сообщения в секунду в один чат. Точных чисел
// Telegram не гарантирует, единственный надёжный сигнал о превышении — 429.
const (
	defaultGlobalRate = 30
	defaultChatRate   = time.Second
)

// APIError — ошибка, о которой сообщил сам Bot API (ok = false).
// Сетевые ошибки и таймауты этим типом не описываются: там исход неизвестен.
type APIError struct {
	Code        int
	Description string
	RetryAfter  time.Duration
	MigrateTo   int64
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram api: %d %s", e.Code, e.Description)
}

func apiErrorFrom(r response) *APIError {
	e := &APIError{Code: r.ErrorCode, Description: r.Description}
	if r.Parameters != nil {
		e.RetryAfter = time.Duration(r.Parameters.RetryAfter) * time.Second
		e.MigrateTo = r.Parameters.MigrateToChatID
	}
	return e
}

// Action — что делать с сообщением после неудачной попытки.
type Action int

const (
	// ActionRetry — временный отказ: повторить с нарастающей выдержкой.
	ActionRetry Action = iota
	// ActionWait — 429: подождать названное сервером время.
	ActionWait
	// ActionDisable — 403: адресат недоступен навсегда, отключить его.
	ActionDisable
	// ActionMigrate — чат сменил идентификатор, повторить по новому.
	ActionMigrate
	// ActionFail — ошибка вызова, повторять бессмысленно.
	ActionFail
)

// Classify различает четыре класса отказов. Незнакомый код трактуется как
// временный отказ: ограниченное число повторов безопаснее немедленного отказа.
func Classify(err error) Action {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return ActionRetry
	}
	switch {
	case apiErr.Code == 429:
		return ActionWait
	case apiErr.Code == 403:
		return ActionDisable
	case apiErr.Code == 401:
		return ActionFail
	case apiErr.MigrateTo != 0:
		return ActionMigrate
	case apiErr.Code == 400:
		return ActionFail
	case apiErr.Code >= 500:
		return ActionRetry
	}
	return ActionRetry
}

// Priority разделяет ответы пользователю и фоновые рассылки: у задержки
// в этих двух случаях разная цена.
type Priority int

const (
	// Interactive — ответ на действие пользователя, обгоняет рассылку.
	Interactive Priority = iota
	// Background — рассылка, отчёт, напоминание.
	Background
)

// Outgoing — единица работы отправителя. Входящее сообщение описано
// отдельным типом Message: у них разные поля и разные роли.
type Outgoing struct {
	ChatID   int64
	Text     string
	Priority Priority
	// ValidUntil отбрасывает устаревшее: уведомление о готовности заказа
	// через четыре часа лучше не отправлять вовсе. Нулевое значение — без срока.
	ValidUntil time.Time
}

// Sender — единственная точка отправки. Перед вызовом Bot API сообщение
// проходит оба ограничителя: общий на бота и персональный на чат.
type Sender struct {
	Token  string
	Client *http.Client

	global *rate.Limiter

	mu    sync.Mutex
	chats map[int64]*rate.Limiter
}

func NewSender(token string) *Sender {
	return &Sender{
		Token:  token,
		Client: &http.Client{Timeout: 30 * time.Second},
		global: rate.NewLimiter(rate.Limit(defaultGlobalRate), defaultGlobalRate),
		chats:  make(map[int64]*rate.Limiter),
	}
}

// chatLimiter выдаёт ограничитель конкретного чата. Burst = 1: всплески
// в один чат не накапливаются, иначе после паузы уйдёт пачка сообщений подряд.
func (s *Sender) chatLimiter(chatID int64) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.chats[chatID]
	if !ok {
		l = rate.NewLimiter(rate.Every(defaultChatRate), 1)
		s.chats[chatID] = l
	}
	return l
}

// Send выполняет одну попытку отправки. Ожидание на ограничителях —
// часть попытки: превышать предел дороже, чем ждать своей очереди.
func (s *Sender) Send(ctx context.Context, m Outgoing) error {
	if err := s.global.Wait(ctx); err != nil {
		return err
	}
	if err := s.chatLimiter(m.ChatID).Wait(ctx); err != nil {
		return err
	}

	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(m.ChatID, 10))
	form.Set("text", m.Text)

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", APIBase, s.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.Client.Do(req)
	if err != nil {
		return fmt.Errorf("send message: %w", redactToken(err, s.Token))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if !r.OK {
		return apiErrorFrom(r)
	}
	return nil
}

// Subscribers — то, что отправитель знает о состоянии адресатов.
// Disable вызывается по ошибке 403 и убирает чат из будущих рассылок.
type Subscribers interface {
	Disable(ctx context.Context, chatID int64) error
	Migrate(ctx context.Context, oldID, newID int64) error
}

// Deliver отправляет сообщение с учётом классификации отказов.
//
// Попытка, завершившаяся 429, не расходует лимит попыток: это не отказ,
// а названная сервером задержка. От бесконечного цикла защищает ValidUntil
// и контекст вызывающего.
func (s *Sender) Deliver(ctx context.Context, subs Subscribers, m Outgoing, maxAttempts int) error {
	for attempt := 1; attempt <= maxAttempts; {
		if !m.ValidUntil.IsZero() && time.Now().After(m.ValidUntil) {
			return fmt.Errorf("message to %d expired", m.ChatID)
		}

		err := s.Send(ctx, m)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		switch Classify(err) {
		case ActionWait:
			var apiErr *APIError
			errors.As(err, &apiErr)
			wait := apiErr.RetryAfter
			if wait <= 0 {
				wait = time.Second
			}
			if !sleep(ctx, wait) {
				return ctx.Err()
			}

		case ActionDisable:
			return subs.Disable(ctx, m.ChatID)

		case ActionMigrate:
			var apiErr *APIError
			errors.As(err, &apiErr)
			if err := subs.Migrate(ctx, m.ChatID, apiErr.MigrateTo); err != nil {
				return err
			}
			m.ChatID = apiErr.MigrateTo

		case ActionFail:
			return err

		case ActionRetry:
			backoff := time.Duration(1<<attempt) * time.Second
			if !sleep(ctx, backoff) {
				return ctx.Err()
			}
			attempt++
		}
	}
	return fmt.Errorf("deliver to %d: attempts exhausted", m.ChatID)
}

// Queue — две очереди со строгим приоритетом интерактивных сообщений.
// Фон не голодает, пока интерактивный поток заметно меньше общего предела;
// при равном потоке фону нужна отдельная квота.
type Queue struct {
	interactive chan Outgoing
	background  chan Outgoing
}

func NewQueue(interactiveSize, backgroundSize int) *Queue {
	return &Queue{
		interactive: make(chan Outgoing, interactiveSize),
		background:  make(chan Outgoing, backgroundSize),
	}
}

// Put ставит сообщение в очередь по его приоритету.
func (q *Queue) Put(ctx context.Context, m Outgoing) error {
	ch := q.background
	if m.Priority == Interactive {
		ch = q.interactive
	}
	select {
	case ch <- m:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Next отдаёт следующее сообщение: сначала интерактивное, если оно есть.
// Первый select с default даёт интерактивной очереди право первого выбора,
// второй блокируется на обеих, чтобы отправитель не крутился вхолостую.
func (q *Queue) Next(ctx context.Context) (Outgoing, bool) {
	select {
	case m := <-q.interactive:
		return m, true
	default:
	}
	select {
	case m := <-q.interactive:
		return m, true
	case m := <-q.background:
		return m, true
	case <-ctx.Done():
		return Outgoing{}, false
	}
}
