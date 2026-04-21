# Go implementation: feature flag client

Этот файл — Go-специфичный companion к [02-feature-flags-in-practice.md](./02-feature-flags-in-practice.md). Здесь детали, которые спрашивают на system design интервью для senior Go инженера: как реализовать потокобезопасный клиент, как правильно управлять goroutine, как тестировать.

## Содержание

- [Interface design для testability](#interface-design-для-testability)
- [Thread-safe snapshot: atomic.Value vs RWMutex](#thread-safe-snapshot-atomicvalue-vs-rwmutex)
- [Background refresh goroutine](#background-refresh-goroutine)
- [Graceful shutdown](#graceful-shutdown)
- [Deterministic bucketing с FNV-32a](#deterministic-bucketing-с-fnv-32a)
- [Rules evaluation](#rules-evaluation)
- [Fallback и degraded mode](#fallback-и-degraded-mode)
- [Testing patterns](#testing-patterns)
- [Performance considerations](#performance-considerations)
- [Интеграция с context и tracing](#интеграция-с-context-и-tracing)
- [Interview-ready answer](#interview-ready-answer)

## Interface design для testability

Начинать реализацию нужно с interface, а не с конкретного типа. Handlers зависят от interface — это позволяет подменять реализацию в тестах без сетевых вызовов.

```go
type Client interface {
    Bool(ctx context.Context, key string, subject Subject, fallback bool) bool
    Variant(ctx context.Context, key string, subject Subject, fallback string) string
    Close() error
}

type Subject struct {
    UserID     string
    AccountID  string
    DeviceID   string
    Country    string
    Platform   string
    AppVersion string
}

type Decision struct {
    Variant string
    Reason  string // "allowlist", "percentage_rollout", "default", "flag_not_found"
}
```

`Close()` на интерфейсе — принципиальный момент: он заставляет вызывающий код думать о lifecycle, иначе goroutine leak в тестах и при hot reload.

Handler принимает интерфейс, а не конкретный тип:

```go
type CheckoutHandler struct {
    flags flagsvc.Client
    repo  CheckoutRepository
}
```

## Thread-safe snapshot: atomic.Value vs RWMutex

Проблема: background goroutine пишет новый config; тысячи request goroutine читают его одновременно.

### sync.RWMutex

```go
type client struct {
    mu   sync.RWMutex
    snap configSnapshot
}

func (c *client) Bool(ctx context.Context, key string, sub Subject, fallback bool) bool {
    c.mu.RLock()
    snap := c.snap
    c.mu.RUnlock()
    // ...
}
```

Плюс: позволяет обновлять один flag без замены всего snapshot.
Минус: каждый вызов `Bool` берет RLock — при высоком RPS это measurable contention.

### atomic.Value (предпочтительно для read-heavy)

```go
type configSnapshot struct {
    flags     map[string]flagConfig
    version   string
    loadedAt  time.Time
}

type client struct {
    snapshot atomic.Value // хранит *configSnapshot
    // ...
}

func (c *client) Bool(ctx context.Context, key string, sub Subject, fallback bool) bool {
    snap := c.snapshot.Load().(*configSnapshot) // zero allocation, no lock
    d := evaluate(snap, key, sub)
    if d.Variant == "" {
        return fallback
    }
    return d.Variant == "true" || d.Variant == "treatment"
}
```

Background goroutine строит полностью новый `configSnapshot` и делает один `atomic.Value.Store()`. Читатели делают `Load()` — одна инструкция, без lock contention.

Почему `*configSnapshot`, а не `configSnapshot`:
- `atomic.Value` хранит interface{} внутри, поэтому тип должен быть конкретным;
- pointer позволяет держать immutable map — после `Store` никто не изменяет уже загруженный snapshot.

## Background refresh goroutine

```go
func (c *client) startRefresh(ctx context.Context, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop() // освобождает внутренний timer goroutine

    for {
        select {
        case <-ticker.C:
            if err := c.refresh(ctx); err != nil {
                c.metrics.refreshErrors.Add(1)
                // snapshot не заменяем — продолжаем работать с последним хорошим
                c.logger.Error("flag config refresh failed", "err", err)
            }
        case <-ctx.Done():
            return
        }
    }
}

func (c *client) refresh(ctx context.Context) error {
    raw, err := c.fetcher.Fetch(ctx)
    if err != nil {
        return err
    }
    snap, err := buildSnapshot(raw)
    if err != nil {
        return err
    }
    c.snapshot.Store(snap)
    return nil
}
```

Ключевые моменты:
- `defer ticker.Stop()` обязателен: без него `time.NewTicker` держит внутреннюю goroutine;
- при ошибке refresh — логируем и оставляем последний хороший snapshot (не nil);
- shutdown сигнализируется через `ctx.Done()`, не отдельным channel.

## Graceful shutdown

```go
type client struct {
    snapshot atomic.Value
    cancel   context.CancelFunc
    wg       sync.WaitGroup
    cfg      Config
    fetcher  Fetcher
    logger   *slog.Logger
    metrics  clientMetrics
}

func New(cfg Config, fetcher Fetcher) (*client, error) {
    ctx, cancel := context.WithCancel(context.Background())
    c := &client{
        cancel:  cancel,
        cfg:     cfg,
        fetcher: fetcher,
        logger:  cfg.Logger,
    }

    // первая загрузка синхронная — если упала, клиент не создается
    if err := c.refresh(ctx); err != nil {
        cancel()
        return nil, fmt.Errorf("initial flag config load: %w", err)
    }

    c.wg.Add(1)
    go func() {
        defer c.wg.Done()
        c.startRefresh(ctx, cfg.RefreshInterval)
    }()

    return c, nil
}

func (c *client) Close() error {
    c.cancel()  // сигнализируем goroutine остановиться
    c.wg.Wait() // ждем завершения in-flight refresh
    return nil
}
```

Зачем `wg.Wait()` в `Close()`:
- гарантирует, что in-flight refresh завершится до того, как процесс выйдет;
- без этого можно получить запись в уже закрытый fetcher или частично обновленный snapshot.

Типичная инициализация в `main`:

```go
flagClient, err := flagsvc.New(cfg, fetcher)
if err != nil {
    log.Fatal(err)
}
defer flagClient.Close()
```

## Deterministic bucketing с FNV-32a

Deterministic значит: один и тот же `(flagKey, subjectID)` всегда дает одно и то же число.

Почему FNV-32a:
- стандартная библиотека `hash/fnv`, нет внешних зависимостей;
- чрезвычайно быстро — нет crypto overhead как у MD5/SHA;
- хорошее распределение для коротких строк;
- результат воспроизводим между Go версиями (в отличие от `map` iteration order).

```go
import "hash/fnv"

// bucket возвращает число от 0 до 99 включительно
func bucket(flagKey, subjectID string) uint32 {
    h := fnv.New32a()
    h.Write([]byte(flagKey))
    h.Write([]byte{':'})
    h.Write([]byte(subjectID))
    return h.Sum32() % 100
}

func inRollout(flagKey, subjectID string, percentage uint8) bool {
    if percentage == 0 {
        return false
    }
    if percentage >= 100 {
        return true
    }
    return bucket(flagKey, subjectID) < uint32(percentage)
}
```

Соль `flagKey + ":" + subjectID` принципиальна: без нее пользователь попадал бы в одинаковый bucket для всех flags и всегда оказывался бы в одной и той же группе treatment/control по всем экспериментам сразу.

## Rules evaluation

Модель данных для rule:

```go
type ruleKind string

const (
    RuleDenylist   ruleKind = "denylist"
    RuleAllowlist  ruleKind = "allowlist"
    RuleAttribute  ruleKind = "attribute"
    RulePercentage ruleKind = "percentage"
)

type rule struct {
    Kind       ruleKind
    Attribute  string            // "country", "platform", "app_version"
    Values     []string          // исходный список
    valuesSet  map[string]struct{} // pre-built при загрузке config
    Variant    string
    Percentage uint8
}

type flagConfig struct {
    Key            string
    Rules          []rule
    DefaultVariant string
}
```

Evaluation loop с short-circuit и capture причины решения:

```go
func evaluate(snap *configSnapshot, key string, sub Subject) Decision {
    cfg, ok := snap.flags[key]
    if !ok {
        return Decision{Variant: "", Reason: "flag_not_found"}
    }

    for _, r := range cfg.Rules {
        if d, matched := matchRule(r, key, sub); matched {
            return d
        }
    }

    return Decision{Variant: cfg.DefaultVariant, Reason: "default"}
}

func matchRule(r rule, flagKey string, sub Subject) (Decision, bool) {
    switch r.Kind {
    case RuleDenylist:
        if _, ok := r.valuesSet[sub.UserID]; ok {
            return Decision{Variant: "control", Reason: "denylist"}, true
        }
    case RuleAllowlist:
        if _, ok := r.valuesSet[sub.UserID]; ok {
            return Decision{Variant: r.Variant, Reason: "allowlist"}, true
        }
    case RuleAttribute:
        val := attributeValue(sub, r.Attribute)
        if _, ok := r.valuesSet[val]; ok {
            return Decision{Variant: r.Variant, Reason: "attribute:" + r.Attribute}, true
        }
    case RulePercentage:
        if inRollout(flagKey, sub.UserID, r.Percentage) {
            return Decision{Variant: r.Variant, Reason: "percentage_rollout"}, true
        }
    }
    return Decision{}, false
}
```

`valuesSet` строится один раз при загрузке config, не при каждом вызове. Это устраняет аллокации в hot path для allowlist/denylist с тысячами элементов.

## Fallback и degraded mode

Три уровня деградации:

```go
func (c *client) currentSnapshot() *configSnapshot {
    v := c.snapshot.Load()
    if v == nil {
        return nil // начальная загрузка не прошла
    }
    snap := v.(*configSnapshot)

    age := time.Since(snap.loadedAt)
    if age > c.cfg.StaleThreshold {
        // работаем, но предупреждаем
        c.metrics.staleConfigSeconds.Store(int64(age.Seconds()))
        c.logger.Warn("feature flag config is stale", "age", age)
    }

    return snap
}
```

| Ситуация | Поведение |
|---|---|
| Snapshot свежий (< StaleThreshold) | evaluation работает нормально |
| Snapshot устарел (> StaleThreshold) | evaluation работает, warning log + stale gauge |
| Snapshot отсутствует (nil) | возвращаем caller-provided `fallback` значение, error log |

В `Bool` и `Variant`:

```go
func (c *client) Bool(ctx context.Context, key string, sub Subject, fallback bool) bool {
    snap := c.currentSnapshot()
    if snap == nil {
        c.logger.Error("no feature flag config available, using fallback", "key", key, "fallback", fallback)
        c.metrics.fallbackTotal.Add(1)
        return fallback
    }
    d := evaluate(snap, key, sub)
    if d.Reason == "flag_not_found" {
        return fallback
    }
    return d.Variant == "true" || d.Variant == "treatment"
}
```

## Testing patterns

### Mock-реализация интерфейса

```go
type mockClient struct {
    variants map[string]string
}

func (m *mockClient) Bool(_ context.Context, key string, _ flags.Subject, fallback bool) bool {
    v, ok := m.variants[key]
    if !ok {
        return fallback
    }
    return v == "true" || v == "treatment"
}

func (m *mockClient) Variant(_ context.Context, key string, _ flags.Subject, fallback string) string {
    if v, ok := m.variants[key]; ok {
        return v
    }
    return fallback
}

func (m *mockClient) Close() error { return nil }
```

Использование в handler тесте:

```go
func TestCheckoutUsesV2WhenFlagEnabled(t *testing.T) {
    h := &CheckoutHandler{
        flags: &mockClient{variants: map[string]string{"checkout_v2": "treatment"}},
        repo:  &fakeRepo{},
    }
    resp := httptest.NewRecorder()
    h.ServeHTTP(resp, httptest.NewRequest("POST", "/checkout", nil))
    // assert new checkout behavior
}
```

### Table-driven тесты для rules

```go
func TestEvaluateRules(t *testing.T) {
    snap := buildTestSnapshot()

    cases := []struct {
        name    string
        subject flags.Subject
        want    string
    }{
        {"unknown flag returns empty", flags.Subject{UserID: "u1"}, ""},
        {"denylist blocks treatment", flags.Subject{UserID: "blocked_user"}, "control"},
        {"allowlist grants treatment", flags.Subject{UserID: "beta_user"}, "treatment"},
        {"country match", flags.Subject{UserID: "u2", Country: "GE"}, "treatment"},
        {"country miss", flags.Subject{UserID: "u3", Country: "DE"}, "control"},
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            d := evaluate(snap, "checkout_v2", tc.subject)
            if d.Variant != tc.want {
                t.Errorf("got %q, want %q", d.Variant, tc.want)
            }
        })
    }
}
```

### Bucketing stability

```go
func TestBucketingIsStable(t *testing.T) {
    for i := 0; i < 1000; i++ {
        subjectID := fmt.Sprintf("user_%d", i)
        first := bucket("checkout_v2", subjectID)
        second := bucket("checkout_v2", subjectID)
        if first != second {
            t.Fatalf("bucket not stable for %s: got %d then %d", subjectID, first, second)
        }
    }
}

func TestBucketingDistribution(t *testing.T) {
    const n = 10_000
    inBucket := 0
    for i := 0; i < n; i++ {
        if inRollout("some_flag", fmt.Sprintf("user_%d", i), 10) {
            inBucket++
        }
    }
    pct := float64(inBucket) / n * 100
    // ожидаем ~10%, допуск ±2%
    if pct < 8 || pct > 12 {
        t.Errorf("distribution out of range: %.1f%%", pct)
    }
}
```

### Shutdown test

```go
func TestClientCloseShutsDownGoroutine(t *testing.T) {
    fetcher := &fakeFetcher{data: testConfig}
    c, err := New(Config{RefreshInterval: 50 * time.Millisecond}, fetcher)
    if err != nil {
        t.Fatal(err)
    }

    done := make(chan struct{})
    go func() {
        c.Close()
        close(done)
    }()

    select {
    case <-done:
        // ok
    case <-time.After(time.Second):
        t.Fatal("Close() did not return in time")
    }
}
```

## Performance considerations

- `atomic.Value.Load()` — single pointer dereference, zero allocation, no lock; измеримо быстрее RWMutex при > 10k RPS на горутину.
- `Subject` передается по значению (маленькая struct); pointer не нужен, избегаем heap escape.
- `valuesSet map[string]struct{}` компилируется при загрузке config, не при вызове `Bool`; lookup O(1) без аллокаций.
- В `bucket()` нет `fmt.Sprintf` — только прямая запись байт в хэш через `h.Write`.
- Не добавлять `user_id` и `account_id` в Prometheus labels — только в structured log fields; иначе cardinality взорвет memory хранилища метрик.
- Ориентир: `Bool()` на кэшированном snapshot должна выполняться за ~100–200ns. Если больше 1µs — смотреть на аллокации через `go test -bench=. -benchmem`.

## Интеграция с context и tracing

Decision нужно добавлять в span текущего request handler, а не создавать дочерний span — это сохраняет данные видимыми без увеличения глубины трейса.

```go
import (
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/trace"
    "log/slog"
)

func recordDecision(ctx context.Context, key string, d flags.Decision) {
    span := trace.SpanFromContext(ctx)
    span.SetAttributes(
        attribute.String("feature_flag.key", key),
        attribute.String("feature_flag.variant", d.Variant),
        attribute.String("feature_flag.reason", d.Reason),
    )

    slog.InfoContext(ctx, "feature flag evaluated",
        slog.String("flag.key", key),
        slog.String("flag.variant", d.Variant),
        slog.String("flag.reason", d.Reason),
    )
}
```

В `Evaluate` после evaluation loop:

```go
func (c *client) Evaluate(ctx context.Context, key string, sub Subject) Decision {
    snap := c.currentSnapshot()
    if snap == nil {
        return Decision{Variant: "", Reason: "no_config"}
    }
    d := evaluate(snap, key, sub)
    recordDecision(ctx, key, d)
    return d
}
```

`Reason` в логах критичен для debug: при инциденте сразу понятно, почему конкретный user получил `control` — denylist, attribute rule или просто не попал в процент.

## Interview-ready answer

Я бы начал с interface — `Bool`, `Variant`, `Close` — чтобы handlers не зависели от конкретного типа и можно было мокировать в тестах без сетевых вызовов. Внутри: `atomic.Value` для in-memory snapshot, потому что на read-heavy path он дает zero-lock read — один pointer dereference против RLock/RUnlock. Background goroutine обновляет snapshot каждые N секунд через `time.Ticker`; shutdown сигнализируется через `context.CancelFunc`, goroutine завершается, `wg.Wait()` в `Close()` гарантирует что in-flight refresh доделается. Bucketing — FNV-32a по `flagKey:subjectID`; соль flagKey важна, иначе все flags дают одинаковые bucket'ы. Rules оцениваются по очереди: denylist, allowlist, атрибуты, percentage, default — с short-circuit и capture `reason` для логов. Targeting lists компилируются в `map[string]struct{}` при загрузке config, не при каждом вызове. При stale или отсутствующем snapshot возвращаем явный fallback, пишем warning/error в лог. Тесты: mock-реализация интерфейса для handler-тестов; table-driven тесты на rules evaluation; bucketing stability и distribution test; shutdown test через `Close` + timeout.
