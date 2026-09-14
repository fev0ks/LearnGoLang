# Задача 7: batch с семафором

## Содержание

- [Формулировка](#формулировка)
- [Исходный код](#исходный-код)
- [Основные проблемы](#основные-проблемы)
- [Исправленное решение](#исправленное-решение)
- [Что именно ограничивает семафор](#что-именно-ограничивает-семафор)
- [Пример теста](#пример-теста)
- [Что проверить тестами](#что-проверить-тестами)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Задача проверяет ограничение параллелизма через `semaphore.Weighted`. Нужно
различать число одновременно выполняющихся операций, число созданных goroutine
и размер ожидающей очереди.

---

## Формулировка

`LoadAll` загружает элементы с максимальным параллелизмом `limit`, сохраняет
порядок входа и прекращает batch после первой ошибки. Caller должен иметь
возможность отменить и ожидание разрешения семафора, и уже запущенные загрузки.

---

## Исходный код

```go
func LoadAll(
    ctx context.Context,
    ids []string,
    limit int64,
    load func(context.Context, string) (Item, error),
) ([]Item, error) {
    sem := semaphore.NewWeighted(limit)
    results := make([]Item, 0, len(ids))
    errCh := make(chan error)

    var wg sync.WaitGroup
    for _, id := range ids {
        wg.Add(1)
        go func(id string) {
            defer wg.Done()

            if err := sem.Acquire(ctx, 1); err != nil {
                errCh <- err
                return
            }

            item, err := load(ctx, id)
            if err != nil {
                errCh <- err
                return
            }

            sem.Release(1)
            results = append(results, item)
        }(id)
    }

    wg.Wait()
    close(errCh)

    if err := <-errCh; err != nil {
        return nil, err
    }
    return results, nil
}
```

---

## Основные проблемы

| Проблема | Конкретное последствие |
| --- | --- |
| Goroutine создаётся до `Acquire` | При миллионе входов создаётся до миллиона goroutines; семафор ограничивает только вход в `load` |
| `errCh` небуферизован, а чтение начинается после `wg.Wait` | Первая ошибка блокируется на send, `Done` не вызывается, `Wait` не завершается |
| `Release` не находится в `defer` сразу после успешного `Acquire` | Ошибка `load` навсегда удерживает разрешение; следующие goroutines зависают |
| Конкурентный `append` в один слайс | Возникает data race, повреждается заголовок слайса, порядок не сохраняется |
| Ошибка одной операции не отменяет остальные | Batch продолжает тратить ресурсы после известного terminal outcome |
| `limit <= 0` не проверяется | При нулевой или отрицательной ёмкости ни одна операция с весом `1` не получит разрешение |

`Release(1)` можно вызывать только после успешного `Acquire(ctx, 1)`. Если
`Acquire` вернул ошибку, разрешение не получено и освобождение нарушит счётчик
семафора.

---

## Исправленное решение

Разрешение захватывается до создания goroutine. Поэтому не накапливается очередь
из goroutines, заблокированных внутри `Acquire`. Каждая операция пишет в
собственный индекс заранее выделенного слайса, что одновременно убирает гонку и
сохраняет порядок.

```go
func LoadAll(
    ctx context.Context,
    ids []string,
    limit int64,
    load func(context.Context, string) (Item, error),
) ([]Item, error) {
    if limit <= 0 {
        return nil, fmt.Errorf("limit must be positive")
    }

    batchCtx, cancel := context.WithCancelCause(ctx)
    defer cancel(nil)

    sem := semaphore.NewWeighted(limit)
    results := make([]Item, len(ids))

    var wg sync.WaitGroup
    for index, id := range ids {
        if err := sem.Acquire(batchCtx, 1); err != nil {
            cancel(err)
            break
        }

        wg.Add(1)
        go func(index int, id string) {
            defer wg.Done()
            defer sem.Release(1)

            item, err := load(batchCtx, id)
            if err != nil {
                cancel(fmt.Errorf("load %q: %w", id, err))
                return
            }
            results[index] = item
        }(index, id)
    }

    wg.Wait()
    if err := context.Cause(batchCtx); err != nil {
        return nil, err
    }
    return results, nil
}
```

После ошибки `cancel` разблокирует producer, если тот ждёт `Acquire`, и передаёт
сигнал уже запущенным `load`. Функция всё равно ждёт их завершения: безопасно
вернуться со слайсом можно только после прекращения всех записей в него.

Запись в разные элементы `results` не требует mutex, пока длина слайса больше не
меняется и никто не читает элементы до `wg.Wait`. `append` меняет общий заголовок
слайса и для этого паттерна не подходит.

---

## Что именно ограничивает семафор

| Место `Acquire` | Одновременно выполняющихся `load` | Ожидающие goroutines |
| --- | --- | --- |
| Внутри goroutine | Не больше `limit` | До `len(ids)` могут ждать внутри `Acquire` |
| Перед `go` | Не больше `limit` | На семафоре ждёт producer, отдельные job goroutines ещё не созданы |

Захват до `go` создаёт backpressure на вызывающую goroutine. Если вход должен
приниматься независимо от скорости обработки, нужен bounded worker pool или
внешняя очередь с явно описанной политикой переполнения.

`semaphore.Weighted` умеет захватывать разный вес. Это полезно, если задачи
потребляют разное количество ограниченного ресурса. Для одинакового веса `1`
буферизованный канал часто проще, но требует той же дисциплины acquire/release.

---

## Пример теста

Loader считает активные вызовы и ждёт общего сигнала. Первые два вызова
заполняют лимит, после чего тест освобождает их и проверяет наблюдавшийся максимум
и порядок результата.

```go
func TestLoadAll_LimitsConcurrencyAndPreservesOrder(test *testing.T) {
    ids := []string{"a", "b", "c", "d"}
    entered := make(chan struct{}, len(ids))
    release := make(chan struct{})
    var releaseOnce sync.Once

    releaseAll := func() {
        releaseOnce.Do(func() { close(release) })
    }
    defer releaseAll()

    var mutex sync.Mutex
    active := 0
    maxActive := 0

    load := func(_ context.Context, id string) (Item, error) {
        mutex.Lock()
        active++
        if active > maxActive {
            maxActive = active
        }
        mutex.Unlock()

        entered <- struct{}{}
        <-release

        mutex.Lock()
        active--
        mutex.Unlock()
        return Item{ID: id}, nil
    }

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    type batchResult struct {
        items []Item
        err   error
    }
    done := make(chan batchResult, 1)
    go func() {
        items, err := LoadAll(ctx, ids, 2, load)
        done <- batchResult{items: items, err: err}
    }()

    for started := 0; started < 2; started++ {
        select {
        case <-entered:
        case <-ctx.Done():
            test.Fatalf("only %d loaders entered", started)
        }
    }
    releaseAll()

    var result batchResult
    select {
    case result = <-done:
    case <-ctx.Done():
        test.Fatal("LoadAll did not return")
    }
    if result.err != nil {
        test.Fatalf("LoadAll: %v", result.err)
    }
    if len(result.items) != len(ids) {
        test.Fatalf("result length = %d, want %d", len(result.items), len(ids))
    }
    if maxActive != 2 {
        test.Fatalf("max active = %d, want 2", maxActive)
    }
    for index, id := range ids {
        if result.items[index].ID != id {
            test.Fatalf(
                "result[%d].ID = %q, want %q",
                index,
                result.items[index].ID,
                id,
            )
        }
    }
}
```

Канал `entered` буферизован по числу входов, поэтому поздние задачи не зависят от
того, продолжает ли тест его читать после освобождения первых двух вызовов.
`releaseAll` вызывается через `defer`, чтобы ошибка теста не оставила loaders
заблокированными.

---

## Что проверить тестами

- Счётчик активных `load` никогда не превышает `limit`.
- Не образуется очередь job goroutines, заблокированных внутри `Acquire`.
- Ошибка одной задачи отменяет ожидающий `Acquire` и остальные загрузки.
- Ошибка не оставляет занятое разрешение.
- Результаты соответствуют порядку `ids`, даже если загрузки завершаются иначе.
- Отмена parent context сохраняется как причина через `context.Cause`.
- `limit == 0` и отрицательный limit возвращают ошибку без запуска работы.
- Конкурентный тест проходит с `go test -race`.

Для проверки параллелизма loader увеличивает atomic-счётчик, сообщает о входе в
канал и ждёт `release`. Это даёт точную границу без `time.Sleep`.

---

## Interview-ready answer

**1. Почему исходный semaphore не ограничивает число goroutines?**

- Порядок — goroutine создаётся раньше `Acquire` и только потом ждёт разрешение.
- Следствие — выполняется не больше `limit` загрузок, но ожидающих goroutines
  может быть столько же, сколько входов.

**2. Где должен стоять `Release`?**

- Условие — только после успешного `Acquire`.
- Cleanup — сразу зарегистрировать `defer sem.Release(weight)`, чтобы покрыть
  успех, ошибку и ранний выход.

**3. Почему исходный код зависает на ошибке?**

- Цикл — worker отправляет ошибку в небуферизованный канал, а receiver начнёт
  чтение только после `wg.Wait`.
- Deadlock — worker не доходит до `Done`, поэтому `Wait` не возвращается.

**4. Как сохранить порядок без mutex?**

- Индекс — заранее выделить `results` нужной длины и передать каждой задаче свой
  индекс.
- Синхронизация — читать результаты только после `Wait`.

---

## Связанные материалы

- [Worker Pool](../concurrency/01-worker-pool.md)
- [Worker Pool: code review](../concurrency/07-worker-pool-debug.md)
- [`semaphore.Weighted`](https://pkg.go.dev/golang.org/x/sync/semaphore)
