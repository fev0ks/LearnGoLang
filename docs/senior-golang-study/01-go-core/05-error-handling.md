# Error Handling

Обработка ошибок — одна из самых часто задаваемых тем на senior Go собеседованиях. Не из-за синтаксиса, а из-за принятия решений: когда sentinel, когда тип, когда паника, как передавать через goroutines.

## Содержание

- [Интерфейс `error`](#интерфейс-error)
- [`errors.Is` и `errors.As` — механика wrapping chain](#errorsis-и-errorsas--механика-wrapping-chain)
- [Sentinel errors vs типизированные ошибки](#sentinel-errors-vs-типизированные-ошибки)
- [Оборачивание с контекстом: правила `%w`](#оборачивание-с-контекстом-правила-w)
- [Кастомные типы ошибок](#кастомные-типы-ошибок)
- [Типичные анти-паттерны](#типичные-анти-паттерны)
- [`errgroup` — параллельные задачи с первой ошибкой](#errgroup--параллельные-задачи-с-первой-ошибкой)
- [Ошибки в конкурентном коде — errCh паттерн](#ошибки-в-конкурентном-коде--errch-паттерн)
- [`errors.Join` (Go 1.20+)](#errorsjoin-go-120)
- [Итоговые правила](#итоговые-правила)
- [Разбор примеров-загадок](#разбор-примеров-загадок)
- [Interview-ready answer](#interview-ready-answer)

---

## Интерфейс `error`

```go
type error interface {
    Error() string
}
```

`error` — просто интерфейс. Это значит:
- любой тип с методом `Error() string` — ошибка;
- `nil` — отсутствие ошибки;
- ошибку можно оборачивать (wrapping) сохраняя цепочку причин.

---

## `errors.Is` и `errors.As` — механика wrapping chain

### Wrapping

`fmt.Errorf("context: %w", err)` создаёт новую ошибку, которая **оборачивает** оригинальную:

```go
var ErrNotFound = errors.New("not found")

func findUser(id int) error {
    return fmt.Errorf("findUser id=%d: %w", id, ErrNotFound)
}

func getProfile(userID int) error {
    return fmt.Errorf("getProfile: %w", findUser(userID))
}
```

Цепочка: `getProfile` → `findUser` → `ErrNotFound`

Итоговое `Error()` — это конкатенация сообщений всех уровней, разделённых `": "`. `%w` подставляет в текст результат `err.Error()` обёрнутой ошибки, поэтому строки складываются слева направо от внешнего уровня к корню:

```go
err := getProfile(42)
fmt.Println(err)
// getProfile: findUser id=42: not found
//  └─────┬───┘ └─────┬──────┘ └───┬───┘
//   getProfile    findUser    ErrNotFound
```

Каждый `fmt.Errorf("...: %w", inner)` ставит свой префикс перед текстом вложенной ошибки — отдельного разделителя `%w` не добавляет, `": "` пишется вручную в строке формата. Отсюда правило формата `"операция контекст: %w"`: читаемая трасса собирается сама, а саму цепочку (не текст) видят `errors.Is`/`As`.

### `errors.Is` — проверка по значению в цепочке

```go
err := getProfile(42)

errors.Is(err, ErrNotFound) // true — идёт вглубь цепочки через Unwrap()
err == ErrNotFound           // false — это другой объект ошибки
```

Как работает: если тип реализует `Unwrap() error`, `errors.Is` рекурсивно разматывает цепочку и проверяет `==` на каждом уровне.

#### Кастомный `Is`

**Зачем.** По умолчанию `errors.Is(err, target)` на каждом уровне цепочки делает простое `err == target` — сравнение указателей/значений. Это работает для sentinel-ошибок (`ErrNotFound` — один глобальный объект, его и сравниваем). Но если ошибка несёт **данные** (ID, код, статус), то каждый экземпляр — отдельный объект, и `==` почти всегда false: два разных `&NotFoundError{ID: 1}` не равны, даже если по смыслу это «одна и та же» категория ошибки.

`errors.Is` даёт перехватить это сравнение: если у твоего типа есть метод `Is(target error) bool`, движок вызовет **его** вместо `==`. Ты сам решаешь, что считать «совпадением».

**Пример 1 — совпадение по типу, игнорируя поля.** «Любой not-found считается not-found, ID не важен»:

```go
type NotFoundError struct{ ID int }

func (e *NotFoundError) Error() string { return fmt.Sprintf("id=%d not found", e.ID) }

func (e *NotFoundError) Is(target error) bool {
    _, ok := target.(*NotFoundError)
    return ok  // считаем любой *NotFoundError "тем же"
}
```

```go
err := fmt.Errorf("getUser: %w", &NotFoundError{ID: 42})

errors.Is(err, &NotFoundError{})       // true  — Is() сравнивает по типу, ID игнорирует
err == &NotFoundError{}                // false — разные объекты
errors.Is(err, &NotFoundError{ID: 99}) // true  — поле target тоже не смотрим
```

Здесь `target` (`&NotFoundError{}`) — это просто «эталон-маркер»: его поля не читаются, важен лишь тип. Без своего `Is` обе проверки дали бы false.

**Пример 2 — совпадение по значению поля.** «Считаем ошибки одинаковыми, если совпал HTTP-статус»:

```go
type APIError struct {
    Status  int
    Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("api %d: %s", e.Status, e.Message) }

func (e *APIError) Is(target error) bool {
    t, ok := target.(*APIError)
    return ok && e.Status == t.Status  // совпадение, если статус тот же
}

// Заранее объявленные «образцы» для проверок
var ErrRateLimited = &APIError{Status: 429}
var ErrServerDown  = &APIError{Status: 503}
```

```go
err := fmt.Errorf("fetch: %w", &APIError{Status: 429, Message: "too many requests"})

errors.Is(err, ErrRateLimited) // true  — Status совпал (429), Message не сравнивается
errors.Is(err, ErrServerDown)  // false — 429 ≠ 503
```

Теперь весь код проверяет `errors.Is(err, ErrRateLimited)` вместо того, чтобы тащить `errors.As` и руками сравнивать `ve.Status == 429`.

**Ключевой нюанс — кто над кем вызывает `Is`.** `errors.Is(err, target)` разматывает **первый аргумент** `err` по `Unwrap()` и на каждом уровне вызывает `level.Is(target)`. То есть метод `Is` определяется на типе **ошибки из цепочки**, а `target` приходит аргументом. `target.Is(...)` никогда не вызывается.

> Аналогично можно определить метод `As(target any) bool` для кастомной логики извлечения, но это нужно крайне редко — в 99% случаев хватает стандартного поведения `errors.As`.

### `errors.As` — извлечение конкретного типа

**Зачем.** `errors.Is` отвечает на вопрос «**та ли это ошибка?**» — да/нет. Но если ошибка несёт данные (`Field`, `Status`, `Code`), часто нужно не просто узнать факт, а **достать сам объект** и прочитать его поля. Это и делает `errors.As`: разматывает цепочку, находит первый уровень нужного типа и **записывает его** в твою переменную.

```go
type ValidationError struct {
    Field   string
    Message string
}
func (e *ValidationError) Error() string {
    return fmt.Sprintf("validation: %s — %s", e.Field, e.Message)
}

func validate(name string) error {
    if name == "" {
        return fmt.Errorf("validate: %w", &ValidationError{Field: "name", Message: "required"})
    }
    return nil
}

err := validate("")
var ve *ValidationError                 // 1. объявляем переменную нужного типа
if errors.As(err, &ve) {                // 2. передаём УКАЗАТЕЛЬ на неё
    fmt.Println(ve.Field, ve.Message)   // 3. ve уже заполнен: name, required
}
```

**Что происходит по шагам.** `errors.As(err, &ve)` идёт по цепочке `err` через `Unwrap()`; на каждом уровне проверяет, можно ли присвоить ошибку этого уровня в `*target` (то есть в `ve`). Нашёл `*ValidationError` — записывает его в `ve`, возвращает `true`. Не нашёл до конца цепочки — `ve` остаётся `nil`, возвращает `false`.

**Почему аргумент — указатель на указатель (`&ve`).** `ve` уже имеет тип `*ValidationError`. Чтобы `errors.As` мог **присвоить** найденную ошибку в `ve` (изменить её снаружи), ему нужен адрес самой переменной — отсюда `&ve` типа `**ValidationError`. Передашь просто `ve` — будет паника:

```go
errors.As(err, ve)   // ❌ panic: errors: target must be a non-nil pointer
errors.As(err, &ve)  // ✅
```

**`Is` vs `As` — когда что.**

```go
// Нужен только факт «это ошибка валидации?» → Is (если у типа есть кастомный Is)
if errors.Is(err, someValidationMarker) { ... }

// Нужны данные ошибки (какое поле, какое сообщение) → As
var ve *ValidationError
if errors.As(err, &ve) {
    log.Printf("поле %q невалидно: %s", ve.Field, ve.Message)
}
```

| | `errors.Is(err, target)` | `errors.As(err, &target)` |
|---|---|---|
| Вопрос | «это та ошибка?» | «есть ли в цепочке ошибка типа T?» |
| Возвращает | `bool` | `bool` + **заполняет** `target` |
| target — это | конкретное значение-эталон | указатель на переменную типа T |
| Зачем | проверить категорию | достать поля/данные |

**Пример с несколькими типами в цепочке.** `As` находит **первый подходящий** уровень, считая снаружи внутрь:

```go
err := fmt.Errorf("handler: %w",
    fmt.Errorf("db: %w", &APIError{Status: 503, Message: "down"}))

var apiErr *APIError
if errors.As(err, &apiErr) {
    fmt.Println(apiErr.Status) // 503 — нашёлся в глубине цепочки
}
```

### Unwrap для нескольких ошибок (Go 1.20+)

```go
// errors.Join создаёт ошибку с несколькими Unwrap
err := errors.Join(ErrNotFound, ErrPermission)

// Такая ошибка реализует Unwrap() []error
errors.Is(err, ErrNotFound)    // true
errors.Is(err, ErrPermission)  // true
```

---

## Sentinel errors vs типизированные ошибки

### Sentinel errors — переменные-значения

```go
var (
    ErrNotFound   = errors.New("not found")
    ErrPermission = errors.New("permission denied")
    ErrTimeout    = errors.New("timeout")
)
```

**Когда использовать:**
- ошибка — конкретное условие, дополнительный контекст не нужен;
- публичный API пакета: вызывающий сравнивает через `errors.Is`;
- примеры из stdlib: `io.EOF`, `sql.ErrNoRows`, `http.ErrNoCookie`.

**Проблема:**
```go
// Плохо: теряем контекст "какой именно объект не нашли"
return ErrNotFound

// Лучше: оборачиваем с контекстом
return fmt.Errorf("user %d: %w", id, ErrNotFound)
```

### Типизированные ошибки — кастомные типы

```go
type NotFoundError struct {
    Resource string
    ID       any
}

func (e *NotFoundError) Error() string {
    return fmt.Sprintf("%s with id %v not found", e.Resource, e.ID)
}
```

**Когда использовать:**
- нужны структурированные данные (поле, код, ID ресурса);
- вызывающий принимает разные решения в зависимости от полей ошибки;
- HTTP-обработчик маппит тип на статус-код.

```go
func handleError(err error) int {
    var nfe *NotFoundError
    if errors.As(err, &nfe) {
        return http.StatusNotFound
    }
    var ve *ValidationError
    if errors.As(err, &ve) {
        return http.StatusBadRequest
    }
    return http.StatusInternalServerError
}
```

### Сравнение

| Критерий | Sentinel | Typed |
|---|---|---|
| Сравнение | `errors.Is` | `errors.As` |
| Дополнительные данные | нет | да (поля struct) |
| Версионирование API | стабильны как переменные | структура может меняться |
| Типичный use case | io.EOF, sql.ErrNoRows | ValidationError, NotFoundError |
| Оборачивание контекста | через `%w` | через поля + `%w` |

---

## Оборачивание с контекстом: правила `%w`

### Базовый паттерн

```go
func (s *UserService) GetUser(ctx context.Context, id int) (*User, error) {
    user, err := s.repo.FindByID(ctx, id)
    if err != nil {
        // добавляем "откуда" и "что делали", сохраняем оригинал через %w
        return nil, fmt.Errorf("UserService.GetUser id=%d: %w", id, err)
    }
    return user, nil
}
```

**Правила:**
1. Используй `%w` (wrap), а не `%v` (stringify) — только `%w` сохраняет цепочку для `errors.Is`/`As`
2. Добавляй **операцию** ("doing X"), не просто пересылай ошибку
3. Добавляй **контекст** (id, имя ресурса) — то, что поможет при дебаге
4. Не начинай сообщение с заглавной буквы и не ставь точку в конце — ошибки могут складываться в цепочки

```go
// Правильно
fmt.Errorf("get user %d: %w", id, err)

// Неправильно (потеряна оригинальная ошибка)
fmt.Errorf("get user %d: %v", id, err)

// Неправильно (нет контекста)
fmt.Errorf("%w", err)

// Неправильно (стиль — заглавная буква, точка)
fmt.Errorf("Get user failed: %w.", err)
```

### `%w` vs `fmt.Errorf` без оборачивания

```go
// Оборачивает — можно проверить через errors.Is
err1 := fmt.Errorf("op: %w", io.EOF)
errors.Is(err1, io.EOF) // true

// Не оборачивает — теряем оригинал
err2 := fmt.Errorf("op: %v", io.EOF)
errors.Is(err2, io.EOF) // false
```

### Когда НЕ оборачивать

Если ошибка уже содержит нужный контекст и ты просто пробрасываешь её вверх:

```go
// Плохо — двойное оборачивание одного уровня
func (r *UserRepo) FindByID(ctx context.Context, id int) (*User, error) {
    row := r.db.QueryRowContext(ctx, query, id)
    if err := row.Scan(&u); err != nil {
        return nil, fmt.Errorf("FindByID: %w", fmt.Errorf("scan: %w", err))
    }
    return &u, nil
}

// Хорошо — каждый уровень добавляет что-то новое
func (r *UserRepo) FindByID(ctx context.Context, id int) (*User, error) {
    row := r.db.QueryRowContext(ctx, query, id)
    if err := row.Scan(&u); err != nil {
        return nil, fmt.Errorf("scan user id=%d: %w", id, err)
    }
    return &u, nil
}
```

---

## Кастомные типы ошибок

### Базовый шаблон

```go
type AppError struct {
    Code    int
    Message string
    Cause   error
}

func (e *AppError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Cause)
    }
    return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Реализуем Unwrap чтобы errors.Is/As работали через этот тип
func (e *AppError) Unwrap() error {
    return e.Cause
}
```

### Домен-специфичные ошибки

```go
// domain/errors.go

type ValidationError struct {
    Fields map[string]string // field -> message
}

func (e *ValidationError) Error() string {
    msgs := make([]string, 0, len(e.Fields))
    for field, msg := range e.Fields {
        msgs = append(msgs, field+": "+msg)
    }
    sort.Strings(msgs)
    return "validation failed: " + strings.Join(msgs, "; ")
}

func (e *ValidationError) Add(field, message string) {
    if e.Fields == nil {
        e.Fields = make(map[string]string)
    }
    e.Fields[field] = message
}

func (e *ValidationError) HasErrors() bool {
    return len(e.Fields) > 0
}

// Использование
func validateOrder(o *Order) error {
    ve := &ValidationError{}
    if o.Quantity <= 0 {
        ve.Add("quantity", "must be positive")
    }
    if o.Price.IsZero() {
        ve.Add("price", "must be set")
    }
    if ve.HasErrors() {
        return ve
    }
    return nil
}
```

---

## Типичные анти-паттерны

### 1. Проглатывание ошибки

```go
// Плохо — ошибка потеряна, программа продолжает в неопределённом состоянии
result, _ := doSomething()

// Хорошо — логируй или возвращай
result, err := doSomething()
if err != nil {
    log.Printf("doSomething failed: %v", err)
    // или return fmt.Errorf("...: %w", err)
}
```

### 2. `panic` вместо error

```go
// Плохо — паника крашит горутину (и весь сервер без recover)
func divide(a, b int) int {
    if b == 0 {
        panic("division by zero")
    }
    return a / b
}

// Хорошо — возвращаем ошибку
func divide(a, b int) (int, error) {
    if b == 0 {
        return 0, errors.New("division by zero")
    }
    return a / b, nil
}
```

**Когда `panic` допустима:**
- программирование, а не runtime: `panic("implement me")`, `panic("unreachable")`
- нарушение инварианта, которое невозможно исправить (index out of bounds в слайсе)
- инициализация приложения: `mustConnect(db)` — если нет подключения, нет смысла запускаться

```go
// Паттерн Must — для инициализации
func MustCompile(pattern string) *regexp.Regexp {
    r, err := regexp.Compile(pattern)
    if err != nil {
        panic(err)
    }
    return r
}

var emailRegex = MustCompile(`^[a-zA-Z0-9.]+@[a-zA-Z0-9.]+$`)
```

### 3. Двойной возврат / игнорирование второго значения

```go
// Плохо — проверяем ошибку, но потом используем result независимо от неё
result, err := fetch()
if err != nil {
    log.Println(err)
}
process(result) // result может быть нулевым!

// Хорошо — ранний return при ошибке
result, err := fetch()
if err != nil {
    return fmt.Errorf("fetch: %w", err)
}
process(result)
```

### 4. Создание новой ошибки вместо оборачивания

```go
// Плохо — теряем оригинальную ошибку, нельзя использовать errors.Is/As
if err != nil {
    return errors.New("database error") // информация потеряна
}

// Хорошо — оборачиваем
if err != nil {
    return fmt.Errorf("query users: %w", err)
}
```

### 5. Возврат `error` вместо конкретного типа (только для возвращаемых значений)

```go
// Антипаттерн — возвращаем конкретный тип, но присваиваем ему nil-указатель
func getError() error {
    var err *MyError = nil
    return err // НЕ nil! это error{type=*MyError, value=nil}
}

// Правильно — если нет ошибки, возвращай untyped nil
func getError() error {
    // ...
    return nil // это настоящий nil error
}
```

---

## `errgroup` — параллельные задачи с первой ошибкой

`golang.org/x/sync/errgroup` — группа горутин где первая ошибка завершает все.

```go
import "golang.org/x/sync/errgroup"

func fetchAll(ctx context.Context, ids []int) ([]*User, error) {
    g, ctx := errgroup.WithContext(ctx) // ctx отменяется при первой ошибке
    
    users := make([]*User, len(ids))
    
    for i, id := range ids {
        // i, id := i, id  // до Go 1.22 нужен был захват; с 1.22 переменные цикла per-iteration
        g.Go(func() error {
            u, err := fetchUser(ctx, id)
            if err != nil {
                return fmt.Errorf("user %d: %w", id, err)
            }
            users[i] = u
            return nil
        })
    }
    
    if err := g.Wait(); err != nil {
        return nil, err // первая ошибка из любой горутины
    }
    return users, nil
}
```

### Ограничение параллелизма через `errgroup.SetLimit`

```go
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(10) // не более 10 горутин одновременно

for _, id := range ids {  // Go 1.22+: id уже per-iteration, отдельный захват не нужен
    g.Go(func() error {
        return processUser(ctx, id)
    })
}

return g.Wait()
```

### `errgroup` vs `sync.WaitGroup`

| | `sync.WaitGroup` | `errgroup` |
|---|---|---|
| Сбор ошибок | вручную через channel/slice | автоматически, первая ошибка |
| Отмена при ошибке | вручную через cancel | автоматически через context |
| Ограничение параллелизма | нет | `SetLimit` |
| Ожидание всех | `Wait()` возвращает void | `Wait()` возвращает error |

---

## Ошибки в конкурентном коде — errCh паттерн

### Наивный паттерн `errCh chan error, 1` — и его подвох

Буферизованный канал размером 1: только первая ошибка побеждает, остальные дропаются через `default`, чтобы отправитель не залип.

```go
func fetchAll(ctx context.Context, ids []int) ([]*User, error) {
    errCh := make(chan error, 1) // буфер 1 — первая ошибка wins, остальные дропаются
    results := make(chan *User, len(ids))

    var wg sync.WaitGroup
    for _, id := range ids {  // Go 1.22+: id per-iteration, отдельный захват не нужен
        wg.Add(1)
        go func() {
            defer wg.Done()
            u, err := fetchUser(ctx, id)
            if err != nil {
                select {
                case errCh <- err: // отправляем только если канал пустой
                default:
                }
                return
            }
            results <- u
        }()
    }

    go func() { wg.Wait(); close(results) }() // закрыть results, когда все завершатся

    var users []*User
    for u := range results {
        users = append(users, u)
    }

    select {
    case err := <-errCh:
        return nil, err
    default:
        return users, nil
    }
}
```

⚠️ **Подвох: буфер 1 выбирает, какую ошибку вернуть, но НЕ останавливает работу.** Это две разные задачи, которые легко спутать:

| Задача | Решает буфер 1? |
|---|---|
| Выбрать одну ошибку для возврата наружу | ✅ да |
| Не дать горутине залипнуть на `send` | ✅ да |
| Прекратить остальные запросы при первой ошибке | ❌ **нет** |

Цикл спавнит все `len(ids)` горутин **сразу**, до того как хоть одна вернёт ошибку, и каждая безусловно вызывает `fetchUser`. `ctx` никто не отменяет. На 1000 ids с ошибкой на первом запросе:

```
ids = 1000, ошибка на id=1
├─ запущены все 1000 горутин
├─ все 1000 реально дёргают fetchUser (сеть/БД)
└─ 999 доделывают работу впустую — их результат всё равно выбросим
```

Плюс второй прод-дефект: 1000 одновременных соединений к БД/внешнему API — это перегрузка пула и самого бэкенда. Параллелизм нужно **ограничивать**.

### Почему буфер именно 1

- Буфер 0 (unbuffered): горутина блокируется на отправке, если читатель ещё не готов → **goroutine leak**.
- Буфер 1: первая ошибка записывается, остальные дропаются через `default` → отправитель не блокируется.
- Буфер N (`len(ids)`): собираем все ошибки, но обычно нужна только первая.

Ключевое: буфер 1 — про **выбор ошибки**, а прекращение работы — это **отмена контекста**. Их складывают вместе.

### Прод-вариант: отмена контекста + лимит параллелизма

Чтобы оставшиеся запросы реально прервались, нужны два механизма поверх `errCh`:

1. **`context.WithCancel` + `cancel()` при первой ошибке** — сигнал всем горутинам бросить работу. `fetchUser` обязан уважать `ctx` (выходить по `ctx.Done()`).
2. **Семафор** (`chan struct{}` с буфером N) — не больше N запросов одновременно.

Семафор и проверку отмены держим **внутри горутины**: проверка `ctx` тогда стоит прямо перед работой и не «протухает», а цикл спавна не блокируется — главная горутина сразу переходит к чтению результатов.

```go
func fetchAll(ctx context.Context, ids []int) ([]*User, error) {
    ctx, cancel := context.WithCancel(ctx)
    defer cancel() // освобождаем ресурсы ctx на ЛЮБОМ выходе (в т.ч. при success)

    errCh := make(chan error, 1)
    results := make(chan *User, len(ids))
    sem := make(chan struct{}, 16) // не больше 16 запросов к БД одновременно

    var wg sync.WaitGroup
    for _, id := range ids {
        wg.Add(1)
        go func() {
            defer wg.Done()

            // Ожидание слота — отменяемое: если cancel() уже случился,
            // не занимаем слот и не делаем работу.
            select {
            case <-ctx.Done():
                return
            case sem <- struct{}{}:
            }
            defer func() { <-sem }() // освобождаем слот

            u, err := fetchUser(ctx, id) // увидит отмену → вернёт ctx.Err() досрочно
            if err != nil {
                select {
                case errCh <- err: // первая ошибка побеждает
                    cancel()       // ← сигнал остальным: бросайте работу
                default:           // не первая — дропаем
                }
                return
            }
            results <- u
        }()
    }

    go func() { wg.Wait(); close(results) }()

    var users []*User
    for u := range results {
        users = append(users, u)
    }

    select {
    case err := <-errCh:
        return nil, err
    default:
        return users, nil
    }
}
```

Теперь на тех же 1000 ids с ошибкой на первом: в полёте максимум 16 запросов, `cancel()` обрывает их через `ctx.Done()`, а припаркованные на `sem <-` горутины просыпаются в ветке `ctx.Done()` и уходят без работы. Вместо 999 лишних вызовов — десятки в худшем случае.

Trade-off: спавним сразу все `len(ids)` горутин, и «лишние» висят припаркованными на семафоре (горутина дешёвая, ~2–8 КБ стека — для тысяч ok). Если ids **очень** много, экономичнее не плодить столько горутин, а взять [пул воркеров](../12-interview-practice/coding-tasks/concurrency/07-worker-pool-debug.md) с фиксированным числом исполнителей.

### Дефолт на практике — `errgroup`

Весь ручной обвяз из прод-варианта (`errCh` + `cancel` + семафор + `WaitGroup`) — это ровно то, что `golang.org/x/sync/errgroup` даёт из коробки. Готовый `fetchAll` на `errgroup` — в разделе [`errgroup` — параллельные задачи с первой ошибкой](#errgroup--параллельные-задачи-с-первой-ошибкой). Соответствие один-к-одному:

| Ручной `errCh`-паттерн | `errgroup` |
|---|---|
| `make(chan error, 1)` + `select/default` | первая ошибка из `g.Wait()` |
| `context.WithCancel` + `cancel()` при ошибке | `WithContext` отменяет `ctx` сам |
| семафор `chan struct{}, N` | `g.SetLimit(N)` |
| `sync.WaitGroup` + закрытие `results` | `g.Wait()` |
| `results chan *User` | запись по индексу в `make([]*User, len(ids))` |

Поэтому ручной `errCh` стоит знать для собеседования и редких случаев без зависимостей, а в проде по умолчанию — `errgroup`.

> Анти-паттерн — прокидывать ошибку через `context.WithValue`. `context` предназначен для отмены и request-scoped данных, а не для возврата результатов; ошибку так не увидит вызывающий. Используй `errgroup`.

---

## `errors.Join` (Go 1.20+)

Объединяет несколько ошибок в одну:

```go
func validateUser(u *User) error {
    var errs []error
    
    if u.Name == "" {
        errs = append(errs, errors.New("name is required"))
    }
    if u.Email == "" {
        errs = append(errs, errors.New("email is required"))
    }
    if u.Age < 0 {
        errs = append(errs, fmt.Errorf("age %d is invalid", u.Age))
    }
    
    return errors.Join(errs...) // nil если errs пустой
}
```

`errors.Join` возвращает nil если все аргументы nil.

```go
err := validateUser(u)
if err != nil {
    fmt.Println(err) // "name is required\nemail is required"
}
```

### `errors.Join` vs `fmt.Errorf` с несколькими `%w`

```go
// Go 1.20+: несколько %w в одном Errorf
err := fmt.Errorf("combined: %w and %w", err1, err2)
errors.Is(err, err1) // true
errors.Is(err, err2) // true

// vs errors.Join — без дополнительного сообщения
err := errors.Join(err1, err2)
```

---

## Итоговые правила

| Ситуация | Решение |
|---|---|
| Проверка конкретной ошибки | `errors.Is(err, ErrXxx)` |
| Извлечение данных из ошибки | `errors.As(err, &target)` |
| Простая ошибка без контекста | `errors.New("message")` |
| Ошибка с контекстом | `fmt.Errorf("op context: %w", err)` |
| Ошибка со структурированными данными | кастомный тип, реализующий `Error()` и `Unwrap()` |
| Параллельные задачи, первая ошибка | `errgroup.Group` |
| Параллельные задачи, вручную | `chan error, 1` + `select { default }` + `context.WithCancel` (буфер 1 ловит ошибку, отмену делает `cancel()`) |
| Несколько ошибок вместе | `errors.Join(errs...)` |
| Нет ошибки | `return nil` (не типизированный nil!) |

---

## Разбор примеров-загадок

### Загадка 1: typed nil в error

```go
type MyErr struct{}
func (*MyErr) Error() string { return "boom" }

func do() error {
    var e *MyErr        // nil-указатель
    return e            // возвращаем конкретный тип
}

func main() {
    if err := do(); err != nil {
        fmt.Println("есть ошибка:", err)  // ?
    } else {
        fmt.Println("ошибок нет")
    }
}
```

<details>
<summary>Ответ</summary>

```
есть ошибка: boom
```

`do()` возвращает `error` со «слотом типа» `*MyErr` (не nil) и данными nil → интерфейс ≠ nil, хотя указатель внутри nil. Ветка `err != nil` срабатывает ложно. Это та же typed-nil-ловушка, что в [03-interfaces](./03-interfaces-method-sets-and-nil.md#nil-interface-vs-typed-nil-как-это-работает). Фикс — возвращать `nil` явно, не типизированный указатель. (`fmt` тут не упал, т.к. `Error()` не разыменовывает receiver; если бы разыменовывал — была бы паника.)
</details>

---

### Загадка 2: `%w` против `%v`

```go
var ErrNotFound = errors.New("not found")

e1 := fmt.Errorf("load: %w", ErrNotFound)
e2 := fmt.Errorf("load: %v", ErrNotFound)

fmt.Println(errors.Is(e1, ErrNotFound))  // ?
fmt.Println(errors.Is(e2, ErrNotFound))  // ?
```

<details>
<summary>Ответ</summary>

```
true
false
```

`%w` **оборачивает** — сохраняет ссылку на оригинал, `errors.Is` дойдёт по `Unwrap`. `%v` лишь форматирует текст: получается новая ошибка с тем же сообщением, но без связи → `Is` возвращает false. Сообщения у `e1` и `e2` идентичны (`load: not found`) — различие только в цепочке. Поэтому для пробрасывания ошибки всегда `%w`.
</details>

---

### Загадка 3: `errors.As` требует указатель на нужный тип

```go
type AppErr struct{ Code int }
func (e *AppErr) Error() string { return "app" }

func main() {
    err := fmt.Errorf("wrap: %w", &AppErr{Code: 42})

    var ae *AppErr
    fmt.Println(errors.As(err, ae))   // (1) ?
    fmt.Println(errors.As(err, &ae))  // (2) ?
}
```

<details>
<summary>Ответ</summary>

```
(1) panic: errors: target must be a non-nil pointer
(2) true   (ae.Code == 42)
```

`errors.As` записывает найденное значение в цель → ему нужен **указатель на переменную целевого типа**. `ae` имеет тип `*AppErr`, значит передавать надо `&ae` (тип `**AppErr`). Передача самого `ae` (1) — паника. Частая ошибка: забыть `&`. Мнемоника: «`As(err, &target)` — всегда адрес».
</details>

---

### Загадка 4: `errors.Join` и nil-аргументы

```go
e := errors.Join(nil, nil)
fmt.Println(e == nil)          // ?

e2 := errors.Join(nil, errors.New("x"), nil)
fmt.Println(e2)                // ?
```

<details>
<summary>Ответ</summary>

```
true
x
```

`errors.Join` **пропускает nil**-аргументы: если все nil — возвращает `nil` (а не «пустую не-nil ошибку»). Поэтому паттерн «накопить `[]error` и `Join(errs...)`» безопасен — при отсутствии ошибок выйдет чистый `nil`. С одной не-nil ошибкой результат печатается как её сообщение; с несколькими — через `\n`.
</details>

---

### Загадка 5: сравнение обёрнутого sentinel через `==`

```go
var ErrClosed = errors.New("closed")
err := fmt.Errorf("conn: %w", ErrClosed)

fmt.Println(err == ErrClosed)            // ?
fmt.Println(errors.Is(err, ErrClosed))   // ?
```

<details>
<summary>Ответ</summary>

```
false
true
```

`==` сравнивает **конкретные объекты**: `err` — это новая `*fmt.wrapError`, не `ErrClosed`, → false. `errors.Is` разматывает цепочку `Unwrap` и находит `ErrClosed` → true. Поэтому sentinel-ошибки проверяют **только** через `errors.Is`, а не `==` (иначе одна обёртка `%w` где-то в середине — и проверка молча перестаёт работать).
</details>

---

## Interview-ready answer

**1. Как работает wrapping и зачем `%w`?**
`%w` добавляет контекст, сохраняя ссылку на оригинал. `errors.Is` рекурсивно идёт по `Unwrap()` и сравнивает с целевой ошибкой; `errors.As` — то же, но с type assertion для извлечения данных (нужен указатель на цель, `&target`). `%v` связь рвёт — `Is`/`As` перестают находить. Правило: каждый уровень добавляет `"операция контекст: %w"`.

**2. Sentinel vs typed errors — когда что?**
Sentinel (`errors.New` в переменную) — когда ошибка это «условие» без данных: `io.EOF`, `sql.ErrNoRows`; проверяют `errors.Is`. Typed (кастомный struct) — когда нужны поля для решений (HTTP-статус, валидация); извлекают `errors.As`. Sentinel сравнивают только через `Is`, не `==` (обёртка ломает `==`).

**3. Типичная typed-nil-ловушка с error?**
Вернуть из функции конкретный nil-указатель (`var e *MyErr; return e`) → интерфейс `error` становится non-nil (слот типа заполнен), и `err != nil` ложно срабатывает. Возвращать `nil` явно.

**4. Как передавать ошибки из горутин?**
`errgroup` (`golang.org/x/sync/errgroup`): группирует горутины, отменяет контекст при первой ошибке, `SetLimit` ограничивает параллелизм. Вручную — `chan error, 1` + `select { default }`, чтобы не блокироваться (буфер 1 = первая ошибка побеждает, остальные дропаются). Важный нюанс: буфер 1 лишь **выбирает** ошибку для возврата, но сам по себе НЕ останавливает остальные горутины — для этого нужен `context.WithCancel` + `cancel()` при первой ошибке, иначе на 1000 ids все 1000 запросов доедут до конца впустую.

**5. `errors.Join` vs несколько `%w`?**
`errors.Join(errs...)` объединяет список (пропуская nil; все nil → nil), реализует `Unwrap() []error` — для агрегации (валидация многих полей). Несколько `%w` в одном `fmt.Errorf` — когда нужно одно сообщение с двумя причинами. Обе работают с `errors.Is`/`As`.
