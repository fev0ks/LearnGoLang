# Задача 10: первый успешный провайдер

## Содержание

- [Формулировка](#формулировка)
- [Исходный код](#исходный-код)
- [Основные проблемы](#основные-проблемы)
- [Исправленное решение](#исправленное-решение)
- [Границы гарантии отмены](#границы-гарантии-отмены)
- [Пример теста](#пример-теста)
- [Что проверить тестами](#что-проверить-тестами)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Задача проверяет паттерн hedged request: одну логическую операцию запускают у
нескольких провайдеров и используют первый успешный ответ. Первый завершившийся
ответ и первый успешный ответ — разные контракты.

---

## Формулировка

Сервис запрашивает курс валют у нескольких провайдеров. Нужно вернуть первый
успешный результат, отменить проигравшие запросы и вернуть объединённую ошибку,
если отказали все. Пустой список провайдеров должен завершаться сразу.

---

## Исходный код

```go
func FirstRate(
    ctx context.Context,
    providers []Provider,
    pair Pair,
) (Rate, error) {
    type outcome struct {
        rate Rate
        err  error
    }

    outcomes := make(chan outcome)
    for _, provider := range providers {
        go func(provider Provider) {
            rate, err := provider.Rate(ctx, pair)
            outcomes <- outcome{rate: rate, err: err}
        }(provider)
    }

    result := <-outcomes
    return result.rate, result.err
}
```

---

## Основные проблемы

| Проблема | Последствие |
| --- | --- |
| Возвращается первый завершившийся outcome | Быстрая ошибка побеждает более медленный успешный ответ |
| После возврата никто не читает `outcomes` | Остальные goroutines блокируются на send в небуферизованный канал |
| Проигравшие запросы не отменяются | Они продолжают занимать соединения и квоту провайдеров |
| Receive не выбирает `ctx.Done()` | Пустой список или зависшие провайдеры блокируют функцию навсегда |
| Ошибки остальных провайдеров теряются | При полном отказе невозможно увидеть все причины |
| Параллелизм равен числу провайдеров | Для большого динамического списка может понадобиться отдельный лимит |

В Go 1.22 и новее переменные цикла `range` создаются заново для каждой итерации.
В примере provider дополнительно передан параметром goroutine, поэтому здесь нет
ошибки захвата переменной цикла и искать её как главный дефект не следует.

---

## Исправленное решение

Канал имеет ёмкость по числу producers. Это позволяет каждой завершившейся
goroutine отправить один outcome, даже если функция уже вернулась. Отмена
уменьшает лишнюю работу, а буфер страхует именно фазу доставки результата.

```go
func FirstRate(
    ctx context.Context,
    providers []Provider,
    pair Pair,
) (Rate, error) {
    if len(providers) == 0 {
        return Rate{}, fmt.Errorf("no rate providers")
    }

    type outcome struct {
        provider string
        rate     Rate
        err      error
    }

    raceCtx, cancel := context.WithCancel(ctx)
    defer cancel()

    outcomes := make(chan outcome, len(providers))
    for _, provider := range providers {
        go func(provider Provider) {
            rate, err := provider.Rate(raceCtx, pair)
            outcomes <- outcome{
                provider: provider.Name(),
                rate:     rate,
                err:      err,
            }
        }(provider)
    }

    errorsByProvider := make([]error, 0, len(providers))
    for range providers {
        select {
        case <-ctx.Done():
            return Rate{}, ctx.Err()
        case result := <-outcomes:
            if result.err == nil {
                cancel()
                return result.rate, nil
            }
            errorsByProvider = append(
                errorsByProvider,
                fmt.Errorf(
                    "%s: %w",
                    result.provider,
                    result.err,
                ),
            )
        }
    }

    return Rate{}, fmt.Errorf(
        "all rate providers failed: %w",
        errors.Join(errorsByProvider...),
    )
}
```

Закрывать `outcomes` здесь не требуется: consumer знает точное максимальное
число сообщений и возвращается раньше после успеха. Канал станет недостижимым
после последних отправок и будет собран сборщиком мусора. Если API использует
`range outcomes`, отдельный coordinator должен закрыть канал после завершения
всех producers.

---

## Границы гарантии отмены

Go не может безопасно остановить произвольную goroutine снаружи. `cancel`
срабатывает только если `Provider.Rate` наблюдает context и передаёт его в
сетевой запрос. Для защиты от зависшего транспорта нужны timeout клиента и
deadline конкретной попытки.

Буферизированный канал предотвращает утечку на отправке outcome, но не лечит
провайдера, который навсегда завис внутри собственного кода. Контракт интерфейса
должен требовать соблюдения context.

Hedged requests снижают хвостовую latency ценой дополнительной нагрузки и
расхода квоты. Часто второй запрос запускают после небольшой задержки, а не
одновременно с первым. Задержка должна прерываться через context.

---

## Пример теста

Первый провайдер сразу возвращает ошибку, второй ждёт разрешения и затем
возвращает успех. Исходная реализация завершит тест быстрой ошибкой, а
исправленная продолжит ждать возможный успешный outcome.

```go
type providerStub struct {
    name string
    call func(context.Context, Pair) (Rate, error)
}

func (provider providerStub) Name() string {
    return provider.name
}

func (provider providerStub) Rate(
    ctx context.Context,
    pair Pair,
) (Rate, error) {
    return provider.call(ctx, pair)
}

func TestFirstRate_FastErrorDoesNotBeatSuccess(test *testing.T) {
    failureStarted := make(chan struct{})
    successStarted := make(chan struct{})
    releaseSuccess := make(chan struct{})
    dependencyErr := errors.New("provider unavailable")
    want := Rate{Value: "42.10"}

    providers := []Provider{
        providerStub{
            name: "failed",
            call: func(context.Context, Pair) (Rate, error) {
                close(failureStarted)
                return Rate{}, dependencyErr
            },
        },
        providerStub{
            name: "successful",
            call: func(
                ctx context.Context,
                _ Pair,
            ) (Rate, error) {
                close(successStarted)
                select {
                case <-releaseSuccess:
                    return want, nil
                case <-ctx.Done():
                    return Rate{}, ctx.Err()
                }
            },
        },
    }

    type rateResult struct {
        rate Rate
        err  error
    }
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    done := make(chan rateResult, 1)
    go func() {
        rate, err := FirstRate(
            ctx,
            providers,
            Pair{Base: "USD", Quote: "EUR"},
        )
        done <- rateResult{rate: rate, err: err}
    }()

    select {
    case <-failureStarted:
    case <-ctx.Done():
        test.Fatal("failed provider did not start")
    }
    select {
    case <-successStarted:
    case <-ctx.Done():
        test.Fatal("successful provider did not start")
    }
    close(releaseSuccess)

    var result rateResult
    select {
    case result = <-done:
    case <-ctx.Done():
        test.Fatal("FirstRate did not return")
    }
    if result.err != nil {
        test.Fatalf("FirstRate: %v", result.err)
    }
    if !reflect.DeepEqual(result.rate, want) {
        test.Fatalf("rate = %#v, want %#v", result.rate, want)
    }
}
```

Сигналы `failureStarted` и `successStarted` гарантируют, что обе попытки начаты.
Ошибка уже готова до `releaseSuccess`, поэтому тест действительно различает
первый завершившийся ответ и первый успех.

---

## Что проверить тестами

- Быстрая ошибка не мешает вернуть более медленный успех.
- Первый успех отменяет оставшиеся провайдеры.
- После раннего успеха все goroutines могут отправить outcome и завершиться.
- Полный отказ возвращает ошибки всех провайдеров с их именами.
- Отмена caller возвращается без ожидания результатов.
- Пустой список сразу возвращает ошибку.
- Порядок провайдеров в слайсе не определяет победителя.

Моки получают каналы `started`, `release` и `canceled`. Так тест точно управляет
порядком завершения и проверяет отмену без пауз по времени.

---

## Interview-ready answer

**1. Чем первый ответ отличается от первого успеха?**

- Первый ответ — может быть ошибкой и преждевременно завершить гонку.
- Первый успех — ошибки накапливаются, пока один провайдер не ответит успешно или
  не закончатся все варианты.

**2. Почему исходные goroutines текут?**

- Consumer — читает только одно сообщение и возвращается.
- Producers — остальные блокируются на send в небуферизованный канал.

**3. Зачем одновременно нужны cancel и буфер?**

- Cancel — просит проигравшие операции прекратить внешнюю работу.
- Буфер — не даёт их финальной отправке зависеть от уже ушедшего consumer.

**4. Можно ли гарантировать остановку проигравших?**

- Условие — только если реализация провайдера соблюдает context.
- Защита — сетевые timeout и deadline ограничивают плохо работающую зависимость.

---

## Связанные материалы

- [Fan-In / Fan-Out](../concurrency/03-fan-in-fan-out.md)
- [HTTP-клиент платёжного сервиса](./04-payment-http-client.md)
- [Context patterns](../../../01-go-core/concurrency-and-performance/04-context-patterns.md)
