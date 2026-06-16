package main

// OnFullPolicy задаёт поведение Submit при полной очереди.
//
// Zero-value (OnFullBlock) сохраняет поведение middle-версии: Submit
// блокируется до освобождения слота, ctx-отмены или Shutdown.
type OnFullPolicy int

const (
	// OnFullBlock - ждать слот в очереди (дефолт; поведение middle).
	OnFullBlock OnFullPolicy = iota
	// OnFullFailFast - вернуть ErrQueueFull сразу, без ожидания.
	OnFullFailFast
	// OnFullDropNewest - молча дропнуть задачу + инкрементить DroppedTasks.
	OnFullDropNewest
)

// Config задаёт параметры пула. Валидируется в New.
type Config struct {
	Workers   int
	QueueSize int
	ErrBuf    int
	OnFull    OnFullPolicy
}

func (c Config) validate() {
	if c.Workers <= 0 {
		panic("workerpool: Workers должен быть > 0")
	}
	if c.QueueSize < 0 {
		panic("workerpool: QueueSize должен быть >= 0")
	}
	if c.ErrBuf < 0 {
		panic("workerpool: ErrBuf должен быть >= 0")
	}
	if c.OnFull < OnFullBlock || c.OnFull > OnFullDropNewest {
		panic("workerpool: неизвестная OnFullPolicy")
	}
}
