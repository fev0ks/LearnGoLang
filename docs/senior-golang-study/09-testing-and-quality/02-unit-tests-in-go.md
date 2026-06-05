# Unit Tests в Go

Unit test проверяет один логический сценарий быстро, детерминированно и без внешних зависимостей.

## Содержание

- [Table-driven tests](#table-driven-tests)
- [t.Run — subtests](#trun--subtests)
- [t.Parallel — параллельные тесты](#tparallel--параллельные-тесты)
- [t.Helper — корректные номера строк](#thelper--корректные-номера-строк)
- [t.Cleanup — очистка ресурсов](#tcleanup--очистка-ресурсов)
- [t.Setenv — переменные окружения](#tsetenv--переменные-окружения)
- [t.TempDir — временные файлы](#ttempdir--временные-файлы)
- [Тестирование ошибок](#тестирование-ошибок)
- [Тестирование времени и случайности](#тестирование-времени-и-случайности)
- [Golden files](#golden-files)
- [testdata/ директория](#testdata-директория)
- [Что тестировать в первую очередь](#что-тестировать-в-первую-очередь)

---

## Table-driven tests

Стандартный стиль в Go. Компактно, легко добавить кейс, хорошо читается в ревью.

```go
func TestCalculateDiscount(t *testing.T) {
    tests := []struct {
        name          string
        orderTotal    float64
        customerTier  string
        wantDiscount  float64
        wantErr       bool
    }{
        {
            name:         "gold customer above threshold",
            orderTotal:   1000,
            customerTier: "gold",
            wantDiscount: 0.15,
        },
        {
            name:         "silver customer below threshold",
            orderTotal:   50,
            customerTier: "silver",
            wantDiscount: 0,
        },
        {
            name:         "unknown tier",
            orderTotal:   500,
            customerTier: "unknown",
            wantErr:      true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := CalculateDiscount(tt.orderTotal, tt.customerTier)
            if tt.wantErr {
                if err == nil {
                    t.Fatal("expected error, got nil")
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if got != tt.wantDiscount {
                t.Errorf("got %v, want %v", got, tt.wantDiscount)
            }
        })
    }
}
```

Когда table-driven **не нужен**:
- тест один и специфичный — проще линейно
- каждый кейс требует разного setup — структура перегружается

---

## t.Run — subtests

`t.Run` создаёт именованный подтест. Можно запускать отдельно:

```bash
go test -run TestCalculateDiscount/gold_customer_above_threshold
```

Subtests полезны и без table-driven — для группировки связанных проверок:

```go
func TestOrderService_Create(t *testing.T) {
    svc := newTestService(t)

    t.Run("creates order with correct total", func(t *testing.T) {
        order, err := svc.Create(ctx, items)
        // ...
    })

    t.Run("publishes OrderCreated event", func(t *testing.T) {
        // другой аспект того же сценария
    })

    t.Run("returns error when inventory empty", func(t *testing.T) {
        // error path
    })
}
```

---

## t.Parallel — параллельные тесты

`t.Parallel()` позволяет тестам в пакете выполняться параллельно. Ускоряет suite если тесты независимы.

```go
func TestNormalizePhone(t *testing.T) {
    t.Parallel()   // этот тест идёт параллельно с другими Parallel-тестами

    tests := []struct{ ... }{ ... }

    for _, tt := range tests {
        tt := tt  // захватить переменную (до Go 1.22 обязательно!)
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()  // subtests тоже параллельны
            // ...
        })
    }
}
```

**Когда НЕ использовать `t.Parallel()`:**
- тест меняет глобальное состояние (переменные окружения, глобальные переменные)
- тест использует общий ресурс (один DB, один порт)
- subtests в table-driven теле меняют общее состояние

---

## t.Helper — корректные номера строк

Без `t.Helper()` в failure message указывается строка внутри helper-функции, а не в тесте.

```go
// Без t.Helper() — бесполезный stack trace
func assertEqual(t *testing.T, got, want string) {
    if got != want {
        t.Errorf("got %q, want %q", got, want)  // ← строка внутри helper
    }
}

// С t.Helper() — показывает строку вызова в тесте
func assertEqual(t *testing.T, got, want string) {
    t.Helper()  // ← добавить в каждую helper-функцию
    if got != want {
        t.Errorf("got %q, want %q", got, want)
    }
}

// Теперь ошибка покажет строку в TestXxx, где вызван assertEqual
```

Правило: любая функция которая вызывает `t.Errorf/Fatal/...` — должна начинаться с `t.Helper()`.

---

## t.Cleanup — очистка ресурсов

`t.Cleanup` регистрирует функцию которая выполнится после теста (в т.ч. при провале). Замена `defer` с лучшей семантикой в тестах.

```go
func TestWithTempDB(t *testing.T) {
    db := openTestDB(t)
    t.Cleanup(func() {
        db.Close()
    })
    // ...
}

// Удобно инкапсулировать в helper
func openTestDB(t *testing.T) *sql.DB {
    t.Helper()
    db, err := sql.Open("pgx", testDSN)
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { db.Close() })
    return db
}

// Вызывающий тест не думает об очистке
func TestUserRepo(t *testing.T) {
    db := openTestDB(t)  // cleanup зарегистрирован внутри
    repo := NewRepository(db)
    // ...
}
```

---

## t.Setenv — переменные окружения

`t.Setenv` устанавливает env var и автоматически восстанавливает исходное значение после теста. Безопасно при `t.Parallel()` в рамках одного теста.

```go
func TestConfigFromEnv(t *testing.T) {
    t.Setenv("DB_HOST", "localhost")
    t.Setenv("DB_PORT", "5432")
    t.Setenv("LOG_LEVEL", "debug")

    cfg, err := LoadConfig()
    // cfg.DBHost == "localhost", cfg.LogLevel == "debug"
    // После теста значения автоматически восстановятся
}
```

---

## t.TempDir — временные файлы

`t.TempDir()` создаёт временную директорию и удаляет её после теста.

```go
func TestFileProcessor(t *testing.T) {
    dir := t.TempDir()  // удалится вместе со всем содержимым после теста

    input := filepath.Join(dir, "input.csv")
    if err := os.WriteFile(input, []byte("a,b,c\n1,2,3"), 0644); err != nil {
        t.Fatal(err)
    }

    output := filepath.Join(dir, "output.json")
    if err := Process(input, output); err != nil {
        t.Fatalf("Process: %v", err)
    }

    got, err := os.ReadFile(output)
    // проверить содержимое output
}
```

---

## Тестирование ошибок

**Проверять через `errors.Is` и `errors.As`**, а не через строку.

```go
var (
    ErrNotFound   = errors.New("not found")
    ErrPermission = errors.New("permission denied")
)

func TestGetUser_NotFound(t *testing.T) {
    repo := &fakeRepo{err: ErrNotFound}
    svc := NewService(repo)

    _, err := svc.GetUser(ctx, "missing-id")

    if !errors.Is(err, ErrNotFound) {
        t.Errorf("got %v, want ErrNotFound", err)
    }
}

// Проверка через errors.As — когда нужны поля ошибки
type ValidationError struct {
    Field   string
    Message string
}

func TestCreateUser_ValidationError(t *testing.T) {
    _, err := svc.CreateUser(ctx, CreateUserRequest{Email: "bad-email"})

    var ve *ValidationError
    if !errors.As(err, &ve) {
        t.Fatalf("expected ValidationError, got %T: %v", err, err)
    }
    if ve.Field != "email" {
        t.Errorf("got field %q, want %q", ve.Field, "email")
    }
}
```

**Никогда не сравнивать строку ошибки:**
```go
// Плохо — ломается при любом изменении текста
if err.Error() != "user not found" { ... }

// Хорошо — работает через wrapping
if !errors.Is(err, ErrNotFound) { ... }
```

---

## Тестирование времени и случайности

Детерминированные тесты требуют контроля над временем и случайностью.

### Инъекция часов через интерфейс

```go
type Clock interface {
    Now() time.Time
}

type realClock struct{}
func (realClock) Now() time.Time { return time.Now() }

type fakeClock struct{ t time.Time }
func (f *fakeClock) Now() time.Time { return f.t }
func (f *fakeClock) Advance(d time.Duration) { f.t = f.t.Add(d) }

type TokenService struct {
    clock Clock
    ttl   time.Duration
}

func (s *TokenService) IsExpired(token Token) bool {
    return s.clock.Now().After(token.ExpiresAt)
}

// Тест
func TestTokenService_IsExpired(t *testing.T) {
    clock := &fakeClock{t: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)}
    svc := &TokenService{clock: clock, ttl: time.Hour}

    token := Token{ExpiresAt: clock.Now().Add(30 * time.Minute)}
    if svc.IsExpired(token) {
        t.Error("token should not be expired")
    }

    clock.Advance(2 * time.Hour)  // промотать время вперёд
    if !svc.IsExpired(token) {
        t.Error("token should be expired")
    }
}
```

### Инъекция random source

```go
type IDGenerator struct {
    rand *rand.Rand  // не math/rand глобальный, а конкретный source
}

func (g *IDGenerator) Generate() string {
    return fmt.Sprintf("%016x", g.rand.Int63())
}

// В продакшне: rand.New(rand.NewSource(time.Now().UnixNano()))
// В тесте:     rand.New(rand.NewSource(42)) — детерминировано
```

---

## Golden files

Golden files хранят эталонный вывод в файлах. Полезны когда результат большой и текстовый (JSON, SQL, шаблоны, codegen).

```go
func TestRenderInvoice(t *testing.T) {
    invoice := Invoice{ID: "INV-001", Amount: 1500}
    got := RenderInvoice(invoice)

    goldenFile := filepath.Join("testdata", "invoice.golden")

    if *update {  // go test -run TestRenderInvoice -update
        os.WriteFile(goldenFile, []byte(got), 0644)
        return
    }

    want, err := os.ReadFile(goldenFile)
    if err != nil {
        t.Fatalf("read golden file: %v", err)
    }
    if string(want) != got {
        t.Errorf("output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
    }
}

var update = flag.Bool("update", false, "update golden files")
```

**Опасности golden files:**
- обновляют механически (`-update`) без проверки что изменилось
- команда доверяет golden без понимания что там написано
- diff трудно интерпретировать при реальных регрессиях

Golden файлы хороши для стабильных snapshot'ов. Плохи для покрытия бизнес-логики.

---

## testdata/ директория

Стандартное место для вспомогательных файлов тестов. Go игнорирует её при сборке.

```
user/
├── service.go
├── service_test.go
└── testdata/
    ├── valid_request.json
    ├── invalid_request.json
    └── fixtures/
        └── users.sql
```

```go
func TestParseUserRequest(t *testing.T) {
    data, err := os.ReadFile("testdata/valid_request.json")
    if err != nil {
        t.Fatal(err)
    }
    var req CreateUserRequest
    if err := json.Unmarshal(data, &req); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    // ...
}
```

---

## Что тестировать в первую очередь

```
1. Самый критичный happy path
2. Главный error path (например, NotFound, ValidationError)
3. Граничные условия (пустые входы, нули, максимумы)
4. Regression test на уже найденный баг
5. Сложный edge case в алгоритме
```

Признаки что unit test написан хорошо:
- название объясняет что тестируется и при каком условии
- тест падает именно когда поведение сломано, а не при рефакторинге
- failure message сразу объясняет что пошло не так
- тест запускается < 10ms
