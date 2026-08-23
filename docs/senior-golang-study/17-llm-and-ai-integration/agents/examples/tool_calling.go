// Package agentloop — минимальная эталонная реализация протокола вызова
// инструментов и агентного цикла для учебных заметок.
//
// Типы ниже — упрощённая форма, общая для основных провайдеров. В реальных
// запросах имена полей отличаются, но протокол одинаковый.
package agentloop

import (
	"context"
	"encoding/json"
	"fmt"
)

// StopReason — причина, по которой модель прекратила генерацию.
type StopReason string

const (
	// StopEnd — модель дала финальный ответ.
	StopEnd StopReason = "end"
	// StopToolUse — модель просит сервис выполнить один или несколько вызовов.
	StopToolUse StopReason = "tool_use"
	// StopMaxTokens — упёрлись в лимит вывода, ответ оборван на середине.
	StopMaxTokens StopReason = "max_tokens"
)

// Tool — то, что сервис объявляет модели: имя, описание, по которому модель
// решает, вызывать ли функцию, и JSON-схема аргументов.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ToolCall — заявка модели на выполнение инструмента. Поле ID связывает заявку
// с результатом: без него параллельные вызовы не сопоставить между собой.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult — то, что сервис отправляет обратно. Флаг IsError помечает
// неудачное выполнение: модель всё равно должна его увидеть, иначе она будет
// ждать результат, которого не будет.
type ToolResult struct {
	CallID  string `json:"tool_use_id"`
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`
}

// Message — одна запись истории диалога. Осмысленно ровно одно из полей
// с содержимым, в зависимости от роли.
type Message struct {
	Role        string       `json:"role"` // "user", "assistant", "system"
	Text        string       `json:"text,omitempty"`
	ToolCalls   []ToolCall   `json:"tool_calls,omitempty"`   // при роли "assistant"
	ToolResults []ToolResult `json:"tool_results,omitempty"` // при роли "user"
}

// Usage — расход токенов, приходящий с каждым ответом. Это единственный способ
// объяснить задним числом, откуда взялись стоимость и время ответа.
type Usage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	ReasoningTokens int `json:"reasoning_tokens"`
	CachedTokens    int `json:"cached_tokens"`
}

// Response — то, что возвращает провайдер.
type Response struct {
	Text       string
	ToolCalls  []ToolCall
	StopReason StopReason
	Usage      Usage
}

// Model — транспорт до провайдера. Реальная реализация отправляет JSON по HTTP.
type Model interface {
	Complete(ctx context.Context, history []Message, tools []Tool) (*Response, error)
}

// ToolFunc выполняет один инструмент. Возврат ошибки — штатный ход событий:
// её текст уходит модели, чтобы та исправила аргументы и повторила вызов.
type ToolFunc func(ctx context.Context, args json.RawMessage) (string, error)

// Registry сопоставляет объявленные имена инструментов с их реализациями.
type Registry map[string]ToolFunc

// execute выполняет одну заявку и всегда возвращает результат — в том числе
// для неизвестного инструмента и для ошибки. Выброшенный результат оставляет
// модель в ожидании.
func (r Registry) execute(ctx context.Context, call ToolCall) ToolResult {
	fn, ok := r[call.Name]
	if !ok {
		return ToolResult{
			CallID:  call.ID,
			Content: fmt.Sprintf("unknown tool %q", call.Name),
			IsError: true,
		}
	}

	out, err := fn(ctx, call.Arguments)
	if err != nil {
		// Текст должен быть таким, чтобы по нему можно было исправиться:
		// модель читает его и корректирует аргументы.
		return ToolResult{CallID: call.ID, Content: err.Error(), IsError: true}
	}
	return ToolResult{CallID: call.ID, Content: out}
}

// executeAll выполняет все заявки и собирает результаты в одно сообщение.
// Разбиение результатов по нескольким сообщениям ломает их соответствие
// заявкам и приучает модель не запрашивать параллельные вызовы.
func (r Registry) executeAll(ctx context.Context, calls []ToolCall) Message {
	results := make([]ToolResult, 0, len(calls))
	for _, call := range calls {
		results = append(results, r.execute(ctx, call))
	}
	return Message{Role: "user", ToolResults: results}
}

// RoundTrip выполняет один полный обмен: объявить, получить заявку, выполнить,
// вернуть результат. Это весь протокол без цикла.
func RoundTrip(ctx context.Context, m Model, tools []Tool, reg Registry, prompt string) (string, error) {
	history := []Message{{Role: "user", Text: prompt}}

	// Шаги 1–2: отправляем запрос вместе с объявлениями инструментов. Модель
	// может ответить сразу, а может попросить вызов.
	resp, err := m.Complete(ctx, history, tools)
	if err != nil {
		return "", fmt.Errorf("first call: %w", err)
	}
	if resp.StopReason != StopToolUse {
		return resp.Text, nil
	}

	// Сообщение ассистента с заявками обязано остаться в истории. Если оставить
	// только результаты, модель не сможет понять, к чему они относятся.
	history = append(history, Message{Role: "assistant", ToolCalls: resp.ToolCalls})

	// Шаги 3–4: выполняем вызовы и отдаём результаты обратно модели.
	history = append(history, reg.executeAll(ctx, resp.ToolCalls))

	final, err := m.Complete(ctx, history, tools)
	if err != nil {
		return "", fmt.Errorf("second call: %w", err)
	}
	return final.Text, nil
}
