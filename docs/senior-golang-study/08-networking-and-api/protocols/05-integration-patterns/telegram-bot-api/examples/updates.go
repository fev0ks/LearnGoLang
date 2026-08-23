// Package tgbot contains reference implementations for the Telegram Bot API
// study notes: long polling with correct offset handling and a rate-limited
// sender with priorities.
//
// Код учебный: разобраны только те поля апдейтов, которые нужны для механики
// приёма и отправки. Реальный бот описывает типы полнее.
package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// APIBase — базовый адрес Bot API. Выносится в переменную, чтобы тесты
// подставляли адрес локального сервера.
var APIBase = "https://api.telegram.org"

// Update — входящее событие. UpdateID уникален у бота и служит ключом
// идемпотентности при дедупликации повторных доставок.
type Update struct {
	UpdateID      int              `json:"update_id"`
	Message       *Message         `json:"message,omitempty"`
	EditedMessage *Message         `json:"edited_message,omitempty"`
	CallbackQuery *json.RawMessage `json:"callback_query,omitempty"`
}

// Message — сообщение в чате, урезанное до полей, нужных примерам.
type Message struct {
	MessageID    int    `json:"message_id"`
	Text         string `json:"text,omitempty"`
	MediaGroupID string `json:"media_group_id,omitempty"`
	Chat         struct {
		ID int64 `json:"id"`
	} `json:"chat"`
	From *struct {
		ID int64 `json:"id"`
	} `json:"from,omitempty"`
}

// response — общая форма ответа Bot API. Признак успеха — только поле OK;
// error_code в документации помечен как способный измениться, поэтому
// классификация ошибок должна переживать незнакомый код.
type response struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	Parameters  *struct {
		RetryAfter      int   `json:"retry_after"`
		MigrateToChatID int64 `json:"migrate_to_chat_id"`
	} `json:"parameters"`
}

// Poller получает апдейты длинным опросом.
//
// Ограничения: активный получатель у токена может быть только один —
// второй параллельный getUpdates и установленный webhook дают ошибку 409.
type Poller struct {
	Token string
	// Timeout — сколько Telegram держит запрос при отсутствии апдейтов.
	Timeout time.Duration
	// Limit — размер пачки, максимум 100.
	Limit int
	// AllowedUpdates фильтрует типы апдейтов на стороне Telegram.
	AllowedUpdates []string
	// Backoff — пауза после ошибки: смещение при этом не двигается.
	Backoff time.Duration
	Client  *http.Client
}

// NewPoller собирает Poller с согласованными таймаутами: таймаут клиента
// заведомо больше таймаута опроса, иначе клиент рвёт соединение сам.
func NewPoller(token string) *Poller {
	const pollTimeout = 30 * time.Second
	return &Poller{
		Token:   token,
		Timeout: pollTimeout,
		Limit:   100,
		Backoff: 3 * time.Second,
		Client:  &http.Client{Timeout: pollTimeout + 10*time.Second},
	}
}

// Run опрашивает Telegram, пока жив контекст.
//
// Порядок важен: смещение сдвигается только после того, как handle вернул nil,
// то есть апдейты сохранены. Апдейт считается подтверждённым, как только
// getUpdates вызван со смещением больше его UpdateID, и вернуть его после
// этого нельзя.
func (p *Poller) Run(ctx context.Context, handle func(context.Context, []Update) error) error {
	offset := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		updates, err := p.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !sleep(ctx, p.Backoff) {
				return ctx.Err()
			}
			continue
		}
		if len(updates) == 0 {
			continue
		}
		if err := handle(ctx, updates); err != nil {
			// Смещение не двигаем: те же апдейты придут снова.
			if !sleep(ctx, p.Backoff) {
				return ctx.Err()
			}
			continue
		}
		// Смещение — максимальный полученный идентификатор плюс один.
		offset = maxUpdateID(updates) + 1
	}
}

func (p *Poller) getUpdates(ctx context.Context, offset int) ([]Update, error) {
	form := url.Values{}
	form.Set("offset", strconv.Itoa(offset))
	form.Set("limit", strconv.Itoa(p.Limit))
	form.Set("timeout", strconv.Itoa(int(p.Timeout.Seconds())))
	if len(p.AllowedUpdates) > 0 {
		allowed, err := json.Marshal(p.AllowedUpdates)
		if err != nil {
			return nil, fmt.Errorf("marshal allowed_updates: %w", err)
		}
		form.Set("allowed_updates", string(allowed))
	}

	endpoint := fmt.Sprintf("%s/bot%s/getUpdates", APIBase, p.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.Client.Do(req)
	if err != nil {
		// В адресе есть токен: наружу он попасть не должен.
		return nil, fmt.Errorf("get updates: %w", redactToken(err, p.Token))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if !r.OK {
		return nil, apiErrorFrom(r)
	}

	var updates []Update
	if err := json.Unmarshal(r.Result, &updates); err != nil {
		return nil, fmt.Errorf("decode updates: %w", err)
	}
	return updates, nil
}

// maxUpdateID не полагается на порядок: при webhook апдейты приходят
// вперемешку, и та же осторожность дешевле для общего кода.
func maxUpdateID(updates []Update) int {
	max := updates[0].UpdateID
	for _, u := range updates[1:] {
		if u.UpdateID > max {
			max = u.UpdateID
		}
	}
	return max
}

// sleep ждёт d или завершения контекста. Возвращает false, если контекст истёк.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// redactToken убирает токен из текста ошибки: он входит в адреса Bot API
// и в ссылку на скачивание файла.
func redactToken(err error, token string) error {
	if token == "" || err == nil {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), token, "<token>"))
}
