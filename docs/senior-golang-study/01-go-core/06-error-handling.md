# Error Handling

Обработка ошибок — одна из самых часто задаваемых тем на senior Go собеседованиях. Не из-за синтаксиса, а из-за принятия решений: когда sentinel, когда тип, когда паника, как передавать через goroutines.

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

### `errors.Is` — проверка по значению в цепочке

```go
err := getProfile(42)

errors.Is(err, ErrNotFound) // true — идёт вглубь цепочки через Unwrap()
err == ErrNotFound           // false — это другой объект ошибки
```

Как работает: если тип реализует `Unwrap() error`, `errors.Is` рекурсивно разматывает цепочку и проверяет `==` на каждом уровне.

Кастомный `Is`:
```go
type NotFoundError struct{ ID int }

func (e *NotFoundError) Is(target error) bool {
    _, ok := target.(*NotFoundError)
    return ok  // считаем любой *NotFoundError "тем же" типом
}
```

### `errors.As` — извлечение конкретного типа

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
var ve *ValidationError
if errors.As(err, &ve) {
    fmt.Println(ve.Field, ve.Message) // name, required
}
```

`errors.As` разматывает цепочку и делает type assertion. Аргумент — **указатель на целевой тип**.

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
        i, id := i, id // захват переменных цикла
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

for _, id := range ids {
    id := id
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

### Паттерн `errCh chan error, 1`

Буферизованный канал размером 1: только первая ошибка побеждает.

```go
func fetchAll(ctx context.Context, ids []int) ([]*User, error) {
    errCh := make(chan error, 1) // буфер 1 — первая ошибка wins, остальные дропаются
    results := make(chan *User, len(ids))
    
    var wg sync.WaitGroup
    for _, id := range ids {
        wg.Add(1)
        id := id
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
    
    // Закрываем results когда все горутины завершились
    go func() {
        wg.Wait()
        close(results)
    }()
    
    // Читаем результаты
    var users []*User
    for u := range results {
        users = append(users, u)
    }
    
    // Проверяем была ли ошибка
    select {
    case err := <-errCh:
        return nil, err
    default:
        return users, nil
    }
}
```

### Почему буфер именно 1

```
errCh := make(chan error, 1)
```

- Если буфер 0 (unbuffered): горутина заблокируется на отправке, если никто не читает → **goroutine leak**
- Если буфер 1: первая ошибка записывается, остальные дропаются через `default` → не блокируется
- Если буфер N (len(ids)): собираем все ошибки, но обычно нужна только первая

### Альтернатива: ошибка через context

```go
type errKey struct{}

func withError(ctx context.Context, err error) context.Context {
    return context.WithValue(ctx, errKey{}, err)
}

func errorFrom(ctx context.Context) error {
    if err, ok := ctx.Value(errKey{}).(error); ok {
        return err
    }
    return nil
}
```

Обычно используют `errgroup` вместо этого — он делает то же самое чище.

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
| Параллельные задачи, вручную | `chan error, 1` + `select { default }` |
| Несколько ошибок вместе | `errors.Join(errs...)` |
| Нет ошибки | `return nil` (не типизированный nil!) |

---

## Interview-ready answer

**Q: Как в Go работает wrapping ошибок и зачем?**

Оборачивание (`%w`) добавляет контекст к ошибке без потери оригинала. `errors.Is` рекурсивно разматывает цепочку Unwrap-вызовов и сравнивает с целевой ошибкой. `errors.As` делает то же самое, но с type assertion для извлечения данных.

Правило: каждый уровень стека добавляет `"операция контекст: %w"` — это позволяет прочитать полный путь выполнения из одного сообщения об ошибке.

**Q: Sentinel errors vs typed errors — когда что?**

Sentinel — когда ошибка это просто "условие" без дополнительных данных: `io.EOF`, `sql.ErrNoRows`. Typed — когда нужна структурированная информация для принятия решений: HTTP-маппинг на статус-коды, поля для валидации ответа.

**Q: Как передавать ошибки из горутин?**

Стандартный паттерн — `errgroup` из `golang.org/x/sync/errgroup`: группирует горутины, автоматически отменяет контекст при первой ошибке. Для ручного управления — `chan error, 1` с `select { default }` чтобы не блокироваться.
