# pkg/errors

`github.com/pkg/errors` — ошибки со стектрейсом для Go. Существует с до-1.13 эпохи и добавляет главное, чего нет в stdlib: **stacktrace**.

## Проблема stdlib

```go
// fmt.Errorf("%w") оборачивает ошибку, но не добавляет стектрейс
err := fmt.Errorf("get user: %w", dbErr)
fmt.Println(err)     // "get user: no rows in result set"
fmt.Printf("%+v", err)  // то же — стектрейса нет
```

При логировании ошибок глубоко в стеке вызовов сложно понять, откуда она пришла.

## Что даёт pkg/errors

```go
import pkgErrors "github.com/pkg/errors"

// Создать новую ошибку со стектрейсом
err := pkgErrors.New("connection refused")

// Обернуть с сообщением + стектрейс в точке вызова
err = pkgErrors.Wrap(dbErr, "get user")

// Обернуть с форматированием
err = pkgErrors.Wrapf(dbErr, "get user id=%d", userID)

// Получить original error (без оборачивания)
pkgErrors.Cause(err)

// Вывод — %+v для стектрейса
fmt.Printf("%v", err)   // "get user: no rows in result set"
fmt.Printf("%+v", err)
// get user: no rows in result set
// main.getUser
//     /app/repository/user.go:42
// main.UserService.Create
//     /app/service/user.go:18
// main.main
//     /app/main.go:30
```

## Совместимость с errors.Is / errors.As

`pkg/errors.Wrap` совместим со stdlib:

```go
import (
    stdErrors "errors"
    pkgErrors "github.com/pkg/errors"
    "github.com/jackc/pgx/v5"
)

err := pkgErrors.Wrap(pgx.ErrNoRows, "get user")

stdErrors.Is(err, pgx.ErrNoRows)  // true — работает через Unwrap/Cause
```

**Но `pkgErrors.New` — нет:**

```go
var ErrNotFound = pkgErrors.New("not found")  // плохо
var ErrNotFound2 = pkgErrors.New("not found") // другой экземпляр

stdErrors.Is(ErrNotFound, ErrNotFound2)  // false! разные указатели
```

**Правило:** sentinel errors всегда через `errors.New` из stdlib, оборачивание — через `pkgErrors.Wrap`.

## Паттерн совместного использования

```go
import (
    stdErrors "errors"
    pkgErrors "github.com/pkg/errors"
)

// Sentinel — через stdlib
var ErrNotFound = stdErrors.New("not found")
var ErrConflict = stdErrors.New("conflict")

// Оборачивание в репозитории — через pkg/errors
func (r *UserRepo) GetByID(ctx context.Context, id int64) (User, error) {
    var u User
    err := r.pool.QueryRow(ctx, `SELECT id, email FROM users WHERE id = $1`, id).
        Scan(&u.ID, &u.Email)

    if stdErrors.Is(err, pgx.ErrNoRows) {
        return User{}, pkgErrors.Wrap(ErrNotFound, "GetByID")
    }
    if err != nil {
        return User{}, pkgErrors.Wrapf(err, "GetByID id=%d", id)
    }
    return u, nil
}

// В сервисе или хендлере — проверяем через stdlib
func (s *UserService) Get(ctx context.Context, id int64) (User, error) {
    u, err := s.repo.GetByID(ctx, id)
    if stdErrors.Is(err, ErrNotFound) {
        return User{}, ErrNotFound  // передаём выше без стектрейса (уже есть)
    }
    return u, err
}

// В логе — полный стектрейс
if err != nil {
    logger.Error("failed to get user", "error", fmt.Sprintf("%+v", err))
}
```

## pkg/errors vs stdlib в новых проектах

| | stdlib `fmt.Errorf("%w")` | `pkg/errors.Wrap` |
|---|---|---|
| Stacktrace | нет | есть |
| errors.Is/As | да | да |
| Зависимость | нет | внешняя |
| Go версия | 1.13+ | любая |

**В новых проектах на Go 1.21+** часто достаточно:
- `fmt.Errorf("context: %w", err)` — для wrapping
- `slog.Error("msg", "error", err)` — structured logging
- если нужен стектрейс — `go.uber.org/zap` с `zap.Error(err)` показывает source location в полях лога

`pkg/errors` полезен, когда:
- команда уже использует его
- нужен стектрейс явно в тексте ошибки (например, в Sentry через `%+v`)
- legacy проект до Go 1.13

## Работа с Sentry

```go
import (
    "github.com/getsentry/sentry-go"
    pkgErrors "github.com/pkg/errors"
)

err := pkgErrors.Wrapf(dbErr, "create order user=%d", userID)

// Sentry умеет читать стектрейс из pkg/errors
sentry.CaptureException(err)
// В Sentry будет виден полный стектрейс с именами функций и строками файлов
```

## Interview-ready answer

`pkg/errors` добавляет stacktrace к ошибкам — то, чего нет в stdlib. `Wrap` и `Wrapf` совместимы с `errors.Is`/`errors.As`. Правило использования: sentinel errors через `errors.New` из stdlib, оборачивание с контекстом через `pkgErrors.Wrap`. В новых проектах на Go 1.21+ можно обойтись `fmt.Errorf("%w")` + structured logging, но `pkg/errors` незаменим когда нужен читаемый stacktrace в тексте ошибки или интеграция с Sentry.
