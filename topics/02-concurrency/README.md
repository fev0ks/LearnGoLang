# Concurrency

Здесь собраны примеры по конкурентности и координации goroutine.

Основные каталоги:
- `topics/02-concurrency/examples/channels`
- `topics/02-concurrency/examples/goroutines`
- `topics/02-concurrency/examples/multithreading`
- `topics/02-concurrency/code_examples/async_counter`
- `topics/02-concurrency/code_examples/chan_patterns`
- `topics/02-concurrency/code_examples/chans`
- `topics/02-concurrency/code_examples/goleak`
- `topics/02-concurrency/code_examples/lock_free_stack`
- `topics/02-concurrency/code_examples/process_parallel`
- `topics/02-concurrency/code_examples/singleflight`
- `topics/02-concurrency/code_examples/rate_limiter`
- `topics/02-concurrency/examples/rate_limiter_interview`
- `topics/02-concurrency/examples/timer_after_interview`

Что здесь изучать:
- каналы, закрытие каналов, `nil channel`;
- `WaitGroup`, fan-in, tee, bounded concurrency;
- atomic и race-like сценарии;
- утечки goroutine;
- rate limiting и координация параллельной работы.
