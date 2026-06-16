# log/slog: структурированное логирование

`log/slog` (Go 1.21+) — стандартный пакет структурированного логирования: вместо строк-сообщений лог состоит из **пар ключ-значение**, которые легко парсить, фильтровать и агрегировать (JSON/logfmt). До 1.21 эту нишу занимали `zap`, `zerolog`, `logrus`; теперь базовый функционал есть в stdlib, а главное — единый интерфейс `slog.Handler`, к которому подключаются и сторонние бэкенды.

Центральный для продакшна сценарий — **прокидывать `traceID`/`requestID`/`userID` из `context.Context` в каждую запись лога автоматически**, не передавая их в каждый вызов вручную. Ему посвящён отдельный раздел ниже.

## Содержание

- [Зачем структурированное логирование](#зачем-структурированное-логирование)
- [Базовый API: Logger, Handler, уровни](#базовый-api-logger-handler-уровни)
- [Атрибуты: Attr, типизированные хелперы, группы](#атрибуты-attr-типизированные-хелперы-группы)
- [With: предзаполненные поля](#with-предзаполненные-поля)
- [Логирование с context: LogAttrs и ...Context методы](#логирование-с-context-logattrs-и-context-методы)
- [traceID / requestID / userID из ctx через кастомный Handler](#traceid--requestid--userid-из-ctx-через-кастомный-handler)
- [Уровни и динамическая фильтрация](#уровни-и-динамическая-фильтрация)
- [ReplaceAttr: редактирование и маскирование](#replaceattr-редактирование-и-маскирование)
- [Производительность: Attr vs Any, LogAttrs](#производительность-attr-vs-any-logattrs)
- [Разбор примеров-загадок](#разбор-примеров-загадок)
- [Interview-ready answer](#interview-ready-answer)

---

## Зачем структурированное логирование

```go
// Текстовый лог — человекочитаемо, но машинно бесполезно:
log.Printf("user %d failed login from %s", userID, ip)
// "user 42 failed login from 10.0.0.1"

// Структурированный лог — каждое поле адресуемо:
slog.Info("failed login", "user_id", userID, "ip", ip)
// {"time":"...","level":"INFO","msg":"failed login","user_id":42,"ip":"10.0.0.1"}
```

Выигрыш: по структурированным логам можно строить запросы (`level=ERROR AND user_id=42`), алерты, дашборды. В распределённой системе ключи вроде `trace_id` связывают записи одного запроса через все сервисы.

---

## Базовый API: Logger, Handler, уровни

Три сущности:

- **`slog.Logger`** — фронтенд, у которого вызывают `Info/Warn/Error/Debug`;
- **`slog.Handler`** — бэкенд, решающий *как* и *куда* писать (формат, фильтрация, вывод);
- **`slog.Record`** — одна запись (время, уровень, сообщение, атрибуты).

```go
// Два встроенных хендлера:
textLogger := slog.New(slog.NewTextHandler(os.Stdout, nil)) // logfmt: key=value
jsonLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil)) // JSON-строки

jsonLogger.Info("server started", "port", 8080, "env", "prod")
// {"time":"2026-06-08T10:00:00Z","level":"INFO","msg":"server started","port":8080,"env":"prod"}

// Пакетный логгер по умолчанию (TextHandler в stderr):
slog.Info("hello", "x", 1)

// Заменить дефолтный логгер на весь процесс:
slog.SetDefault(jsonLogger)
```

Четыре уровня: `Debug` (-4), `Info` (0), `Warn` (4), `Error` (8). Значения — числа, между ними можно вставлять кастомные.

---

## Атрибуты: Attr, типизированные хелперы, группы

Пары ключ-значение можно передавать двумя способами:

```go
// 1. Чередующиеся аргументы (удобно, но без проверки типов и нечётности)
slog.Info("event", "user_id", 42, "ok", true)

// 2. Типизированные slog.Attr — быстрее и безопаснее
slog.LogAttrs(ctx, slog.LevelInfo, "event",
    slog.Int("user_id", 42),
    slog.Bool("ok", true),
    slog.String("ip", ip),
    slog.Duration("took", elapsed),
    slog.Time("at", t),
)
```

Типизированные конструкторы: `slog.String`, `slog.Int`, `slog.Int64`, `slog.Uint64`, `slog.Float64`, `slog.Bool`, `slog.Time`, `slog.Duration`, `slog.Any` (для произвольного типа через reflection).

**Группы** вкладывают атрибуты под общим ключом:

```go
slog.Info("request",
    slog.Group("http",
        slog.String("method", "GET"),
        slog.Int("status", 200),
    ),
)
// JSON: "http":{"method":"GET","status":200}
```

Тип со своим представлением реализует `slog.LogValuer` — аналог `Stringer` для логов (удобно для маскирования секретов, см. загадку 3):

```go
func (t Token) LogValue() slog.Value {
    return slog.StringValue("REDACTED")
}
```

---

## With: предзаполненные поля

`logger.With(...)` возвращает **новый логгер** с «приклеенными» атрибутами — они попадут в каждую последующую запись. Так создают логгер компонента/запроса:

```go
// Логгер сервиса с постоянными полями
svcLog := slog.Default().With(
    slog.String("service", "billing"),
    slog.String("version", "1.4.2"),
)
svcLog.Info("charge ok", "amount", 100)
// {... "service":"billing","version":"1.4.2","msg":"charge ok","amount":100}

// Логгер запроса — добавляем поля поверх
reqLog := svcLog.With("request_id", reqID)
reqLog.Info("processing")  // request_id попадёт автоматически
```

`With` вычисляет приклеенные атрибуты **один раз** (хендлер их предформатирует) — дешевле, чем повторять их в каждом вызове.

> Передача логгера по цепочке вызовов через `With` — рабочий приём, но для request-scoped полей (trace/request/user ID) чаще удобнее достать их из `context.Context` в самом хендлере — следующий раздел.

---

## Логирование с context: LogAttrs и ...Context методы

У каждого метода логирования есть `...Context`-вариант, принимающий `ctx`:

```go
slog.InfoContext(ctx, "msg", "key", val)
slog.WarnContext(ctx, "msg")
slog.ErrorContext(ctx, "msg", "err", err)

// LogAttrs — самый эффективный путь: ctx + уровень + типизированные Attr
slog.LogAttrs(ctx, slog.LevelInfo, "msg", slog.Int("x", 1))
```

Зачем передавать `ctx` в лог, если в `Record` его поля не попадают автоматически? Затем, что **`ctx` доходит до `Handler.Handle(ctx, record)`** — и кастомный хендлер может вытащить из него request-scoped значения. Именно на этом строится автоматический проброс trace/request/user ID.

---

## traceID / requestID / userID из ctx через кастомный Handler

Цель: единожды положить идентификаторы в `ctx` (обычно в middleware), а дальше **любой** вызов `slog.InfoContext(ctx, ...)` в любом слое автоматически добавит `trace_id`, `request_id`, `user_id` — без передачи их в каждый вызов.

### Шаг 1. Ключи и хелперы для ctx

Используется неэкспортируемый тип ключа (как и в [context-паттернах](../01-go-core/concurrency-and-performance/04-context-patterns.md), раздел «context.Value», чтобы избежать коллизий между пакетами):

```go
package reqctx

import "context"

type ctxKey int

const (
    traceIDKey ctxKey = iota
    requestIDKey
    userIDKey
)

func WithTraceID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, traceIDKey, id)
}
func WithRequestID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, requestIDKey, id)
}
func WithUserID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, userIDKey, id)
}

func TraceID(ctx context.Context) (string, bool) {
    v, ok := ctx.Value(traceIDKey).(string)
    return v, ok
}
func RequestID(ctx context.Context) (string, bool) {
    v, ok := ctx.Value(requestIDKey).(string)
    return v, ok
}
func UserID(ctx context.Context) (string, bool) {
    v, ok := ctx.Value(userIDKey).(string)
    return v, ok
}
```

### Шаг 2. Хендлер-обёртка, читающая ctx

Ключевой приём — **обернуть** существующий хендлер (`JSONHandler`) и в методе `Handle` дописать атрибуты из `ctx`. Реализуются все четыре метода интерфейса `slog.Handler`, но обогащение — только в `Handle`:

```go
package reqctx

import (
    "context"
    "log/slog"
)

// ContextHandler оборачивает любой slog.Handler и добавляет
// request-scoped идентификаторы из ctx в каждую запись.
type ContextHandler struct {
    slog.Handler
}

func NewContextHandler(h slog.Handler) *ContextHandler {
    return &ContextHandler{Handler: h}
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
    if id, ok := TraceID(ctx); ok {
        r.AddAttrs(slog.String("trace_id", id))
    }
    if id, ok := RequestID(ctx); ok {
        r.AddAttrs(slog.String("request_id", id))
    }
    if id, ok := UserID(ctx); ok {
        r.AddAttrs(slog.String("user_id", id))
    }
    return h.Handler.Handle(ctx, r) // делегируем встроенному хендлеру
}
```

> Встраивание `slog.Handler` бесплатно даёт реализации `Enabled`, `WithAttrs`, `WithGroup` — переопределяется только `Handle`. (`WithAttrs`/`WithGroup` вернут обёрнутый, но не «наш» хендлер; для большинства задач это нормально. Если нужно сохранять тип после `With`, эти два метода тоже оборачивают — см. примечание в конце раздела.)

### Шаг 3. Сборка логгера

```go
base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
logger := slog.New(reqctx.NewContextHandler(base))
slog.SetDefault(logger)
```

### Шаг 4. Middleware кладёт идентификаторы в ctx

```go
func TracingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        ctx = reqctx.WithTraceID(ctx, getOrGenTraceID(r))   // из заголовка или новый
        ctx = reqctx.WithRequestID(ctx, uuid.NewString())
        if uid := authUserID(r); uid != "" {
            ctx = reqctx.WithUserID(ctx, uid)
        }
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### Шаг 5. В любом слое — просто логируем с ctx

```go
func (s *Service) Charge(ctx context.Context, amount int) error {
    slog.InfoContext(ctx, "charge started", "amount", amount)
    // ...
    if err != nil {
        slog.ErrorContext(ctx, "charge failed", "err", err)
        return err
    }
    return nil
}

// Вывод — идентификаторы добавлены автоматически:
// {"time":"...","level":"INFO","msg":"charge started","amount":100,
//  "trace_id":"abc123","request_id":"7f3...","user_id":"42"}
```

Идентификаторы нигде не передаются явно в `Charge` — их подставляет хендлер. **Условие — вызывать именно `...Context`-методы** (`InfoContext`, `ErrorContext`, `LogAttrs`), иначе `ctx` до хендлера не дойдёт и поля не появятся (см. загадку 1).

> Альтернатива без кастомного хендлера — класть в ctx сам логгер (`ctx = ContextWithLogger(ctx, log.With("trace_id", id))`) и доставать его в каждом слое. Это работает, но требует таскать «достань логгер из ctx» по всему коду; хендлер-подход прозрачнее: бизнес-код просто вызывает `slog.InfoContext(ctx, ...)`.

---

## Уровни и динамическая фильтрация

Порог уровня задаётся в `HandlerOptions`. Чтобы менять его в рантайме (без рестарта) — `slog.LevelVar`:

```go
var lvl slog.LevelVar          // потокобезопасный изменяемый уровень
lvl.Set(slog.LevelInfo)

h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: &lvl})
logger := slog.New(h)

// Где-то по сигналу/ручке админки — включить debug на лету:
lvl.Set(slog.LevelDebug)
```

`HandlerOptions.AddSource: true` добавляет файл:строку вызова (дороже, для отладки).

---

## ReplaceAttr: редактирование и маскирование

`HandlerOptions.ReplaceAttr` — функция, вызываемая для каждого атрибута перед записью. Применения: переименовать стандартные ключи, скрыть секреты, переформатировать время.

```go
opts := &slog.HandlerOptions{
    ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
        // Замаскировать пароли/токены по имени ключа
        if a.Key == "password" || a.Key == "token" {
            return slog.String(a.Key, "***")
        }
        // Переименовать стандартный msg → message
        if a.Key == slog.MessageKey {
            a.Key = "message"
        }
        return a
    },
}
slog.New(slog.NewJSONHandler(os.Stdout, opts))
```

Для маскирования по типу значения (а не по имени ключа) лучше реализовать `LogValuer` на самом типе — тогда секрет не утечёт, как бы поле ни назвали.

---

## Производительность: Attr vs Any, LogAttrs

- **`LogAttrs` + типизированные `slog.Attr`** — самый дешёвый путь: нет упаковки в `any`, меньше аллокаций.
- **Чередующиеся `any`-аргументы** (`"key", val`) удобны, но каждое значение боксится в `interface{}`; на горячем пути это аллокации.
- **`Enabled` отсекает рано:** если уровень ниже порога, `slog` не форматирует Record. Но аргументы всё равно **вычисляются** до вызова — дорогую подготовку прячьте за `if logger.Enabled(ctx, slog.LevelDebug)` или за `LogValuer` (ленивое вычисление).
- **`With` предформатирует** приклеенные атрибуты один раз — выгодно для постоянных полей.

```go
// Дорогой аргумент вычисляется всегда, даже если Debug выключен:
slog.Debug("dump", "state", expensiveSerialize(obj)) // ❌ expensiveSerialize вызовется

// Защита порогом:
if slog.Default().Enabled(ctx, slog.LevelDebug) {
    slog.Debug("dump", "state", expensiveSerialize(obj))
}
```

---

## Разбор примеров-загадок

### Загадка 1: Info vs InfoContext — куда делся trace_id

```go
// Хендлер из ctx настроен (как выше). Middleware положил trace_id в ctx.
func (s *Service) Do(ctx context.Context) {
    slog.Info("via Info")              // ?
    slog.InfoContext(ctx, "via Ctx")   // ?
}
```

<details>
<summary>Ответ</summary>

```
{"...","msg":"via Info"}                      ← без trace_id
{"...","msg":"via Ctx","trace_id":"abc123"}   ← с trace_id
```

`slog.Info` не принимает `ctx` — он вызывает `Handle` с `context.Background()`, и кастомный хендлер не находит идентификаторов. Только `...Context`-методы (`InfoContext`, `ErrorContext`, `LogAttrs`) доносят `ctx` до хендлера. Правило: в коде, где доступен `ctx`, всегда логировать через `...Context`.
</details>

---

### Загадка 2: With возвращает новый логгер

```go
base := slog.Default()
base.With("a", 1)                      // результат не присвоен
base.Info("hello")                     // ?
```

<details>
<summary>Ответ</summary>

```
{"...","msg":"hello"}   ← без "a":1
```

`With` не мутирует логгер, а возвращает **новый**. `base.With("a", 1)` без присваивания просто выбрасывает результат. Нужно `logger := base.With("a", 1)` и логировать через `logger`. Та же модель, что у `context.WithValue` — иммутабельность.
</details>

---

### Загадка 3: секрет утёк в лог

```go
type Password string

slog.Info("login", "password", Password("hunter2")) // ?
```

<details>
<summary>Ответ</summary>

```
{"...","msg":"login","password":"hunter2"}   ← секрет в логах!
```

`slog` по умолчанию печатает значение как есть. Чтобы тип никогда не светил содержимое — реализовать `LogValuer`:

```go
func (Password) LogValue() slog.Value {
    return slog.StringValue("REDACTED")
}
// теперь всегда: "password":"REDACTED", как бы поле ни назвали
```

Это надёжнее, чем фильтровать по имени ключа в `ReplaceAttr` (имя могут поменять, а тип останется).
</details>

---

### Загадка 4: нечётное число аргументов

```go
slog.Info("event", "user_id", 42, "orphan") // ?
```

<details>
<summary>Ответ</summary>

```
{"...","msg":"event","user_id":42,"!BADKEY":"orphan"}
```

При чередующихся аргументах `slog` ждёт пары ключ-значение. Лишний «висячий» аргумент получает синтетический ключ `!BADKEY` — паники нет, но лог засоряется. Типизированные `slog.Attr` + `LogAttrs` исключают такую ошибку на этапе компиляции (ключ и значение связаны в одном `Attr`).
</details>

---

## Interview-ready answer

**1. Зачем slog, если есть log/zap/zerolog?**
`slog` (Go 1.21+) — стандарт структурированного логирования в stdlib: пары ключ-значение, JSON/text-хендлеры и, главное, единый интерфейс `slog.Handler`, к которому подключаются сторонние бэкенды. Структурированные логи machine-readable: по ним строят запросы, алерты, корреляцию по `trace_id`.

**2. Logger vs Handler?**
`Logger` — фронтенд (`Info/Warn/...`), `Handler` — бэкенд (формат, фильтр, вывод), `Record` — одна запись. Кастомизация = свой `Handler` (часто обёртка над `JSONHandler`).

**3. Как прокинуть trace_id/request_id/user_id из контекста?**
Middleware кладёт ID в `ctx` (неэкспортируемый ключ). Кастомный `Handler` оборачивает `JSONHandler` и в `Handle(ctx, record)` достаёт значения из `ctx` и делает `record.AddAttrs(...)`. Бизнес-код просто вызывает `slog.InfoContext(ctx, ...)` — поля добавляются автоматически. Критично использовать `...Context`-методы, иначе `ctx` до хендлера не дойдёт.

**4. With — что делает?**
Возвращает новый логгер с приклеенными атрибутами (попадают в каждую запись), предформатируя их один раз. Иммутабелен — результат надо присвоить, иначе эффекта нет.

**5. Как не залогировать секрет?**
Реализовать `LogValuer` на типе (`LogValue() slog.Value` → `REDACTED`) — надёжнее, чем фильтровать по имени ключа через `ReplaceAttr`, потому что привязано к типу, а не к названию поля.

**6. Производительность?**
`LogAttrs` + типизированные `slog.Attr` дешевле чередующихся `any`-аргументов (нет боксинга). Аргументы вычисляются даже при выключенном уровне — дорогую подготовку прятать за `Enabled` или ленивый `LogValuer`. Уровень меняется в рантайме через `slog.LevelVar`.
