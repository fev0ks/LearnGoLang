# Задача 9: worker pool с сохранением порядка

## Содержание

- [Формулировка](#формулировка)
- [Исходный код](#исходный-код)
- [Основные проблемы](#основные-проблемы)
- [Исправленное решение](#исправленное-решение)
- [Порядок и head-of-line blocking](#порядок-и-head-of-line-blocking)
- [Пример теста](#пример-теста)
- [Что проверить тестами](#что-проверить-тестами)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Задача проверяет координацию producer, фиксированного числа workers и результата.
Правильные вычисления недостаточны: pipeline должен завершаться при успехе,
ошибке и отмене, а порядок ответа должен соответствовать порядку входа.

---

## Формулировка

`TransformAll` обрабатывает строки через `workers` goroutines. `transform`
поддерживает context. После первой ошибки весь batch прекращается. Порядок
результатов совпадает с порядком `inputs`.

---

## Исходный код

```go
func TransformAll(
    ctx context.Context,
    inputs []string,
    workers int,
    transform func(context.Context, string) (string, error),
) ([]string, error) {
    jobs := make(chan string)
    results := make(chan string)
    errCh := make(chan error, 1)

    var wg sync.WaitGroup
    for workerID := 0; workerID < workers; workerID++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for input := range jobs {
                result, err := transform(ctx, input)
                if err != nil {
                    errCh <- err
                    return
                }
                results <- result
            }
        }()
    }

    go func() {
        defer close(jobs)
        for _, input := range inputs {
            jobs <- input
        }
    }()

    wg.Wait()
    close(results)

    output := make([]string, 0, len(inputs))
    for result := range results {
        output = append(output, result)
    }

    select {
    case err := <-errCh:
        return nil, err
    default:
        return output, nil
    }
}
```

---

## Основные проблемы

| Проблема | Последствие |
| --- | --- |
| `wg.Wait` выполняется раньше чтения `results` | Workers блокируются на send, поэтому не вызывают `Done`; возникает deadlock |
| Producer отправляет в `jobs` без `select` по context | После раннего выхода workers он может навсегда остаться на send |
| Worker отправляет результат без пути отмены | Отмена caller не разблокирует заполненный или оставшийся без receiver канал |
| Несколько workers отправляют ошибки в канал ёмкостью `1` | Вторая ошибка может заблокировать worker и снова удержать `Wait` |
| Результат не содержит индекс | Порядок зависит от времени завершения `transform` |
| `workers <= 0` не проверяется | Producer блокируется на первой задаче, потому что receivers отсутствуют |

Увеличение буфера `results` до `len(inputs)` скрывает конкретный deadlock, но не
определяет отмену, ошибки и порядок. Размер буфера не заменяет lifecycle.

---

## Исправленное решение

Worker пишет результат сразу в выделенный ему индекс. Отдельный выходной канал
не нужен. Producer и workers входят в одну `errgroup`, поэтому первая ошибка
отменяет отправку следующих jobs и внешние операции.

```go
func TransformAll(
    ctx context.Context,
    inputs []string,
    workers int,
    transform func(context.Context, string) (string, error),
) ([]string, error) {
    if workers <= 0 {
        return nil, fmt.Errorf("workers must be positive")
    }

    type job struct {
        index int
        input string
    }

    jobs := make(chan job)
    output := make([]string, len(inputs))
    group, groupCtx := errgroup.WithContext(ctx)

    group.Go(func() error {
        defer close(jobs)
        for index, input := range inputs {
            select {
            case jobs <- job{index: index, input: input}:
            case <-groupCtx.Done():
                return groupCtx.Err()
            }
        }
        return nil
    })

    for workerID := 0; workerID < workers; workerID++ {
        group.Go(func() error {
            for {
                select {
                case <-groupCtx.Done():
                    return groupCtx.Err()
                case job, ok := <-jobs:
                    if !ok {
                        return nil
                    }

                    result, err := transform(groupCtx, job.input)
                    if err != nil {
                        return fmt.Errorf(
                            "transform input %d: %w",
                            job.index,
                            err,
                        )
                    }
                    output[job.index] = result
                }
            }
        })
    }

    if err := group.Wait(); err != nil {
        return nil, err
    }
    return output, nil
}
```

Каждый элемент `output` имеет одного writer. `group.Wait` создаёт границу: до
его успешного возврата слайс не читается. Поэтому общий mutex вокруг записи в
разные индексы не нужен.

Производитель является единственным sender, поэтому он же закрывает `jobs`.
Workers канал не закрывают: ни один из них не знает, завершились ли остальные
отправки.

---

## Порядок и head-of-line blocking

Порядок можно сохранить двумя способами:

| Подход | Поведение |
| --- | --- |
| Запись по исходному индексу | Все результаты возвращаются вместе после `Wait`; реализация проста |
| Поток результатов с reorder buffer | Готовые результаты накапливаются по индексам и выдаются, когда готов следующий ожидаемый индекс |

Если задача с индексом `0` выполняется десять секунд, а остальные заканчиваются
за миллисекунду, поток с сохранением порядка не может выдать индекс `1` раньше
индекса `0`. Это head-of-line blocking — цена строгого порядка, а не дефект
worker pool.

Если caller допускает completion order, API должен сказать это явно и может
возвращать поток пар `(index, result)`.

---

## Пример теста

Все три операции сначала доходят до управляемого барьера. Затем тест разрешает
им завершаться в порядке `c`, `b`, `a`, но ожидает результат в исходном порядке
`a`, `b`, `c`.

```go
func TestTransformAll_PreservesInputOrder(test *testing.T) {
    inputs := []string{"a", "b", "c"}
    gates := map[string]chan struct{}{
        "a": make(chan struct{}),
        "b": make(chan struct{}),
        "c": make(chan struct{}),
    }
    started := make(chan string, len(inputs))
    finished := make(chan string, len(inputs))

    transform := func(
        ctx context.Context,
        input string,
    ) (string, error) {
        started <- input
        select {
        case <-gates[input]:
            finished <- input
            return "result-" + input, nil
        case <-ctx.Done():
            return "", ctx.Err()
        }
    }

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    type transformResult struct {
        values []string
        err    error
    }
    done := make(chan transformResult, 1)
    go func() {
        values, err := TransformAll(ctx, inputs, 3, transform)
        done <- transformResult{values: values, err: err}
    }()

    for startedCount := 0; startedCount < len(inputs); startedCount++ {
        select {
        case <-started:
        case <-ctx.Done():
            test.Fatalf("only %d transforms started", startedCount)
        }
    }
    for _, input := range []string{"c", "b", "a"} {
        close(gates[input])
        var got string
        select {
        case got = <-finished:
        case <-ctx.Done():
            test.Fatalf("transform %q did not finish", input)
        }
        if got != input {
            test.Fatalf("finished %q, want %q", got, input)
        }
    }

    var result transformResult
    select {
    case result = <-done:
    case <-ctx.Done():
        test.Fatal("TransformAll did not return")
    }
    if result.err != nil {
        test.Fatalf("TransformAll: %v", result.err)
    }
    want := []string{"result-a", "result-b", "result-c"}
    if !slices.Equal(result.values, want) {
        test.Fatalf("result = %v, want %v", result.values, want)
    }
}
```

Каналы `gates` управляют допуском к завершению. Timeout context остаётся только
предохранителем от зависшего теста и не заменяет эти барьеры.

---

## Что проверить тестами

- Задачи завершаются в обратном порядке, но результат остаётся в порядке входа.
- Число одновременно работающих `transform` не превышает `workers`.
- Ошибка одной задачи отменяет producer и другие операции.
- Отмена во время send в `jobs` завершает все goroutines.
- Пустой вход возвращает пустой результат.
- `workers <= 0` возвращает ошибку без запуска goroutines.
- `go test -race` не обнаруживает конфликтов при записи в разные индексы.

---

## Interview-ready answer

**1. Откуда возникает deadlock в исходном коде?**

- Worker — ждёт receiver на `results`.
- Coordinator — ждёт `wg`, прежде чем начать читать `results`.
- Цикл — ни одна сторона не может первой продолжить выполнение.

**2. Кто закрывает `jobs` и `results`?**

- Ownership — канал закрывает sender, который знает, что новых значений не
  будет.
- Несколько senders — отдельный coordinator закрывает канал после их завершения.

**3. Как сохранить порядок?**

- Индекс — передавать с задачей её исходную позицию.
- Хранилище — писать в заранее выделенный `results[index]` и читать после
  завершения workers.

**4. Почему одного `ctx` в сигнатуре недостаточно?**

- Блокировки — каждый send, receive и внешний вызов должен иметь путь отмены.
- Завершение — при ошибке downstream producer тоже должен прекратить отправку.

---

## Связанные материалы

- [Worker Pool](../concurrency/01-worker-pool.md)
- [Pipeline](../concurrency/04-pipeline.md)
- [`errgroup`](https://pkg.go.dev/golang.org/x/sync/errgroup)
