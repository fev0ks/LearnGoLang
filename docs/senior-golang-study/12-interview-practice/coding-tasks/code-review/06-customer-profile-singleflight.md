# Задача 6: профиль клиента через `singleflight.Group`

## Содержание

- [Формулировка](#формулировка)
- [Исходный код](#исходный-код)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Основные проблемы](#основные-проблемы)
- [Как проявляются ошибки](#как-проявляются-ошибки)
- [Ловушки наивного исправления](#ловушки-наивного-исправления)
- [Исправленное решение](#исправленное-решение)
- [Контекст общей работы](#контекст-общей-работы)
- [`singleflight`, кэш и ограничение параллелизма](#singleflight-кэш-и-ограничение-параллелизма)
- [Пример теста](#пример-теста)
- [Что проверить тестами](#что-проверить-тестами)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Задача проверяет понимание дедупликации одновременно выполняющихся запросов.
`singleflight.Group` объединяет вызовы с одинаковым ключом: один из них запускает
функцию, остальные ждут и получают тот же результат. Для завершившихся вызовов
результат группа не хранит, поэтому `singleflight` не заменяет кэш.

Основная сложность находится в границах общей работы. Нужно определить, какие
запросы действительно эквивалентны, чей `context` управляет загрузкой и можно ли
безопасно передать один объект всем ожидающим.

---

## Формулировка

`ProfileService` загружает профиль клиента из внешнего сервиса и сохраняет его в
локальном кэше. Tenant здесь означает изолированного клиента платформы.
Ожидаемый контракт:

- одновременные запросы одного профиля выполняют одну внешнюю загрузку;
- профили одинаковых клиентов из разных tenants не смешиваются;
- каждый caller может прекратить своё ожидание через `context.Context`;
- отмена одного caller не прерывает работу, которую ждут другие;
- общая загрузка ограничена собственным timeout;
- получатель не может изменить кэш или результат другого caller через общий
  указатель либо общий backing array слайса.

TTL и распределённая инвалидация кэша остаются за границами задачи. Кэш живёт до
остановки процесса или явного удаления записи.

---

## Исходный код

Код компилируется и может успешно пройти последовательный happy-path тест. В нём
нет явной подсказки, каким механизмом нужно координировать одновременные cache
miss. При конкурентных запросах источник получает несколько одинаковых
загрузок, а доступ к кэшу создаёт data race.

```go
package profile

import "context"

type Profile struct {
    TenantID   string
    CustomerID string
    Segments   []string
}

type Loader interface {
    Load(
        context.Context,
        string,
        string,
    ) (*Profile, error)
}

type ProfileService struct {
    loader Loader
    cache  map[string]*Profile
}

func NewProfileService(loader Loader) *ProfileService {
    return &ProfileService{
        loader: loader,
        cache:  make(map[string]*Profile),
    }
}

func (s *ProfileService) Get(
    ctx context.Context,
    tenantID string,
    customerID string,
) (*Profile, error) {
    key := customerID

    if profile, ok := s.cache[key]; ok {
        return profile, nil
    }

    profile, err := s.loader.Load(ctx, tenantID, customerID)
    if err != nil {
        return nil, err
    }

    s.cache[key] = profile
    return profile, nil
}
```

---

## Уточняющие вопросы

1. Какие поля определяют идентичность профиля: только `customerID`, пара
   `(tenantID, customerID)` или ещё версия прав доступа и локаль?
2. Допустимы ли несколько внешних загрузок при одновременном cache miss одного
   профиля?
3. Должен ли caller отменять только своё ожидание или всю общую загрузку?
4. Как долго загрузка может продолжаться после ухода всех ожидающих?
5. Допустимо ли вернуть устаревший кэш при ошибке источника?
6. Нужно ли кэшировать `not found` и другие ошибки, и на какой срок?
7. Считается ли `Profile` неизменяемым после возврата из сервиса?
8. Достаточна ли дедупликация внутри одного процесса или она требуется между
   несколькими репликами?

В исправленном варианте ключ включает tenant и клиента. Caller отменяет только
своё ожидание, а общая загрузка получает отдельный настроенный timeout. Ошибки
не кэшируются, и каждый caller получает собственную копию профиля.

---

## Основные проблемы

| Проблема | Последствие | Исправление |
| --- | --- | --- |
| Одновременный cache miss не координируется | Наплыв из N запросов может создать N одинаковых обращений к источнику | Дедуплицировать выполняющиеся загрузки через общую `singleflight.Group` |
| Ключ содержит только `customerID` | Разные tenants читают и перезаписывают одну запись кэша | Включить в ключ все измерения идентичности и авторизации |
| Обычная `map` используется без синхронизации | Конкурентное чтение и запись создают data race и могут завершить процесс runtime-ошибкой | Защитить кэш через `RWMutex` или использовать потокобезопасный кэш |
| Возвращается общий `*Profile` | Изменение структуры или `Segments` одним caller затрагивает кэш и других callers | Хранить значение и выдавать глубокую копию изменяемых полей |
| Loader может вернуть `(nil, nil)` | В кэше появляется успешная запись без профиля, а ошибка обнаруживается далеко от источника | Проверить результат на `nil` перед сохранением |

`RWMutex` исправляет гонку на `map`, но не устраняет лишние загрузки. Если два
caller последовательно получили read lock и оба увидели miss, каждый после
освобождения lock может обратиться к источнику. Удерживать write lock на время
сетевого вызова тоже неудачно: один медленный профиль остановит обращения ко
всем остальным ключам.

---

## Как проявляются ошибки

### Одновременный cache miss создаёт наплыв

Два вызова успевают проверить кэш до завершения первой загрузки:

```text
Get A -> cache miss -> Load(tenant-1, customer-42)
Get B -> cache miss -> Load(tenant-1, customer-42)
```

Кэш начинает помогать только после записи результата. Нужна отдельная
координация промежутка между miss и этой записью:

```text
Get A --+
        +-> service.loads[tenant-1/customer-42] -> один Load
Get B --+
```

### Неполный ключ смешивает области данных

Проблема существует уже в исходном кэше и затем переносится в ключ
дедупликации:

```text
tenant-a, customer-42 -> key "customer-42"
tenant-b, customer-42 -> key "customer-42"
```

Даже последовательные запросы позволяют одному tenant получить профиль другого.
При конкурентном выполнении то же смешение происходит и на общей загрузке. Такой
дефект затрагивает не только корректность, но и изоляцию данных. Простая
конкатенация без кодирования тоже ненадёжна: пары `("ab", "c")` и
`("a", "bc")` обе дают строку `"abc"`.

### Общий указатель позволяет изменить кэш снаружи

Кэш сохраняет `*Profile` и возвращает тот же указатель каждому caller. Даже
поверхностного копирования структуры было бы недостаточно: поле `Segments`
осталось бы слайсом с общим backing array. Контракт должен потребовать
неизменяемый результат либо копирование всех изменяемых частей на границе
сервиса.

---

## Ловушки наивного исправления

После обнаружения наплыва естественно предложить `singleflight`, но само название
типа ещё не делает решение корректным. Например, следующий фрагмент почти
повторяет исходную структуру метода:

```go
func (s *ProfileService) Get(
    ctx context.Context,
    tenantID string,
    customerID string,
) (*Profile, error) {
    key := customerID

    if profile, ok := s.cache[key]; ok {
        return profile, nil
    }

    var loads singleflight.Group
    value, err, _ := loads.Do(key, func() (any, error) {
        return s.loader.Load(ctx, tenantID, customerID)
    })
    if err != nil {
        return nil, err
    }
    return value.(*Profile), nil
}
```

У такого исправления остаются самостоятельные дефекты:

| Ошибка исправления | Почему это не работает |
| --- | --- |
| `Group` создаётся внутри `Get` | Каждый вызов получает отдельную таблицу in-flight операций, поэтому запросы не встречаются |
| Ключ по-прежнему равен `customerID` | Общая загрузка смешивает tenants так же, как исходный кэш |
| Используется `Do` | Caller не может отменить только своё ожидание результата |
| В функцию передаётся request context | Deadline caller, который запустил функцию, завершает общую работу для всех |
| Кэш проверяется только до `Do` | Caller, уже увидевший miss, может начать новую загрузку сразу после удаления завершённого вызова из группы |
| Type assertion не проверяется | Изменение внутреннего типа результата превращается в panic на границе сервиса |

Локальная переменная `loads` не создаёт data race: сама группа потокобезопасна.
Ошибка состоит в слишком коротком времени жизни группы. Все конкурентные вызовы
метода должны использовать один экземпляр, принадлежащий сервису.

### Context одного caller управляет всеми

Пусть caller A начинает загрузку с deadline 50 ms, а caller B присоединяется к
тому же ключу с deadline 2 s:

```text
0 ms   A запускает общую Load с ctx A
10 ms  B присоединяется и ждёт тот же результат
50 ms  ctx A отменяется
50 ms  общая Load возвращает context deadline exceeded обоим
```

`singleflight` объединяет результат функции целиком, включая ошибку. Он сам не
объединяет deadlines ожидающих и не выбирает самый длинный из них.

### Проверка кэша только снаружи допускает второй вызов

Последовательность не требует одновременного выполнения двух загрузок:

1. A и B читают кэш и видят miss.
2. A входит в `Do`, загружает профиль, пишет кэш и завершает `Do`.
3. Завершённый вызов удаляется из внутренней таблицы `singleflight`.
4. B, который уже прошёл внешнюю проверку, входит в `Do` и запускает второй
   `Load`.

Поэтому кэш проверяется ещё раз внутри функции, защищённой `singleflight`.

---

## Исправленное решение

В примере кэш и `singleflight.Group` принадлежат сервису. Кэш хранит значения,
а на внешней границе сервис копирует структуру и слайс `Segments`.

<details>
<summary>Показать решение</summary>

```go
package profile

import (
    "context"
    "fmt"
    "sync"
    "time"

    "golang.org/x/sync/singleflight"
)

type Profile struct {
    TenantID   string
    CustomerID string
    Segments   []string
}

type Loader interface {
    Load(
        context.Context,
        string,
        string,
    ) (*Profile, error)
}

type ProfileService struct {
    loader      Loader
    loadTimeout time.Duration

    mu    sync.RWMutex
    cache map[string]Profile
    loads singleflight.Group
}

func NewProfileService(
    loader Loader,
    loadTimeout time.Duration,
) *ProfileService {
    if loadTimeout <= 0 {
        panic("load timeout must be positive")
    }

    return &ProfileService{
        loader:      loader,
        loadTimeout: loadTimeout,
        cache:       make(map[string]Profile),
    }
}

func (s *ProfileService) Get(
    ctx context.Context,
    tenantID string,
    customerID string,
) (*Profile, error) {
    key := profileKey(tenantID, customerID)

    if profile, ok := s.cached(key); ok {
        return cloneProfile(profile), nil
    }
    if err := ctx.Err(); err != nil {
        return nil, err
    }

    resultCh := s.loads.DoChan(key, func() (any, error) {
        if profile, ok := s.cached(key); ok {
            return profile, nil
        }

        workCtx, cancel := context.WithTimeout(
            context.WithoutCancel(ctx),
            s.loadTimeout,
        )
        defer cancel()

        loaded, err := s.loader.Load(workCtx, tenantID, customerID)
        if err != nil {
            return nil, fmt.Errorf("load profile: %w", err)
        }
        if loaded == nil {
            return nil, fmt.Errorf("load profile: loader returned nil profile")
        }

        profile := *cloneProfile(*loaded)
        s.store(key, profile)
        return profile, nil
    })

    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    case result := <-resultCh:
        if result.Err != nil {
            return nil, result.Err
        }

        profile, ok := result.Val.(Profile)
        if !ok {
            return nil, fmt.Errorf(
                "singleflight returned %T instead of Profile",
                result.Val,
            )
        }
        return cloneProfile(profile), nil
    }
}

func (s *ProfileService) cached(key string) (Profile, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    profile, ok := s.cache[key]
    return profile, ok
}

func (s *ProfileService) store(key string, profile Profile) {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.cache[key] = profile
}

func profileKey(tenantID string, customerID string) string {
    return fmt.Sprintf(
        "%d:%s:%d:%s",
        len(tenantID),
        tenantID,
        len(customerID),
        customerID,
    )
}

func cloneProfile(profile Profile) *Profile {
    copyOfProfile := profile
    copyOfProfile.Segments = append(
        []string(nil),
        profile.Segments...,
    )
    return &copyOfProfile
}
```

</details>

`DoChan` возвращает отдельный канал каждому caller. По контракту пакета этот
канал получает ровно один `singleflight.Result` и не закрывается. Поэтому его
читают один раз через `select`, а не через `range`.

Если caller уходит по `ctx.Done()`, общая загрузка продолжает выполняться и может
заполнить кэш для оставшихся или следующих callers. В используемой репозиторием
версии `golang.org/x/sync` v0.22.0 канал результата буферизован на один элемент,
поэтому отправка результата не блокируется из-за ушедшего caller. Размер буфера
является деталью реализации этой версии, а не отдельной гарантией API.

---

## Контекст общей работы

В reference implementation у caller и общей загрузки разные времена жизни:

| Контекст | Чем управляет | Что происходит при отмене |
| --- | --- | --- |
| `ctx` метода `Get` | Ожиданием конкретного caller | Только этот caller возвращает `ctx.Err()` |
| `workCtx` | Одной общей загрузкой профиля | Все ожидающие получают ошибку загрузки |

`context.WithoutCancel(ctx)` сохраняет значения исходного контекста, но убирает
его deadline, сигнал отмены и причину отмены. Затем `WithTimeout` задаёт конечную
жизнь общей операции. Без собственного timeout загрузка может продолжаться
бесконечно после ухода всех callers.

У подхода есть цена: если все callers отменились, работа всё равно продолжается
до завершения `Load` или `loadTimeout`. Отмена общей операции после ухода
последнего waiter требует отдельного учёта активных ожидающих и заметно усложняет
lifecycle.

Контекст, от которого создаётся общая работа, не должен неявно определять права
доступа к данным. В этом примере область авторизации выражена через `tenantID` и
включена в ключ. Если результат зависит от роли, набора разрешений, локали или
версии данных, эти измерения также входят в ключ либо загрузка не объединяется.

---

## `singleflight`, кэш и ограничение параллелизма

Эти механизмы решают разные задачи:

| Механизм | Гарантия | Чего не гарантирует |
| --- | --- | --- |
| `singleflight.Group` | Не больше одного одновременного вызова для одного ключа внутри процесса | Повторное использование завершённого результата и общий лимит разных ключей |
| Кэш | Повторное использование результата после завершения загрузки | Защиту от одновременного cache miss без дополнительной координации |
| Semaphore или worker pool | Ограничение общего числа одновременно выполняющихся работ | Дедупликацию одинаковых ключей |

Сто одновременных запросов одного ключа превращаются в одну загрузку. Сто
одновременных запросов ста разных ключей по-прежнему могут запустить сто
загрузок. Если источник выдерживает только двадцать параллельных запросов,
внутри функции `singleflight` нужен semaphore или другой общий ограничитель с
лимитом `20`.

`Forget(key)` удаляет связь ключа с текущей in-flight операцией, но не отменяет
уже запущенную функцию. Следующий `Do` или `DoChan` сможет начать вторую функцию
с тем же ключом, пока первая ещё работает. Поэтому вызов `Forget` при отмене
одного caller разрушает желаемую дедупликацию и обычно здесь не нужен.

Группа работает только в одном экземпляре процесса. При нескольких репликах
каждая выполнит по одной загрузке. Обычно этого достаточно для подавления
локального наплыва. Если источник требует глобальной координации, нужны внешний
кэш, очередь либо распределённый протокол с отдельным анализом отказов.

---

## Пример теста

Тест отменяет первого caller, пока общая загрузка заблокирована. После возврата
первого caller второй запрос должен получить результат той же загрузки. Если
request context ошибочно передан в `Loader`, первый cancel завершит загрузку и
счётчик покажет второй вызов.

```go
type loaderFunc func(
    context.Context,
    string,
    string,
) (*Profile, error)

func (function loaderFunc) Load(
    ctx context.Context,
    tenantID string,
    customerID string,
) (*Profile, error) {
    return function(ctx, tenantID, customerID)
}

func TestProfileService_CallerCancellationDoesNotCancelLoad(
    test *testing.T,
) {
    started := make(chan struct{})
    release := make(chan struct{})
    var startedOnce sync.Once
    var releaseOnce sync.Once
    var calls atomic.Int32

    releaseLoad := func() {
        releaseOnce.Do(func() { close(release) })
    }
    defer releaseLoad()

    testCtx, cancelTest := context.WithTimeout(
        context.Background(),
        2*time.Second,
    )
    defer cancelTest()

    loader := loaderFunc(func(
        ctx context.Context,
        tenantID string,
        customerID string,
    ) (*Profile, error) {
        calls.Add(1)
        startedOnce.Do(func() { close(started) })

        select {
        case <-release:
            return &Profile{
                TenantID:   tenantID,
                CustomerID: customerID,
                Segments:   []string{"active"},
            }, nil
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    })

    service := NewProfileService(loader, time.Second)
    firstCtx, cancelFirst := context.WithCancel(testCtx)
    firstResult := make(chan error, 1)

    go func() {
        _, err := service.Get(firstCtx, "tenant-1", "customer-42")
        firstResult <- err
    }()

    select {
    case <-started:
    case <-testCtx.Done():
        test.Fatal("loader did not start")
    }
    cancelFirst()
    var firstErr error
    select {
    case firstErr = <-firstResult:
    case <-testCtx.Done():
        test.Fatal("first caller did not stop")
    }
    if !errors.Is(firstErr, context.Canceled) {
        test.Fatalf("first error = %v, want context.Canceled", firstErr)
    }

    secondCtx, cancelSecond := context.WithCancel(testCtx)
    defer cancelSecond()

    type result struct {
        profile *Profile
        err     error
    }
    secondResult := make(chan result, 1)
    go func() {
        profile, err := service.Get(
            secondCtx,
            "tenant-1",
            "customer-42",
        )
        secondResult <- result{profile: profile, err: err}
    }()

    releaseLoad()
    var got result
    select {
    case got = <-secondResult:
    case <-testCtx.Done():
        test.Fatal("second caller did not receive the shared result")
    }
    if got.err != nil {
        test.Fatalf("second Get: %v", got.err)
    }
    if got.profile == nil {
        test.Fatal("second Get returned a nil profile")
    }
    if got.profile.CustomerID != "customer-42" {
        test.Fatalf("customer = %q", got.profile.CustomerID)
    }
    if calls.Load() != 1 {
        test.Fatalf("loader calls = %d, want 1", calls.Load())
    }
}
```

Общий timeout ограничивает зависание теста, но не упорядочивает goroutines.
Порядок задают `started`, отмена первого caller и `release`. Отложенный
`releaseLoad` не оставит тестовый loader заблокированным при раннем `Fatal`.

---

## Что проверить тестами

- Десять одновременно ожидающих запросов одного `(tenantID, customerID)`
  вызывают `Loader.Load` ровно один раз и получают равные значения.
- Запросы одинакового `customerID` в двух tenants запускают две загрузки и не
  смешивают результаты.
- Отмена одного waiter возвращает ему `context.Canceled`, но второй waiter
  получает успешный общий результат.
- Зависший loader завершается по `loadTimeout`, даже если caller имеет более
  длинный deadline.
- Caller с уже отменённым context не запускает новую загрузку после cache miss.
- Повторный запрос после успешной загрузки читает кэш и не вызывает loader.
- Изменение `Segments` в полученном профиле не меняет кэш и результат другого
  caller.
- Одновременные обращения проходят `go test -race`.

Тестовый loader лучше управлять каналами `started` и `release`: первый вызов
сообщает, что загрузка началась, и ждёт `release`. После присоединения остальных
callers тест освобождает loader. Так проверка не зависит от случайного
`time.Sleep`. Чтобы точно знать, что все callers дошли до `DoChan`, допустим
небольшой тестовый hook перед вызовом группы; этот hook не должен попадать в
production API.

---

## Interview-ready answer

**1. Какую главную проблему нужно увидеть в исходном коде?**

- Cache stampede — несколько callers одновременно видят miss и независимо
  вызывают медленный источник с одним набором аргументов.
- Координация — mutex вокруг отдельных операций с `map` не объединяет участок
  от чтения кэша до загрузки и записи результата.
- Решение — одновременно выполняющиеся загрузки одного ключа нужно
  дедуплицировать.

**2. Почему `singleflight.Group` должна быть полем сервиса?**

- Область дедупликации — вызовы могут встретиться только в одном экземпляре
  `Group`.
- Локальная группа — новый экземпляр внутри метода ничего не объединяет между
  конкурентными вызовами метода.

**3. Что обязательно входит в ключ?**

- Идентичность — все параметры, от которых зависит результат.
- Изоляция — tenant, права доступа или версия входят в ключ, если они меняют
  видимые данные.
- Кодирование — составной ключ строится без неоднозначной конкатенации.

**4. Чем различаются `Do` и `DoChan`?**

- `Do` — блокирует caller до завершения функции и не принимает context.
- `DoChan` — позволяет ждать результат и `ctx.Done()` через `select`.
- Канал — получает один результат и по контракту не закрывается.

**5. Как разделить отмену caller и общей работы?**

- Caller context — прекращает только ожидание конкретного запроса.
- Общий context — не зависит от отмены одного caller и имеет собственный
  конечный timeout.
- Цена — после ухода всех ожидающих загрузка может выполняться до своего
  timeout.

**6. Почему `singleflight` не заменяет кэш и semaphore?**

- Кэш — хранит результат после завершения вызова; `singleflight` удаляет
  завершённую операцию.
- Semaphore — ограничивает параллелизм разных ключей; `singleflight` объединяет
  только одинаковые ключи.

**7. Что означает `shared` в результате?**

- Семантика — результат был передан нескольким callers; `shared` может быть
  `true` и у caller, фактически выполнившего функцию.
- Владение — общий указатель или слайс нельзя безопасно изменять без копии либо
  контракта неизменяемости.

**8. Что делает `Forget`?**

- Удаление — будущий вызов больше не присоединяется к текущей операции по ключу.
- Ограничение — уже запущенная функция не отменяется, поэтому две функции с
  одним ключом могут выполняться одновременно.

---

## Связанные материалы

- [Singleflight](../concurrency/06-singleflight.md) — API, внутренняя механика и
  дополнительные примеры.
- [Fetcher с кэшем](./01-fetcher-with-cache.md) — worker pool, cache stampede и
  завершение конечного batch.
- [Context patterns](../../../01-go-core/concurrency-and-performance/04-context-patterns.md)
  — распространение отмены и deadline.
- [`golang.org/x/sync/singleflight`](https://pkg.go.dev/golang.org/x/sync/singleflight)
  — официальный контракт `Group`, `Do`, `DoChan` и `Forget`.
