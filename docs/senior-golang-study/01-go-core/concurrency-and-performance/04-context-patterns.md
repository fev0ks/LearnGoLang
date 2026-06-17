# Context Patterns

`context.Context` — стандартный способ передавать сигналы отмены, таймауты и request-scoped данные через цепочку вызовов. Один из немногих интерфейсов, который стоит изучить до конца.

## Содержание

- [`context.Background()` vs `context.TODO()`](#contextbackground-vs-contexttodo)
- [WithCancel, WithTimeout, WithDeadline](#withcancel-withtimeout-withdeadline)
- [Отмена с причиной: `WithCancelCause` и `Cause` (Go 1.20+)](#отмена-с-причиной-withcancelcause-и-cause-go-120)
- [Propagation: почему ctx первый аргумент, не поле struct](#propagation-почему-ctx-первый-аргумент-не-поле-struct)
- [`context.Value` — когда допустимо, когда анти-паттерн](#contextvalue--когда-допустимо-когда-анти-паттерн)
- [Отмена и cleanup: `defer cancel()` всегда](#отмена-и-cleanup-defer-cancel-всегда)
- [context в HTTP сервере](#context-в-http-сервере)
- [Типичные ошибки](#типичные-ошибки)
- [Производные контексты — дерево](#производные-контексты--дерево)
- [`context.WithoutCancel` (Go 1.21+)](#contextwithoutcancel-go-121)
- [`context.AfterFunc` (Go 1.21+)](#contextafterfunc-go-121)
- [Под капотом: как устроена отмена](#под-капотом-как-устроена-отмена)
- [Разбор примеров-загадок](#разбор-примеров-загадок)
- [Interview-ready answer](#interview-ready-answer)

---

## `context.Background()` vs `context.TODO()`

Оба возвращают пустой non-nil context без cancel / timeout / values. Разница — **семантическая**:

```go
// Background — корневой контекст. Используй:
// - в main/init
// - в тестах как основу для дочерних
// - в долгоживущих горутинах верхнего уровня
ctx := context.Background()

// TODO — заглушка. Используй:
// - когда контекст нужен, но откуда его взять — ещё непонятно
// - при рефакторинге: пометить место, которое нужно исправить
// - в тестах, которые ещё не написаны
ctx := context.TODO()
```

`go vet` и `staticcheck` предупреждают если `context.TODO()` остаётся в production-коде.

---

## WithCancel, WithTimeout, WithDeadline

### `WithCancel` — явная отмена

```go
ctx, cancel := context.WithCancel(parent)
defer cancel() // ВСЕГДА defer cancel() — иначе goroutine leak

go longRunning(ctx)

// Отмена: когда нужно
cancel() // ctx.Done() закроется, ctx.Err() = context.Canceled
```

### `WithTimeout` — относительный таймаут

```go
ctx, cancel := context.WithTimeout(parent, 5*time.Second)
defer cancel() // нужен даже если timeout сработал — освобождает ресурсы

result, err := doHTTPRequest(ctx, url)
if errors.Is(err, context.DeadlineExceeded) {
    // запрос не уложился в 5 секунд
}
```

### `WithDeadline` — абсолютное время

```go
deadline := time.Now().Add(5 * time.Second)
ctx, cancel := context.WithDeadline(parent, deadline)
defer cancel()

// Эквивалентно WithTimeout, но с абсолютным временем
// Полезно когда deadline вычислен заранее
```

### Разница Timeout vs Deadline

```go
// WithTimeout(parent, 5s) → deadline = time.Now() + 5s
// WithDeadline(parent, t) → deadline = t (абсолютное)

// Если parent уже имеет deadline раньше — он не перезаписывается
parentCtx, _ := context.WithTimeout(ctx, 2*time.Second)
childCtx, _ := context.WithTimeout(parentCtx, 10*time.Second)
// childCtx истечёт через 2 секунды, не 10 — берётся минимум
```

### Проверка deadline

```go
if dl, ok := ctx.Deadline(); ok {
    remaining := time.Until(dl)
    if remaining < 100*time.Millisecond {
        return errors.New("not enough time for operation")
    }
}
```

---

## Отмена с причиной: `WithCancelCause` и `Cause` (Go 1.20+)

Проблема обычного `WithCancel`: `ctx.Err()` отдаёт лишь `context.Canceled` — **почему** отменили, неизвестно. Когда контекст могут отменить несколько причин (таймаут, ошибка upstream, дисконнект клиента), это важно различать. `WithCancelCause` позволяет передать **причину**-ошибку:

```go
ctx, cancel := context.WithCancelCause(parent)
// cancel принимает причину (error):
cancel(fmt.Errorf("upstream %q вернул 503", svc))

<-ctx.Done()
ctx.Err()            // по-прежнему context.Canceled (обратная совместимость)
context.Cause(ctx)   // → "upstream ... вернул 503" — РИЧЕР, та самая причина
```

- `cancel(nil)` → `Cause` вернёт `context.Canceled` (как `Err`);
- если контекст не отменён — `Cause` вернёт `nil`;
- по дедлайну — `Cause` == `context.DeadlineExceeded` (если не задана своя причина через `WithDeadlineCause`).

**`context.Cause(ctx)`** работает с **любым** контекстом: для отменённого через cause — отдаёт причину, иначе — то же, что `Err()`.

**`WithDeadlineCause(parent, t, cause)`** (Go 1.21) — кастомная причина именно при срабатывании дедлайна:

```go
ctx, cancel := context.WithDeadlineCause(parent, deadline,
    errors.New("SLA 200ms на платёжный шлюз истёк"))
defer cancel()
// по дедлайну: ctx.Err() == DeadlineExceeded, но context.Cause(ctx) == кастомная ошибка
```

Где пригождается: `errgroup.WithContext` (Go 1.20+) внутри использует `WithCancelCause` — при первой ошибке воркера она становится **причиной** отмены, и в других воркерах `context.Cause(gctx)` отдаёт именно ту ошибку, а не безликий `Canceled`.

---

## Propagation: почему ctx первый аргумент, не поле struct

### Правило

```go
// Правильно: ctx — первый параметр каждой функции, выполняющей I/O
func (s *Service) GetUser(ctx context.Context, id int) (*User, error) {
    return s.repo.FindByID(ctx, id)
}

// Неправильно: ctx в поле struct
type Service struct {
    ctx context.Context // антипаттерн
    // ...
}
```

**Почему ctx — параметр, не поле:**

1. **Разные запросы — разные контексты.** Struct существует дольше одного запроса; если сохранить ctx в struct, все будущие запросы получат контекст первого запроса (с его deadline, cancel, values).

2. **Явность:** caller видит, что функция будет учитывать отмену.

3. **Тестируемость:** легко передать `context.Background()` или мок-context.

4. **Go convention:** все stdlib и популярные библиотеки используют `ctx context.Context` как первый параметр.

```go
// Типичный стек вызовов
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context() // context от HTTP сервера
    
    user, err := h.svc.GetUser(ctx, userID) // передаём дальше
    if err != nil {
        // ...
    }
}

func (s *UserService) GetUser(ctx context.Context, id int) (*User, error) {
    return s.repo.FindByID(ctx, id) // ещё дальше
}

func (r *UserRepo) FindByID(ctx context.Context, id int) (*User, error) {
    return r.db.QueryRowContext(ctx, query, id).Scan(&u) // в SQL query
}
```

---

## `context.Value` — когда допустимо, когда анти-паттерн

### Синтаксис

```go
// Всегда используй неэкспортируемый тип ключа!
type contextKey string

const (
    requestIDKey contextKey = "request_id"
    userIDKey    contextKey = "user_id"
)

// Store
ctx = context.WithValue(ctx, requestIDKey, "req-123")

// Load
if reqID, ok := ctx.Value(requestIDKey).(string); ok {
    log.Printf("request_id=%s", reqID)
}
```

**Почему неэкспортируемый тип ключа?** Предотвращает коллизии: два пакета не могут случайно использовать одинаковый ключ, так как типы из разных пакетов разные даже при одинаковом underlying value.

### Когда `context.Value` допустимо

1. **Request-scoped метаданные** для observability, не для бизнес-логики:
   - request ID / trace ID
   - authenticated user (только ID, не полный объект)
   - correlation ID для логов

2. **Cross-cutting concerns**, которые нежелательно тащить через все слои как параметры:
   - трейсинг span
   - logger с полями

```go
// Хороший use case: trace ID для логов
type traceIDKey struct{}

func WithTraceID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, traceIDKey{}, id)
}

func TraceIDFrom(ctx context.Context) string {
    id, _ := ctx.Value(traceIDKey{}).(string)
    return id
}
```

### Когда `context.Value` — анти-паттерн

❌ **Бизнес-данные через контекст** — скрывает зависимости, усложняет тестирование:

```go
// Плохо: productID прячется в контексте
ctx = context.WithValue(ctx, "product_id", productID)
price := calculatePrice(ctx) // откуда берёт productID — непонятно

// Хорошо: явный параметр
price := calculatePrice(ctx, productID)
```

❌ **Опциональные параметры функций** — создаёт hidden coupling:

```go
// Плохо
func createOrder(ctx context.Context) error {
    userID := ctx.Value("user_id").(int) // молчаливая зависимость
    // ...
}

// Хорошо
func createOrder(ctx context.Context, userID int) error { ... }
```

### Правило: контекст несёт "кто это вызывает" (identity, tracing), не "что делать" (data)

---

## Отмена и cleanup: `defer cancel()` всегда

```go
// Правило: всегда defer cancel() сразу после создания контекста
ctx, cancel := context.WithTimeout(parent, 5*time.Second)
defer cancel() // ← сразу, на следующей строке

// Зачем defer cancel() даже когда timeout уже сработал?
// WithTimeout создаёт горутину-таймер внутри.
// Без cancel() горутина-таймер и ресурсы (goroutine, channel) не освобождаются
// до истечения timeout или завершения parent.
// При большом количестве коротких запросов → goroutine leak.
```

### Явная vs неявная отмена

```go
func fetchWithRetry(parent context.Context, url string, maxRetries int) error {
    for attempt := range maxRetries {
        // Создаём timeout для каждой попытки отдельно
        ctx, cancel := context.WithTimeout(parent, 2*time.Second)
        err := fetch(ctx, url)
        cancel() // явный cancel — не defer, чтобы не накапливать
        
        if err == nil {
            return nil
        }
        if errors.Is(err, context.Canceled) {
            return err // parent отменён — прерываем retry
        }
        
        // exponential backoff
        select {
        case <-time.After(time.Duration(attempt) * 100 * time.Millisecond):
        case <-parent.Done():
            return parent.Err()
        }
    }
    return errors.New("max retries exceeded")
}
```

---

## context в HTTP сервере

### `r.Context()` — context запроса

```go
func handler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context() // отменяется когда клиент разрывает соединение
    
    result, err := db.QueryContext(ctx, "SELECT ...")
    if err != nil {
        if errors.Is(err, context.Canceled) {
            // клиент ушёл — тихо игнорируем
            return
        }
        http.Error(w, err.Error(), 500)
        return
    }
    // ...
}
```

### Добавление timeout к запросу

```go
func handler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()
    
    // Теперь весь pipeline имеет 30-секундный лимит
    result, err := s.processRequest(ctx, r)
    // ...
}
```

### Клиентские таймауты через context

```go
// http.Client timeout — для всего запроса
client := &http.Client{Timeout: 10 * time.Second}

// context timeout — можно дифференцировать
func callAPI(ctx context.Context) error {
    ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
    defer cancel()
    
    req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("api call: %w", err)
    }
    defer resp.Body.Close()
    return nil
}
```

---

## Типичные ошибки

### 1. Сохранение ctx в struct

```go
// Плохо
type Handler struct {
    ctx context.Context // будет устаревшим при следующем запросе
}

// Хорошо
func (h *Handler) Handle(ctx context.Context, req *Request) { ... }
```

### 2. Создание без передачи

```go
// Плохо
func processAll(items []Item) error {
    ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
    defer cancel()
    
    for _, item := range items {
        if err := process(item); err != nil { // ctx не передаётся!
            return err
        }
    }
    return nil
}

// Хорошо
func processAll(ctx context.Context, items []Item) error {
    ctx, cancel := context.WithTimeout(ctx, time.Minute)
    defer cancel()
    
    for _, item := range items {
        if err := process(ctx, item); err != nil {
            return err
        }
    }
    return nil
}
```

### 3. Использование context.Background() глубоко в стеке

```go
// Плохо: теряем отмену от caller
func (r *Repo) FindByID(id int) (*User, error) {
    ctx := context.Background() // игнорируем отмену!
    return r.db.QueryRowContext(ctx, query, id).Scan(...)
}

// Хорошо
func (r *Repo) FindByID(ctx context.Context, id int) (*User, error) {
    return r.db.QueryRowContext(ctx, query, id).Scan(...)
}
```

### 4. Забытый cancel — goroutine/timer leak

```go
// Плохо
func makeRequest() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    // забыли cancel!
    doHTTP(ctx)
    // таймер живёт ещё 5 секунд даже если запрос завершился за 10мс
}

// Хорошо
func makeRequest() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    doHTTP(ctx)
}
```

---

## Производные контексты — дерево

```go
// Контексты образуют дерево отмены:
// Отмена parent → отмена всех children

root := context.Background()
  ├── reqCtx (WithCancel) — отменяется при disconnect
  │     ├── dbCtx (WithTimeout, 100ms)
  │     └── extCtx (WithTimeout, 200ms)
  └── bgCtx (долгоживущий background job)
        └── jobCtx (WithCancel) — отменяется при shutdown
```

```go
// Пример: request-scoped дерево
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context() // root для этого запроса

    // Добавляем metadata
    ctx = WithTraceID(ctx, generateTraceID())
    ctx = WithUserID(ctx, extractUserID(r))

    // Создаём timeout для I/O операций
    ioCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
    defer cancel()

    // Передаём дальше
    h.svc.Handle(ioCtx, parseRequest(r))
}
```

---

## `context.WithoutCancel` (Go 1.21+)

Иногда нужно продолжить работу после отмены родительского контекста — например, записать audit log после завершения запроса.

```go
func handler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    result, err := process(ctx, r)
    
    // Даже если ctx отменён (клиент ушёл) — записываем аудит
    auditCtx := context.WithoutCancel(ctx) // наследует values, но не cancel
    go audit.Log(auditCtx, r, result, err)
}
```

---

## `context.AfterFunc` (Go 1.21+)

Регистрирует функцию, которая выполнится **в своей горутине**, когда `ctx` завершится (отмена или дедлайн). Заменяет ручной boilerplate `go func(){ <-ctx.Done(); cleanup() }()` и проблему его остановки.

```go
stop := context.AfterFunc(ctx, func() {
    // вызовется один раз, когда ctx.Done() закроется
    conn.Close() // например, разорвать соединение при отмене
})

// stop() снимает регистрацию. Возвращает:
//   true  — успели отменить ДО запуска f (f не выполнится)
//   false — f уже запущена/запускается или ctx уже был отменён
if stop() {
    // f не понадобилась — отписались
}
```

Тонкости:
- если `ctx` **уже** отменён на момент `AfterFunc` — `f` запускается сразу (в новой горутине);
- `f` выполняется **вне** держателя локов контекста — внутри `f` можно безопасно вызывать методы контекста;
- удобно для «привязать освобождение ресурса к жизни контекста» без своей горутины-сторожа и риска её утечки.

---

## Под капотом: как устроена отмена

Отменяемый контекст — это `*cancelCtx` (для `WithCancel`/`WithCancelCause`); `WithTimeout`/`WithDeadline` дают `*timerCtx`, который **встраивает** `cancelCtx` + таймер; `WithValue` — `*valueCtx` (без отмены).

```go
type cancelCtx struct {
    Context                          // родитель
    mu       sync.Mutex
    done     atomic.Value            // chan struct{}, СОЗДАЁТСЯ ЛЕНИВО при первом Done()
    children map[canceler]struct{}   // потомки, которых надо отменить вслед за собой
    err      error                   // context.Canceled / DeadlineExceeded
    cause    error                   // причина (WithCancelCause)
}
```

Три механики, которые объясняют всё поведение:

1. **`Done()` создаёт канал лениво.** Пока никто не звал `Done()`, канала нет — поэтому контексты, которые не ждут, не аллоцируют его. (И почему у `Background()` `Done()` возвращает `nil` — у него нет `cancelCtx`.)

2. **`propagateCancel` при создании потомка** идёт **вверх** по цепочке родителей до ближайшего `*cancelCtx` и регистрирует себя в его `children`. Для stdlib-родителя это просто запись в map — **без отдельной горутины**. (Своя горутина-сторож заводится только для «чужого» отменяемого родителя не из stdlib.) Если родитель уже отменён — потомок рождается отменённым сразу (Загадка 3).

3. **`cancel(err, cause)`** под `mu`: ставит `err`+`cause`, **закрывает** `done`-канал, затем рекурсивно вызывает `cancel` у всех `children` и **удаляет себя** из `children` родителя. Вот почему отмена идёт строго **вниз** по дереву (Загадка 5), а у `timerCtx` `cancel` ещё и останавливает таймер.

Отсюда два практических следствия, которые мы уже видели:

- **Почему `defer cancel()` обязателен.** Потомок остаётся в `children` родителя, пока не вызван его `cancel` (или пока не отменится родитель). Забыл `cancel` — узел висит в map родителя, не собирается GC, а для `WithTimeout` ещё и таймер тикает до срока. На высоком RPS — утечка памяти/горутин/таймеров.
- **Почему «дедлайн = минимум».** `timerCtx` потомка зарегистрирован в `children` родителя. Чей таймер/cancel сработает первым — тот и закроет `done` по цепочке. Потомок с бóльшим таймаутом не может «пережить» родителя: родительский `cancel` придёт по `children` раньше (Загадка 2).

---

## Разбор примеров-загадок

### Загадка 1: Background().Done() блокируется вечно

```go
func main() {
    ctx := context.Background()
    select {
    case <-ctx.Done():
        fmt.Println("done")
    }
}
```

<details>
<summary>Ответ</summary>

```
fatal error: all goroutines are asleep - deadlock!
```

У `context.Background()` (и `TODO()`) метод `Done()` возвращает **nil-канал** — он никогда не закроется, чтение блокируется навсегда. `select` без других case или `default` зависает → дедлок.

`Done()` возвращает рабочий канал только у производных контекстов с отменой (`WithCancel/Timeout/Deadline`). Если ждёшь `Done()` — убедись, что контекст вообще отменяемый.
</details>

---

### Загадка 2: дочерний таймаут не продлевает родительский

```go
start := time.Now()
parent, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
defer cancel()
child, cancel2 := context.WithTimeout(parent, 1*time.Hour)
defer cancel2()

<-child.Done()
fmt.Println(child.Err(), time.Since(start).Round(10*time.Millisecond))  // ?
```

<details>
<summary>Ответ</summary>

```
context deadline exceeded 50ms
```

Дочерний контекст наследует дедлайн родителя, если тот **раньше**. `WithTimeout(parent, 1h)` не может продлить жизнь за пределы родительских 50ms — берётся **минимум** из дедлайнов по цепочке. Через 50ms отменяется parent → отменяется child. Контекстом нельзя «расширить» отведённое время; для этого нужен отдельный корень (`WithoutCancel` + новый таймаут).
</details>

---

### Загадка 3: производный от уже отменённого

```go
ctx, cancel := context.WithCancel(context.Background())
cancel()                                    // отменяем сразу
child, _ := context.WithTimeout(ctx, time.Hour)
fmt.Println(child.Err())                    // ?
```

<details>
<summary>Ответ</summary>

```
context.Canceled
```

Контекст, произведённый от **уже отменённого**, рождается отменённым — `child.Err()` сразу `context.Canceled` (унаследована причина родителя, не `DeadlineExceeded`). Любой `select { case <-child.Done(): }` сработает мгновенно. Полезно помнить: проверяй `ctx.Err()` перед дорогой работой — родитель мог отмениться ещё до входа.
</details>

---

### Загадка 4: ключ-строка vs типизированный ключ

```go
type ctxKey string
ctx := context.WithValue(context.Background(), "user", "alice") // string-ключ
ctx = context.WithValue(ctx, ctxKey("user"), "bob")            // типизированный

fmt.Println(ctx.Value("user"))          // ?
fmt.Println(ctx.Value(ctxKey("user")))  // ?
```

<details>
<summary>Ответ</summary>

```
alice
bob
```

Ключи сравниваются и по **значению, и по типу**. `"user"` (тип `string`) и `ctxKey("user")` (тип `ctxKey`) — **разные** ключи, оба значения сосуществуют. Отсюда два вывода: (1) на голый `string`-ключ `go vet` ругается («should not use built-in type string as key») — два пакета с ключом `"user"` затрут друг друга; (2) используй неэкспортируемый тип ключа (`type ctxKey struct{}` / `string`), чтобы коллизий между пакетами не было в принципе.
</details>

---

### Загадка 5: отмена идёт только вниз по дереву

```go
parent, cancelP := context.WithCancel(context.Background())
childA, _ := context.WithCancel(parent)
childB, cancelB := context.WithCancel(parent)

cancelB()
fmt.Println(childB.Err(), childA.Err(), parent.Err())  // ?
cancelP()
fmt.Println(childA.Err())                              // ?
```

<details>
<summary>Ответ</summary>

```
context.Canceled <nil> <nil>
context.Canceled
```

`cancel()` распространяется **только вниз** — на потомков. Отмена `childB` не трогает ни брата `childA`, ни родителя. А отмена `parent` отменяет всех потомков (`childA` становится `Canceled`). Поэтому `cancel` дочернего безопасен (не убьёт соседей), а отмена/таймаут на родителе гасит всё поддерево.
</details>

---

## Interview-ready answer

**1. Почему ctx — первый параметр, а не поле struct?**

- Struct живёт дольше запроса; ctx в поле = все будущие запросы получат устаревший контекст первого (его deadline/values/cancel). Context — per-request, не per-service. Параметр делает зависимость явной и упрощает тесты.

**2. Когда использовать context.Value?**

- Только request-scoped метаданные для cross-cutting (trace/request ID, user ID для логов). Не бизнес-данные и не опциональные параметры — это прячет зависимости и ломает тестируемость. Ключ — неэкспортируемого типа, иначе коллизии и `go vet`-варнинг.

**3. Зачем defer cancel(), если timeout сам истечёт?**

- `WithCancel/Timeout` регистрирует контекст в дереве отмены (а таймер — ещё и timer). Без `cancel()` ресурсы висят до срабатывания таймаута/отмены родителя. На высоком RPS это копит goroutine/timer leak и давит на GC.

**4. Background vs TODO?**

- Оба — пустой корневой неотменяемый контекст; `Done()` у них nil (ждать бессмысленно — зависнет). Разница семантическая: `Background` — осознанный корень (main, тесты, top-level); `TODO` — заглушка «контекст нужен, но непонятно откуда», `vet`/`staticcheck` напоминают убрать.

**5. Что с дедлайнами в цепочке контекстов?**

- Берётся самый ранний дедлайн по цепочке — потомок не может продлить родителя. Производный от уже отменённого рождается отменённым (`Err()` = причина родителя сразу).

**6. Куда распространяется cancel?**

- Только вниз, на потомков. Отмена ребёнка не трогает родителя и братьев; отмена/таймаут родителя гасит всё поддерево. `WithoutCancel` (1.21+) обрывает наследование отмены, сохраняя values — для дел вроде audit-лога после ухода клиента.

**7. Как узнать ПРИЧИНУ отмены и как отмена работает внутри?**

- `ctx.Err()` отдаёт только `Canceled`/`DeadlineExceeded`. Чтобы знать, *почему* отменили (таймаут vs ошибка upstream vs дисконнект), — `WithCancelCause` + `context.Cause(ctx)` (Go 1.20+); так делает и `errgroup`. Внутри отменяемый ctx (`cancelCtx`) держит `children map` и ленивый `done`-канал: при создании потомок регистрируется в ближайшем родителе-`cancelCtx` (без горутины), а `cancel` закрывает `done` и рекурсивно отменяет `children`. Отсюда: отмена идёт только вниз, забытый `cancel` оставляет узел в `children` родителя (утечка), а дедлайн в цепочке = минимум.
