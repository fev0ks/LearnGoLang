# Задача 12: pipeline с ранним выходом

## Содержание

- [Формулировка](#формулировка)
- [Исходный код](#исходный-код)
- [Основные проблемы](#основные-проблемы)
- [Исправленное решение](#исправленное-решение)
- [Почему закрытие каналов корректно](#почему-закрытие-каналов-корректно)
- [Пример теста](#пример-теста)
- [Что проверить тестами](#что-проверить-тестами)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Задача проверяет завершение многоступенчатого pipeline. Если downstream перестал
читать, upstream должен получить отмену; иначе корректный ранний `return`
потребителя оставляет producers заблокированными на send.

---

## Формулировка

Pipeline получает идентификаторы, параллельно загружает объекты и последовательно
сохраняет их. Любая ошибка прекращает весь flow. Число workers фиксировано, все
goroutines должны завершаться до возврата функции.

---

## Исходный код

```go
func generate(ids []string) <-chan string {
    output := make(chan string)
    go func() {
        defer close(output)
        for _, id := range ids {
            output <- id
        }
    }()
    return output
}

func load(
    ctx context.Context,
    ids <-chan string,
    workers int,
    repository Repository,
) <-chan Result {
    output := make(chan Result)

    var wg sync.WaitGroup
    for workerID := 0; workerID < workers; workerID++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for id := range ids {
                item, err := repository.Get(ctx, id)
                output <- Result{Item: item, Err: err}
                if err != nil {
                    return
                }
            }
        }()
    }

    go func() {
        wg.Wait()
        close(output)
    }()
    return output
}

func Run(
    ctx context.Context,
    ids []string,
    repository Repository,
    storage Storage,
) error {
    for result := range load(ctx, generate(ids), 4, repository) {
        if result.Err != nil {
            return result.Err
        }
        if err := storage.Save(ctx, result.Item); err != nil {
            return err
        }
    }
    return nil
}
```

---

## Основные проблемы

| Проблема | Последствие |
| --- | --- |
| `generate` отправляет без context | После выхода loaders generator может навсегда зависнуть на send |
| Workers отправляют без context | После раннего `return` из `Run` они блокируются на `output <- Result` |
| Ошибка одного worker не отменяет соседей | Остальные продолжают загрузку и пытаются отправлять результаты |
| Ошибка `Save` не достигает upstream | Generator и workers не знают, что consumer ушёл |
| Функция возвращается до завершения pipeline | Утечки переживают логический запрос и удерживают входные данные |
| Значение `workers` зашито в вызове | Контракт не объясняет ёмкость и не проверяет нулевой limit |

Закрытие `output` отдельной goroutine после `wg.Wait` само по себе корректно:
закрывающая сторона знает, что все senders завершились. Проблема находится в
том, что workers могут никогда не дойти до `Done`.

---

## Исправленное решение

Все стадии входят в одну `errgroup` и используют её context на каждой
потенциальной блокировке. Отдельный `WaitGroup` workers позволяет закрыть канал
результатов после последнего sender.

```go
func Run(
    ctx context.Context,
    ids []string,
    workers int,
    repository Repository,
    storage Storage,
) error {
    if workers <= 0 {
        return fmt.Errorf("workers must be positive")
    }

    jobs := make(chan string)
    results := make(chan Item)
    group, groupCtx := errgroup.WithContext(ctx)

    group.Go(func() error {
        defer close(jobs)
        for _, id := range ids {
            select {
            case jobs <- id:
            case <-groupCtx.Done():
                return groupCtx.Err()
            }
        }
        return nil
    })

    var workersWG sync.WaitGroup
    for workerID := 0; workerID < workers; workerID++ {
        workersWG.Add(1)
        group.Go(func() error {
            defer workersWG.Done()
            for {
                select {
                case <-groupCtx.Done():
                    return groupCtx.Err()
                case id, ok := <-jobs:
                    if !ok {
                        return nil
                    }

                    item, err := repository.Get(groupCtx, id)
                    if err != nil {
                        return fmt.Errorf("load %q: %w", id, err)
                    }

                    select {
                    case results <- item:
                    case <-groupCtx.Done():
                        return groupCtx.Err()
                    }
                }
            }
        })
    }

    group.Go(func() error {
        workersWG.Wait()
        close(results)
        return nil
    })

    group.Go(func() error {
        for {
            select {
            case <-groupCtx.Done():
                return groupCtx.Err()
            case item, ok := <-results:
                if !ok {
                    return nil
                }
                if err := storage.Save(groupCtx, item); err != nil {
                    return fmt.Errorf("save item: %w", err)
                }
            }
        }
    })

    return group.Wait()
}
```

Если `repository.Get` или `storage.Save` возвращает причинную ошибку, `errgroup`
сохраняет её первой и отменяет `groupCtx`. Остальные стадии выходят из `select`,
workers вызывают `Done`, coordinator закрывает `results`, после чего `Wait`
дожидается всей цепочки.

---

## Почему закрытие каналов корректно

| Канал | Кто отправляет | Кто закрывает | Почему |
| --- | --- | --- | --- |
| `jobs` | Один producer | Тот же producer | Он знает, когда закончились все идентификаторы |
| `results` | Несколько workers | Coordinator после `workersWG.Wait` | Только coordinator знает, что завершился последний sender |

Consumer не закрывает `results`: worker может одновременно отправлять туда
значение и получить `panic: send on closed channel`. Отмена передаётся через
context, а закрытие означает только отсутствие будущих значений от владельца
канала.

Буфер может уменьшить число переключений между стадиями, но не устраняет
необходимость отменяемых send. После заполнения любого конечного буфера producer
снова блокируется.

---

## Пример теста

Один worker ждёт context внутри repository. Второй загружает значение только
после старта первого, а storage возвращает причинную ошибку. К моменту возврата
`Run` заблокированный worker должен подтвердить отмену.

```go
type repositoryFunc func(context.Context, string) (Item, error)

func (function repositoryFunc) Get(
    ctx context.Context,
    id string,
) (Item, error) {
    return function(ctx, id)
}

type storageFunc func(context.Context, Item) error

func (function storageFunc) Save(
    ctx context.Context,
    item Item,
) error {
    return function(ctx, item)
}

func TestRun_StorageErrorCancelsUpstream(test *testing.T) {
    blockedStarted := make(chan struct{})
    blockedExit := make(chan error, 1)
    saveErr := errors.New("storage unavailable")

    repository := repositoryFunc(func(
        ctx context.Context,
        id string,
    ) (Item, error) {
        switch id {
        case "blocked":
            close(blockedStarted)
            <-ctx.Done()
            blockedExit <- ctx.Err()
            return Item{}, ctx.Err()
        case "fast":
            <-blockedStarted
            return Item{ID: id}, nil
        default:
            return Item{}, fmt.Errorf("unexpected id %q", id)
        }
    })
    storage := storageFunc(func(
        _ context.Context,
        _ Item,
    ) error {
        return saveErr
    })

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    err := Run(
        ctx,
        []string{"blocked", "fast"},
        2,
        repository,
        storage,
    )
    if !errors.Is(err, saveErr) {
        test.Fatalf("Run error = %v, want %v", err, saveErr)
    }
    select {
    case blockedErr := <-blockedExit:
        if !errors.Is(blockedErr, context.Canceled) {
            test.Fatalf("blocked error = %v, want context.Canceled", blockedErr)
        }
    default:
        test.Fatal("blocked repository call did not observe cancellation")
    }
}
```

Ошибка storage должна остаться результатом `Run`. Производный
`context.Canceled` от соседнего worker менее информативен и не должен заменить
первопричину.

---

## Что проверить тестами

- Ошибка repository отменяет generator, других workers и consumer.
- Ошибка storage прекращает upstream и не оставляет goroutines на send.
- Parent context отменяет pipeline во время каждой стадии.
- `workers <= 0` возвращает ошибку до запуска goroutines.
- `results` закрывается только после последнего worker.
- Медленный consumer создаёт backpressure; при полном успехе каждая загруженная
  запись сохраняется ровно один раз. Порядок здесь контрактом не обещается.
- После возврата `Run` все тестовые goroutines подтвердили завершение каналами.

---

## Interview-ready answer

**1. Почему ранний выход consumer опасен?**

- Upstream — producers продолжают отправлять в канал, который больше никто не
  читает.
- Следствие — send блокируется, goroutine и удерживаемые ею данные остаются в
  памяти.

**2. Где проверять context?**

- Каналы — на каждом потенциально блокирующем send и receive.
- I/O — context должен дойти до БД, HTTP-клиента или другого внешнего вызова.

**3. Кто закрывает канал с несколькими producers?**

- Coordinator — ждёт всех senders через `WaitGroup` и закрывает канал один раз.
- Consumer — сообщает об остановке через context и не закрывает чужой канал.

**4. Исправит ли проблему большой буфер?**

- Временно — producer сможет отправить несколько значений без receiver.
- Предел — после заполнения буфера блокировка вернётся; lifecycle останется
  неправильным.

---

## Связанные материалы

- [Pipeline](../concurrency/04-pipeline.md)
- [Fan-In / Fan-Out](../concurrency/03-fan-in-fan-out.md)
- [Go Concurrency Patterns: Pipelines and cancellation](https://go.dev/blog/pipelines)
