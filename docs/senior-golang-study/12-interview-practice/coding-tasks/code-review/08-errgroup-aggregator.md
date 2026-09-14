# Задача 8: агрегатор через `errgroup`

## Содержание

- [Формулировка](#формулировка)
- [Исходный код](#исходный-код)
- [Основные проблемы](#основные-проблемы)
- [Исправленное решение](#исправленное-решение)
- [Контракт ошибок](#контракт-ошибок)
- [`SetLimit` и `TryGo`](#setlimit-и-trygo)
- [Пример теста](#пример-теста)
- [Что проверить тестами](#что-проверить-тестами)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Задача проверяет параллельный сбор нескольких частей ответа. `errgroup` связывает
ожидание goroutines, первую ошибку и отмену общего контекста, но не определяет,
какие данные обязательны и можно ли вернуть частичный результат.

---

## Формулировка

Главная страница одновременно запрашивает профиль, баланс и предложения. Все три
части обязательны. После успешного сбора formatter выполняет ещё один отменяемый
шаг. При первой ошибке остальные запросы должны получить отмену.

---

## Исходный код

```go
func (s *Service) BuildHome(
    ctx context.Context,
    userID string,
) (Home, error) {
    group, ctx := errgroup.WithContext(ctx)

    var home Home
    group.Go(func() error {
        profile, err := s.profiles.Get(
            context.Background(),
            userID,
        )
        home.Profile = profile
        return err
    })

    group.Go(func() error {
        offers, err := s.offers.Get(ctx, userID)
        if err != nil {
            return nil
        }
        home.Offers = offers
        return nil
    })

    group.Go(func() error {
        balance, err := s.wallet.Get(ctx, userID)
        home.Balance = balance
        return err
    })

    if err := group.Wait(); err != nil {
        return Home{}, err
    }

    return s.formatter.Enrich(ctx, home)
}
```

---

## Основные проблемы

| Проблема | Последствие |
| --- | --- |
| Profile-запрос получает `context.Background()` | Он продолжает работу после ошибки соседа или отмены запроса |
| Offers-запрос проглатывает ошибку | Нарушается контракт обязательного результата; вызывающий код видит ложный успех |
| Ошибки возвращаются без контекста операции | По `profiles.Get: timeout` легче найти источник, чем по одному `timeout` |
| Производный `ctx` используется после `Wait` | Контекст группы уже отменён даже при `Wait() == nil`; `Enrich` получает отмену |
| Не оговорена частичная запись в `home` при ошибке | Сейчас она отбрасывается; попытка вернуть её требует отдельного API-контракта |

Запись goroutines в разные поля заранее созданной структуры допустима, если поля
не разделяют изменяемую память и структура читается только после `Wait`. Запись
нескольких goroutines в один `map`, один счётчик или один слайс через `append`
потребовала бы синхронизации.

---

## Исправленное решение

Исходный context сохраняется отдельно. Производный `groupCtx` передаётся только
параллельным операциям, а следующий шаг использует исходный `ctx`.

```go
func (s *Service) BuildHome(
    ctx context.Context,
    userID string,
) (Home, error) {
    group, groupCtx := errgroup.WithContext(ctx)

    var home Home
    group.Go(func() error {
        profile, err := s.profiles.Get(groupCtx, userID)
        if err != nil {
            return fmt.Errorf("get profile: %w", err)
        }
        home.Profile = profile
        return nil
    })

    group.Go(func() error {
        offers, err := s.offers.Get(groupCtx, userID)
        if err != nil {
            return fmt.Errorf("get offers: %w", err)
        }
        home.Offers = offers
        return nil
    })

    group.Go(func() error {
        balance, err := s.wallet.Get(groupCtx, userID)
        if err != nil {
            return fmt.Errorf("get balance: %w", err)
        }
        home.Balance = balance
        return nil
    })

    if err := group.Wait(); err != nil {
        return Home{}, err
    }

    enriched, err := s.formatter.Enrich(ctx, home)
    if err != nil {
        return Home{}, fmt.Errorf("enrich home: %w", err)
    }
    return enriched, nil
}
```

`Wait` возвращает первую ненулевую ошибку, которую вернула одна из функций.
Отмена `groupCtx` просит остальные функции завершиться, но не останавливает их
принудительно. `Get` должен сам передать context в HTTP, БД или другое
блокирующее API.

---

## Контракт ошибок

Fail-fast подходит, когда без любой части весь ответ бесполезен. Если
предложения необязательны, ошибку нельзя просто проглотить без договорённости.
Вместо этого API может вернуть страницу без предложений и одновременно записать
метрику или типизированное предупреждение.

`errgroup` сохраняет только одну ошибку. Если нужно дождаться всех независимых
проверок и вернуть каждую ошибку, полезнее обычный `sync.WaitGroup` с отдельным
сбором результатов либо функции, возвращающие по одному outcome на вход.

---

## `SetLimit` и `TryGo`

Для трёх фиксированных запросов отдельный лимит обычно не нужен. При динамическом
списке `SetLimit(n)` ограничивает число активных функций. Вызов `Go` блокируется,
пока в группе нет свободного слота; это тоже форма backpressure.

Лимит нельзя изменять, пока в группе активны функции. `TryGo` не ждёт слот: он
возвращает `false`, если лимит уже занят. Caller должен явно решить, означает ли
это отказ, синхронное выполнение или помещение задачи в очередь.

---

## Пример теста

Offers-запрос ждёт начала profile-запроса, затем возвращает причинную ошибку.
Так тест проверяет, что уже работающий сосед действительно получает отмену, а
formatter не вызывается после неуспешного `Wait`.

```go
type profilesFunc func(context.Context, string) (Profile, error)

func (function profilesFunc) Get(
    ctx context.Context,
    userID string,
) (Profile, error) {
    return function(ctx, userID)
}

type offersFunc func(context.Context, string) ([]Offer, error)

func (function offersFunc) Get(
    ctx context.Context,
    userID string,
) ([]Offer, error) {
    return function(ctx, userID)
}

type walletFunc func(context.Context, string) (int64, error)

func (function walletFunc) Get(
    ctx context.Context,
    userID string,
) (int64, error) {
    return function(ctx, userID)
}

type formatterFunc func(context.Context, Home) (Home, error)

func (function formatterFunc) Enrich(
    ctx context.Context,
    home Home,
) (Home, error) {
    return function(ctx, home)
}

func TestBuildHome_ErrorCancelsSiblings(test *testing.T) {
    profileStarted := make(chan struct{})
    profileExit := make(chan error, 1)
    dependencyErr := errors.New("offers unavailable")
    var formatterCalled atomic.Bool

    service := Service{
        profiles: profilesFunc(func(
            ctx context.Context,
            _ string,
        ) (Profile, error) {
            close(profileStarted)
            <-ctx.Done()
            profileExit <- ctx.Err()
            return Profile{}, ctx.Err()
        }),
        offers: offersFunc(func(
            _ context.Context,
            _ string,
        ) ([]Offer, error) {
            <-profileStarted
            return nil, dependencyErr
        }),
        wallet: walletFunc(func(
            ctx context.Context,
            _ string,
        ) (int64, error) {
            <-ctx.Done()
            return 0, ctx.Err()
        }),
        formatter: formatterFunc(func(
            _ context.Context,
            home Home,
        ) (Home, error) {
            formatterCalled.Store(true)
            return home, nil
        }),
    }

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    _, err := service.BuildHome(ctx, "user-42")
    if !errors.Is(err, dependencyErr) {
        test.Fatalf("error = %v, want %v", err, dependencyErr)
    }
    select {
    case profileErr := <-profileExit:
        if !errors.Is(profileErr, context.Canceled) {
            test.Fatalf("profile error = %v, want context.Canceled", profileErr)
        }
    default:
        test.Fatal("profile dependency did not observe cancellation")
    }
    if formatterCalled.Load() {
        test.Fatal("formatter was called after dependency failure")
    }
}
```

`BuildHome` возвращается только после `group.Wait`, поэтому к моменту проверки
`profileExit` все зарегистрированные функции уже завершились. Parent deadline
служит предохранителем: если sibling cancellation сломана, profile вернёт
`context.DeadlineExceeded`, и тест не примет это за ожидаемый
`context.Canceled`.

---

## Что проверить тестами

- Ошибка каждого обязательного dependency возвращается с названием операции.
- После первой ошибки остальные зависимости наблюдают отмену `groupCtx`.
- Profile-запрос не переживает отмену исходного request context.
- `Enrich` вызывается только после успеха трёх операций.
- Успешный `Wait` не приводит к передаче уже отменённого `groupCtx` в `Enrich`.
- Тесты используют каналы для фиксации начала и отмены запросов.
- Конкурентное выполнение проходит `go test -race`.

---

## Interview-ready answer

**1. Что добавляет `errgroup` поверх `WaitGroup`?**

- Ошибка — `Wait` возвращает первую ненулевую ошибку.
- Отмена — `WithContext` отменяет общий context после первой ошибки или возврата
  `Wait`.
- Ограничение — `SetLimit` может ограничить число активных функций.

**2. Почему нельзя использовать `groupCtx` после `Wait`?**

- Lifecycle — контекст группы отменяется при возврате `Wait`, включая успешный.
- Решение — последующие шаги используют исходный context или новый явно
  определённый lifecycle.

**3. Останавливает ли `errgroup` goroutines принудительно?**

- Сигнал — группа только отменяет context.
- Обязанность — функция должна наблюдать `Done` сама либо передать context во
  внешнюю операцию.

**4. Когда `errgroup` не подходит?**

- Все ошибки — если контракт требует собрать ошибки каждой независимой задачи.
- Частичный результат — если нужен outcome на каждый вход, такой контракт часто
  яснее выразить отдельным результатом задачи.

---

## Связанные материалы

- [Context patterns](../../../01-go-core/concurrency-and-performance/04-context-patterns.md)
- [Fan-In / Fan-Out](../concurrency/03-fan-in-fan-out.md)
- [`errgroup`](https://pkg.go.dev/golang.org/x/sync/errgroup)
