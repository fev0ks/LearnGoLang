# Задача 5: оформление бронирования

## Содержание

- [Формулировка](#формулировка)
- [Исходный код](#исходный-код)
- [Основные проблемы](#основные-проблемы)
- [Исправленное решение](#исправленное-решение)
- [Пример теста](#пример-теста)
- [Что проверить тестами](#что-проверить-тестами)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Задача проверяет работу с `defer`, семантику `break` и организацию конечного
retry без бесконечного цикла.

---

## Формулировка

`OrderService` блокирует пользователя и вызывает сервис бронирования. Временные
ошибки разрешено повторять. Нужно найти проблемы и исправить код.

---

## Исходный код

```go
type OrderService struct {
    BookingService BookingService
    UserService    UserService
}

type UserService interface {
    LockUser(User) error
    UnlockUser(User) error
}

type User struct {
    ID string
}

type Receipt struct {
    ID          string
    BookingCode string
    BookedAt    string
}

type BookingService interface {
    BookFlight() (string, *BookingServiceError)
}

type BookingServiceError struct {
    error
    TryAgain bool
}

func (s *OrderService) HandleBookingOrder(user User) *Receipt {
    receipt := Receipt{ID: uuid.New().String()}

    if err := s.UserService.LockUser(user); err != nil {
        log.Logger.Err(err)
        return nil
    }

    for {
        bookingCode, err := s.BookingService.BookFlight()

        switch {
        case err == nil:
            receipt.BookedAt = time.Now().Format(time.RFC3339)
            receipt.BookingCode = bookingCode
            return &receipt
        case err.TryAgain:
        default:
            log.Logger.Err(err)
            break
        }
    }
}
```

---

## Основные проблемы

| Проблема | Последствие |
| --- | --- |
| Нет `UnlockUser` | После успешного lock пользователь останется заблокированным |
| `break` находится внутри `switch` | Он не завершает `for`, поэтому постоянная ошибка зацикливает метод |
| `TryAgain` не имеет лимита и паузы | Возникает tight loop, нагружающий CPU и сервис бронирования |
| Нет `context.Context` | Нельзя отменить lock, бронирование или ожидание retry |
| Метод возвращает только `*Receipt` | Причина ошибки теряется |
| Ошибки логируются внутри сервиса | Вызывающий код не может решить, как их обработать |
| У повторов нет стабильного ID операции | Retry может создать повторное бронирование |

`BookedAt` удобнее хранить как `time.Time`, а форматировать уже на границе API.

---

## Исправленное решение

`orderID` создаётся один раз выше по стеку и передаётся во все попытки. Сервис
бронирования должен использовать его как ключ идемпотентности.

<details>
<summary>Показать решение</summary>

```go
package booking

import (
    "context"
    "errors"
    "fmt"
    "time"
)

type OrderService struct {
    BookingService BookingService
    UserService    UserService
}

type UserService interface {
    LockUser(context.Context, User) error
    UnlockUser(context.Context, User) error
}

type BookingService interface {
    BookFlight(context.Context, string) (string, error)
}

type User struct {
    ID string
}

type Receipt struct {
    ID          string
    BookingCode string
    BookedAt    time.Time
}

type BookingServiceError struct {
    Message  string
    TryAgain bool
}

func (e *BookingServiceError) Error() string {
    return e.Message
}

func (s *OrderService) HandleBookingOrder(
    ctx context.Context,
    user User,
    orderID string,
) (receipt *Receipt, err error) {
    if err := s.UserService.LockUser(ctx, user); err != nil {
        return nil, fmt.Errorf("lock user: %w", err)
    }

    defer func() {
        unlockCtx, cancel := context.WithTimeout(
            context.WithoutCancel(ctx),
            time.Second,
        )
        defer cancel()

        if unlockErr := s.UserService.UnlockUser(unlockCtx, user); unlockErr != nil {
            err = errors.Join(err, fmt.Errorf("unlock user: %w", unlockErr))
        }
    }()

    const (
        maxAttempts = 3
        retryDelay  = 100 * time.Millisecond
    )

    var lastErr error
    for attempt := 1; attempt <= maxAttempts; attempt++ {
        bookingCode, bookingErr := s.BookingService.BookFlight(ctx, orderID)
        if bookingErr == nil {
            return &Receipt{
                ID:          orderID,
                BookingCode: bookingCode,
                BookedAt:    time.Now().UTC(),
            }, nil
        }
        lastErr = bookingErr

        var serviceErr *BookingServiceError
        if !errors.As(bookingErr, &serviceErr) || !serviceErr.TryAgain {
            return nil, fmt.Errorf("book flight: %w", bookingErr)
        }
        if attempt == maxAttempts {
            break
        }

        timer := time.NewTimer(retryDelay)
        select {
        case <-ctx.Done():
            timer.Stop()
            return nil, ctx.Err()
        case <-timer.C:
        }
    }

    return nil, fmt.Errorf(
        "book flight after %d attempts: %w",
        maxAttempts,
        lastErr,
    )
}
```

</details>

`defer` регистрируется сразу после успешного lock, поэтому unlock выполняется
при любом последующем `return`. Если unlock тоже завершится ошибкой,
`errors.Join` не даст потерять основную ошибку бронирования.

---

## Пример теста

Стабы ниже выполняются синхронно, поэтому обычных счётчиков достаточно. Тест
проверяет два связанных инварианта: retry использует прежний `orderID`, а после
успешного lock всегда выполняется ровно один unlock.

```go
type bookingServiceFunc func(
    context.Context,
    string,
) (string, error)

func (function bookingServiceFunc) BookFlight(
    ctx context.Context,
    orderID string,
) (string, error) {
    return function(ctx, orderID)
}

type userServiceStub struct {
    lock   func(context.Context, User) error
    unlock func(context.Context, User) error
}

func (service userServiceStub) LockUser(
    ctx context.Context,
    user User,
) error {
    return service.lock(ctx, user)
}

func (service userServiceStub) UnlockUser(
    ctx context.Context,
    user User,
) error {
    return service.unlock(ctx, user)
}

func TestHandleBookingOrder_RetriesAndUnlocks(test *testing.T) {
    const orderID = "order-42"
    var lockCalls, unlockCalls int
    var attemptedIDs []string

    service := OrderService{
        UserService: userServiceStub{
            lock: func(context.Context, User) error {
                lockCalls++
                return nil
            },
            unlock: func(ctx context.Context, _ User) error {
                unlockCalls++
                if err := ctx.Err(); err != nil {
                    test.Fatalf("unlock context: %v", err)
                }
                return nil
            },
        },
        BookingService: bookingServiceFunc(func(
            _ context.Context,
            gotOrderID string,
        ) (string, error) {
            attemptedIDs = append(attemptedIDs, gotOrderID)
            if len(attemptedIDs) == 1 {
                return "", &BookingServiceError{
                    Message:  "temporary failure",
                    TryAgain: true,
                }
            }
            return "BOOK-7", nil
        }),
    }

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    receipt, err := service.HandleBookingOrder(
        ctx,
        User{ID: "user-7"},
        orderID,
    )
    if err != nil {
        test.Fatalf("HandleBookingOrder: %v", err)
    }
    if receipt.ID != orderID || receipt.BookingCode != "BOOK-7" {
        test.Fatalf("receipt = %#v", receipt)
    }
    if lockCalls != 1 || unlockCalls != 1 {
        test.Fatalf(
            "lock calls = %d, unlock calls = %d",
            lockCalls,
            unlockCalls,
        )
    }
    if !slices.Equal(attemptedIDs, []string{orderID, orderID}) {
        test.Fatalf("attempted ids = %v", attemptedIDs)
    }
}
```

Тест проходит одну реальную задержку `retryDelay`. Для большого набора retry-
сценариев ожидание стоит вынести из сервиса в интерфейс `Sleeper` или функцию и
в тесте заменить управляемым каналом. Тогда можно отдельно проверить backoff и
отмену, не замедляя suite.

---

## Что проверить тестами

- При ошибке lock бронирование и unlock не вызываются.
- После успеха unlock вызывается ровно один раз.
- Временная ошибка приводит максимум к трём попыткам с одним `orderID`.
- Постоянная ошибка возвращается после первой попытки.
- Отмена context прерывает бронирование или ожидание следующей попытки.
- Ошибка unlock не теряется.

---

## Interview-ready answer

### 1. Почему исходный цикл бесконечный?

- `TryAgain` — пустая ветка сразу начинает следующую итерацию.
- `break` — завершает только `switch`, а не внешний `for`.

### 2. Где вызывать `UnlockUser`?

- После lock — сразу зарегистрировать `defer`.
- Ошибка — результат unlock нельзя молча игнорировать.

### 3. Каким должен быть retry?

- Лимит — конечное число попыток.
- Пауза — задержка между попытками должна прерываться через context.
- Фильтр — повторяются только ошибки с `TryAgain`.
- Идемпотентность — все попытки используют один `orderID`.

### 4. Почему нужен `error` в результате?

- Причина — `nil` вместо чека не объясняет, что произошло.
- Решение — вызывающий код сам определяет способ обработки и место логирования.

---

## Связанные материалы

- [HTTP-клиент платёжного сервиса](./04-payment-http-client.md)
- [Retry с backoff](../system-primitives/02-retry-with-backoff.md)
- [Idempotency](../../../05-system-design/reliability-patterns/06-idempotency.md)
