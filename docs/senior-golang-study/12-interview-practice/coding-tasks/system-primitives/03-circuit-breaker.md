# Задача 3: Circuit Breaker

## Содержание

- [Контракт задачи](#контракт-задачи)
- [Модель состояний](#модель-состояний)
- [Корректный baseline](#корректный-baseline)
- [Что считать ошибкой](#что-считать-ошибкой)
- [Окно статистики](#окно-статистики)
- [Композиция с другими примитивами](#композиция-с-другими-примитивами)
- [Тестирование и метрики](#тестирование-и-метрики)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

Circuit breaker прекращает вызовы заведомо нездоровой зависимости и даёт ей
время восстановиться. Он уменьшает wasted work и время быстрого отказа, но не
лечит зависимость и не заменяет timeout, bulkhead или retry budget.

---

## Контракт задачи

Перед реализацией стоит уточнить:

1. Какие результаты считаются failure, success или игнорируются?
2. Порог задан числом последовательных ошибок или долей ошибок в окне?
3. Сколько probe разрешено в `half-open`?
4. Breaker создаётся на service, host, endpoint или tenant?
5. Что получает caller при rejection и есть ли fallback?

Учебный вариант ниже открывается после N последовательных failures и пропускает
ровно один probe. Для error-rate окна лучше использовать проверенную библиотеку.

---

## Модель состояний

```text
closed --порог ошибок--> open --timeout--> half-open
   ^                                      |       |
   +-------------- success probe --------+       |
                       open <--- failure probe ---+
```

- `closed` — вызовы проходят, результаты влияют на порог;
- `open` — callback не вызывается, caller сразу получает `ErrCircuitOpen`;
- `half-open` — проходит ограниченное число probe, остальные отклоняются.

Переход `open -> half-open` выполняет первый пришедший запрос, а не отдельная
goroutine. Это упрощает lifecycle: breaker не нужно останавливать.

---

## Корректный baseline

Главная конкурентная тонкость — не пропустить несколько probe и не позволить
результату старого вызова изменить уже новое состояние. Для этого каждому
разрешённому вызову выдаётся ticket с поколением состояния.

```go
package circuitbreaker

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "time"
)

var ErrCircuitOpen = errors.New("circuit breaker open")

type State uint8

const (
    StateClosed State = iota
    StateOpen
    StateHalfOpen
)

type Config struct {
    MaxFailures     int
    RecoveryTimeout time.Duration
    IsFailure       func(error) bool
}

type ticket struct {
    state      State
    generation uint64
}

type Breaker struct {
    cfg Config
    now func() time.Time

    mu            sync.Mutex
    state         State
    failures      int
    openedAt      time.Time
    generation    uint64
}

func New(cfg Config) (*Breaker, error) {
    if cfg.MaxFailures < 1 {
        return nil, fmt.Errorf("max failures must be positive")
    }
    if cfg.RecoveryTimeout <= 0 {
        return nil, fmt.Errorf("recovery timeout must be positive")
    }
    if cfg.IsFailure == nil {
        cfg.IsFailure = func(err error) bool { return err != nil }
    }

    return &Breaker{cfg: cfg, now: time.Now}, nil
}

func (b *Breaker) Do(
    ctx context.Context,
    fn func(context.Context) error,
) error {
    if err := ctx.Err(); err != nil {
        return err
    }

    t, err := b.allow()
    if err != nil {
        return err
    }

    completed := false
    defer func() {
        if !completed {
            // Panic продолжит распространяться, но half-open не зависнет.
            b.record(t, true)
        }
    }()

    callErr := fn(ctx)
    failed := b.cfg.IsFailure(callErr)
    completed = true
    b.record(t, failed)
    return callErr
}

func (b *Breaker) allow() (ticket, error) {
    b.mu.Lock()
    defer b.mu.Unlock()

    switch b.state {
    case StateClosed:
        return ticket{state: StateClosed, generation: b.generation}, nil

    case StateOpen:
        if b.now().Sub(b.openedAt) < b.cfg.RecoveryTimeout {
            return ticket{}, ErrCircuitOpen
        }

        b.state = StateHalfOpen
        b.generation++
        return ticket{state: StateHalfOpen, generation: b.generation}, nil

    case StateHalfOpen:
        return ticket{}, ErrCircuitOpen

    default:
        return ticket{}, fmt.Errorf("unknown circuit breaker state")
    }
}

func (b *Breaker) record(t ticket, failed bool) {
    b.mu.Lock()
    defer b.mu.Unlock()

    if t.generation != b.generation || t.state != b.state {
        return
    }

    switch t.state {
    case StateClosed:
        if !failed {
            b.failures = 0
            return
        }

        b.failures++
        if b.failures >= b.cfg.MaxFailures {
            b.state = StateOpen
            b.openedAt = b.now()
            b.generation++
        }

    case StateHalfOpen:
        b.generation++

        if failed {
            b.state = StateOpen
            b.openedAt = b.now()
            return
        }

        b.state = StateClosed
        b.failures = 0
    }
}

func (b *Breaker) State() State {
    b.mu.Lock()
    defer b.mu.Unlock()
    return b.state
}
```

Само состояние `StateHalfOpen` не допускает второй вызов. Если контракт
расширится до нескольких probe, понадобится отдельный bounded counter.

Поздний результат вызова из старого поколения игнорируется. Например, пять
запросов стартовали в `closed`: первые ошибки открыли circuit, а поздний success
не должен вернуть его в `closed`.

---

## Что считать ошибкой

Классификация зависит от протокола и операции:

| Результат | Обычно учитывать как failure? | Почему |
|---|---:|---|
| connect error, timeout downstream | да | зависимость недоступна или перегружена |
| HTTP `500`, `502`, `503`, `504` | обычно да | server-side/transient failure |
| HTTP `404` | обычно нет | состояние конкретного ресурса |
| HTTP `429` | зависит от scope | общий breaker может усугубить per-client limit |
| `context.Canceled` caller-а | обычно нет | зависимость могла быть здорова |
| domain validation error | нет | повтор не восстановит зависимость |

`IsFailure` не должен автоматически считать любой `error` проблемой
downstream. Полезно различать `failure`, `success` и `excluded`, если библиотека
это поддерживает.

Breaker обычно создают на независимую failure domain: например, на пару
`service + operation`, а не один на все внешние сервисы. Слишком мелкий scope
даёт мало данных, слишком крупный отключает здоровые операции вместе с больной.

---

## Окно статистики

Последовательные failures удобны для интервью, но чувствительны к единичным
сериям. Production-вариант часто принимает решение по rolling window:

```text
failure rate = failures / (successes + failures)
```

Порог применяют только после `minimum requests`. Например, при минимуме 20 и
пороге 50% результаты `1 failure / 1 request` ещё ничего не открывают, а
`12 / 20 = 60%` уже открывают.

Важно определить:

- count-based или time-based window;
- когда старые buckets очищаются;
- сбрасывается ли окно после перехода состояния;
- сколько success нужно в `half-open`;
- как учитываются slow calls и excluded outcomes.

Самописное lock-free окно легко получить семантически неверным: атомарные
счётчики не делают атомарными переход состояния, ротацию buckets и сброс окна.
Для production разумнее использовать, например,
[sony/gobreaker](https://github.com/sony/gobreaker) и проверить семантику его
`Interval`, `Timeout`, `MaxRequests` и `ReadyToTrip` для выбранной версии.

---

## Композиция с другими примитивами

Обычный порядок вокруг одного downstream-вызова:

```text
overall deadline -> bulkhead -> circuit breaker -> retry -> attempt timeout
```

Это не универсальная формула, но она подчёркивает ограничения:

- retry должен видеть `ErrCircuitOpen` как terminal outcome;
- каждая попытка имеет timeout, а вся операция — меньший bounded budget;
- bulkhead ограничивает число одновременных slow calls, чего breaker в
  `closed` не делает;
- fallback не должен вызывать ту же сломанную зависимость по другому пути;
- mutation повторяется только при доказанной идемпотентности.

Если retry находится снаружи breaker, каждая попытка попадает в статистику. Если
breaker снаружи retry, он видит итог целой retry-сессии. Выбор меняет скорость
открытия и должен быть явным.

---

## Тестирование и метрики

Clock лучше сделать зависимостью в тесте, установив `b.now` на функцию,
возвращающую управляемое время. Тогда проверяются без `Sleep`:

1. callback не вызывается в `open`;
2. до timeout probe не проходит;
3. после timeout из нескольких goroutine проходит ровно один probe;
4. failed probe возвращает `open` и начинает новый timeout;
5. successful probe закрывает circuit;
6. поздний результат предыдущего поколения не меняет состояние;
7. тест проходит с `go test -race`.

Минимальные метрики:

- текущее состояние как bounded gauge;
- transitions с `from` и `to`;
- rejected calls;
- outcomes по классификации;
- длительность пребывания в `open`.

Не следует добавлять raw URL, user ID или error message в labels: это создаёт
неограниченную cardinality.

---

## Типичные ошибки

- Пропускать все запросы в `half-open`: recovery превращается в новый traffic
  spike.
- Давать старому in-flight результату закрыть уже открытый breaker.
- Открывать circuit по любому `4xx` или domain error.
- Считать, что breaker сам прерывает зависший callback: это делает context и
  timeout транспорта.
- Выбирать timeout из «разумного диапазона» без SLO и recovery characteristics.
- Делить один breaker между независимыми dependencies.
- Делать distributed state breaker-а по умолчанию: локальный breaker на pod
  обычно проще и не создаёт новую shared dependency.

---

## Interview-ready answer

1. **Что гарантирует circuit breaker?**
   - **Fast fail —** в `open` callback вообще не вызывается.
   - **Recovery probe —** после timeout в `half-open` проходит ограниченное
     число проверочных вызовов.
   - **Не timeout —** уже запущенную операцию останавливает её context.

2. **Что самое сложное в конкурентной реализации?**
   - **Single probe —** переход `open -> half-open` должен быть атомарным.
   - **Поколения —** поздние результаты старого состояния нельзя применять к
     новому.
   - **Классификация —** circuit открывают только результаты, говорящие о
     здоровье выбранной dependency.

3. **Что использовать в production?**
   - **Library —** готовый breaker с проверенной state machine и rolling
     window.
   - **Настройка —** thresholds выводят из трафика, SLO и наблюдений.
   - **Композиция —** breaker работает вместе с timeout, retry budget,
     bulkhead и fallback.

---

## Связанные материалы

- [Circuit Breaker](../../../05-system-design/reliability-patterns/03-circuit-breaker.md)
- [Retry с Backoff](./02-retry-with-backoff.md)
- [Bulkhead](../../../05-system-design/reliability-patterns/07-bulkhead.md)
- [sony/gobreaker](https://github.com/sony/gobreaker)
