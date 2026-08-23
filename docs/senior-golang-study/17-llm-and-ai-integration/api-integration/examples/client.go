// Package llmclient показывает минимальную форму HTTP-клиента к языковой модели:
// сборку запроса структурой, разбор блочного ответа, проверку причины остановки
// и логирование расхода токенов.
//
// Имена полей соответствуют упрощённой форме, общей для основных провайдеров;
// в реальных запросах они отличаются. Важна структура, а не конкретный JSON.
package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Request собирается структурой и сериализуется — а не подстановкой текста
// в JSON-шаблон, что ломается на первой же кавычке или переводе строки.
type Request struct {
	Model string `json:"model"`
	// System — правила самого сервиса. Всё, что пришло извне, должно попадать
	// в сообщение пользователя, иначе внешний текст получает полномочия инструкции.
	System    string        `json:"system,omitempty"`
	Messages  []Message     `json:"messages"`
	MaxTokens int           `json:"max_tokens"`
	Format    *OutputSchema `json:"response_format,omitempty"`
	// Effort задаёт глубину рассуждений. Незаполненное поле означает значение
	// провайдера по умолчанию, а оно не самое дешёвое.
	Effort string `json:"effort,omitempty"`
}

// Message — одна запись истории. Вся история отправляется в каждом запросе:
// на стороне провайдера между вызовами ничего не хранится.
type Message struct {
	Role string `json:"role"`
	Text string `json:"content"`
}

// OutputSchema привязывает ответ к JSON-схеме. В строгом режиме схема ограничивает
// саму генерацию, поэтому форму ответа нарушить нельзя.
type OutputSchema struct {
	Type   string          `json:"type"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

// Block — один фрагмент ответа. Содержимое приходит списком, а не строкой:
// первым блоком может оказаться рассуждение или вызов инструмента, а при отказе
// блоков не будет вовсе.
type Block struct {
	Type string `json:"type"` // "text", "reasoning", "tool_use"
	Text string `json:"text,omitempty"`
}

// Usage приходит с каждым ответом и является единственным честным источником
// данных о стоимости и о том, куда ушло время.
type Usage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	ReasoningTokens int `json:"reasoning_tokens"`
	CachedTokens    int `json:"cached_tokens"`
}

// Response повторяет форму ответа провайдера.
type Response struct {
	Model      string  `json:"model"`
	Content    []Block `json:"content"`
	StopReason string  `json:"stop_reason"`
	Usage      Usage   `json:"usage"`
}

// ErrTruncated означает, что ответ упёрся в лимит вывода. Приходит с успешным
// HTTP-статусом, поэтому без проверки причины остановки остаётся незамеченным.
var ErrTruncated = errors.New("response truncated by output limit")

// ErrNoText означает, что текстового блока в ответе нет: отказ провайдера либо
// ответ, состоящий только из вызовов инструментов.
var ErrNoText = errors.New("no text block in response")

// Client — тонкая обёртка над net/http. Ради одного эндпоинта тяжёлая
// библиотека провайдера не нужна.
type Client struct {
	Endpoint string
	APIKey   string
	// HTTP держит собственный таймаут: вызов модели не укладывается в общий
	// таймаут сервиса.
	HTTP *http.Client
}

// New собирает клиент с таймаутом, рассчитанным на долгую генерацию.
func New(endpoint, apiKey string, timeout time.Duration) *Client {
	return &Client{
		Endpoint: endpoint,
		APIKey:   apiKey,
		HTTP:     &http.Client{Timeout: timeout},
	}
}

// Complete отправляет один запрос и возвращает первый текстовый блок.
func (c *Client) Complete(ctx context.Context, req Request) (string, Usage, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", Usage{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	httpResp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return "", Usage{}, fmt.Errorf("send request: %w", err)
	}
	defer httpResp.Body.Close()

	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", Usage{}, fmt.Errorf("read response: %w", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return "", Usage{}, fmt.Errorf("provider returned %d: %s", httpResp.StatusCode, truncate(raw, 1024))
	}

	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", Usage{}, fmt.Errorf("decode response: %w", err)
	}

	// Одной длительности недостаточно, чтобы объяснить медленный вызов. Токены
	// рассуждений не видны в теле ответа, а время обычно уходит именно на них.
	slog.InfoContext(ctx, "llm call completed",
		slog.String("model", resp.Model),
		slog.String("stop_reason", resp.StopReason),
		slog.Duration("duration", time.Since(start)),
		slog.Int("input_tokens", resp.Usage.InputTokens),
		slog.Int("output_tokens", resp.Usage.OutputTokens),
		slog.Int("reasoning_tokens", resp.Usage.ReasoningTokens),
		slog.Int("cached_tokens", resp.Usage.CachedTokens),
	)

	text, ok := firstText(resp.Content)
	if resp.StopReason == "max_tokens" {
		return text, resp.Usage, ErrTruncated
	}
	if !ok {
		return "", resp.Usage, ErrNoText
	}
	return text, resp.Usage, nil
}

// firstText ищет блок нужного типа вместо обращения к Content[0], где может
// оказаться рассуждение, вызов инструмента или вообще ничего.
func firstText(blocks []Block) (string, bool) {
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			return b.Text, true
		}
	}
	return "", false
}

// truncate не даёт мегабайтным телам попасть в лог и в текст ошибки.
func truncate(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}
