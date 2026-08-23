package agentloop

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// Limits — условия остановки цикла. Без них цикл способен крутиться, пока не
// кончится бюджет: модель может бесконечно повторять один и тот же вызов,
// если не понимает возвращаемую ошибку.
type Limits struct {
	// MaxIterations ограничивает число раз, которое модель может просить вызовы.
	MaxIterations int
	// MaxOutputTokens ограничивает суммарный расход на выход за весь прогон.
	MaxOutputTokens int
}

// DefaultLimits — осторожные значения, пригодные для запроса, который ждёт
// пользователь.
func DefaultLimits() Limits {
	return Limits{MaxIterations: 10, MaxOutputTokens: 50_000}
}

// ErrLimitReached возвращается вместе с частичным ответом, когда цикл остановлен
// ограничителем. Частичный результат полезнее для вызывающего, чем голая ошибка.
var ErrLimitReached = errors.New("agent limit reached")

// Result — то, что произвёл цикл, вместе с расходом за весь прогон.
type Result struct {
	Text       string
	Iterations int
	Usage      Usage
}

// Run — агентный цикл. Всё остальное, что называют агентом, это управление
// контекстом, наблюдаемость и подтверждения вокруг этих полутора десятков строк.
func Run(ctx context.Context, m Model, tools []Tool, reg Registry, prompt string, lim Limits) (Result, error) {
	history := []Message{{Role: "user", Text: prompt}}
	res := Result{}

	for res.Iterations = 1; res.Iterations <= lim.MaxIterations; res.Iterations++ {
		// Вся история уходит по сети заново на каждой итерации — отсюда и берётся
		// рост суммарной стоимости быстрее числа шагов.
		resp, err := m.Complete(ctx, history, tools)
		if err != nil {
			return res, fmt.Errorf("iteration %d: %w", res.Iterations, err)
		}

		res.Usage.InputTokens += resp.Usage.InputTokens
		res.Usage.OutputTokens += resp.Usage.OutputTokens
		res.Usage.ReasoningTokens += resp.Usage.ReasoningTokens
		res.Usage.CachedTokens += resp.Usage.CachedTokens

		// Лог на каждую итерацию: без него два прогона на одном входе неразличимы,
		// потому что идут разными путями.
		slog.InfoContext(ctx, "agent iteration",
			slog.Int("iteration", res.Iterations),
			slog.String("stop_reason", string(resp.StopReason)),
			slog.Int("tool_calls", len(resp.ToolCalls)),
			slog.Int("output_tokens", resp.Usage.OutputTokens),
			slog.Int("reasoning_tokens", resp.Usage.ReasoningTokens),
		)

		if resp.StopReason != StopToolUse {
			res.Text = resp.Text
			if resp.StopReason == StopMaxTokens {
				// HTTP-статус успешный, ответ оборван. Без проверки причины
				// остановки это молчаливая потеря данных.
				return res, fmt.Errorf("%w: output truncated", ErrLimitReached)
			}
			return res, nil
		}

		history = append(history, Message{Role: "assistant", ToolCalls: resp.ToolCalls})
		history = append(history, reg.executeAll(ctx, resp.ToolCalls))

		if res.Usage.OutputTokens > lim.MaxOutputTokens {
			res.Text = resp.Text
			return res, fmt.Errorf("%w: token budget", ErrLimitReached)
		}
	}

	return res, fmt.Errorf("%w: max iterations", ErrLimitReached)
}
