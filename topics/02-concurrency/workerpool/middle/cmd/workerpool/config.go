package main

// Config задаёт параметры пула. Валидируется в New.
type Config struct {
	Workers   int
	QueueSize int
	ErrBuf    int
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
}
