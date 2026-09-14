# Задача 11: фоновая работа из HTTP-handler

## Содержание

- [Формулировка](#формулировка)
- [Исходный код](#исходный-код)
- [Основные проблемы](#основные-проблемы)
- [Предпочтительное решение](#предпочтительное-решение)
- [Когда допустим in-process executor](#когда-допустим-in-process-executor)
- [Пример теста](#пример-теста)
- [Что проверить тестами](#что-проверить-тестами)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Задача проверяет ownership фоновой goroutine. Ответ `202 Accepted` не объясняет,
кто теперь владеет работой, где хранится её состояние и как она завершается при
shutdown процесса.

---

## Формулировка

HTTP endpoint запускает построение отчёта и сразу отвечает клиенту. Построение
может занимать несколько минут. Нужно проверить отмену, ограничение нагрузки,
наблюдаемость результата и поведение при остановке или падении процесса.

---

## Исходный код

```go
func (h *Handler) StartReport(
    response http.ResponseWriter,
    request *http.Request,
) {
    userID := request.PathValue("userID")

    go func() {
        report, err := h.reports.Generate(
            request.Context(),
            userID,
        )
        if err != nil {
            log.Printf("generate report: %v", err)
            return
        }

        h.storage.Save(context.Background(), report)
    }()

    response.WriteHeader(http.StatusAccepted)
}
```

---

## Основные проблемы

| Проблема | Последствие |
| --- | --- |
| Фоновая работа использует `request.Context()` | Для входящего server request context отменяется при закрытии соединения, отмене клиента или возврате `ServeHTTP`; работа обычно прекращается сразу после `202` |
| На каждый запрос безусловно создаётся goroutine | Наплыв запросов создаёт неограниченное число операций и расходует память, соединения и квоты |
| `Save` получает `context.Background()` | Сохранение не имеет deadline и не связано с shutdown сервиса |
| Ошибка `Save` игнорируется | Отчёт теряется, а система может выглядеть успешной |
| Нет job ID и состояния | Клиент не может проверить `queued`, `running`, `failed` или `done` |
| Нет координации shutdown | Процесс не ждёт работу и не просит её завершиться |
| Нет границы для panic | Panic внутри goroutine завершает весь процесс |
| Работа хранится только в памяти | Crash или рестарт теряет принятый запрос |

Замена `request.Context()` на `context.Background()` исправляет только слишком
раннюю отмену. Она одновременно создаёт работу без владельца, deadline,
backpressure и graceful shutdown, поэтому не является полным решением.

---

## Предпочтительное решение

Для важного многоминутного отчёта handler сохраняет job в durable queue или БД,
а отдельный worker обрабатывает её. Ответ возвращает стабильный `jobID`.

```go
type ReportJob struct {
    UserID string
}

func (h *Handler) StartReport(
    response http.ResponseWriter,
    request *http.Request,
) {
    idempotencyKey := request.Header.Get("Idempotency-Key")
    if idempotencyKey == "" {
        http.Error(
            response,
            "missing Idempotency-Key",
            http.StatusBadRequest,
        )
        return
    }

    jobID, err := h.queue.Enqueue(
        request.Context(),
        idempotencyKey,
        ReportJob{UserID: request.PathValue("userID")},
    )
    if err != nil {
        http.Error(
            response,
            "cannot enqueue report",
            http.StatusServiceUnavailable,
        )
        return
    }

    response.Header().Set("Content-Type", "application/json")
    response.WriteHeader(http.StatusAccepted)
    if err := json.NewEncoder(response).Encode(struct {
        JobID string `json:"job_id"`
    }{JobID: jobID}); err != nil {
        log.Printf("write report response: %v", err)
    }
}
```

Приём считается успешным после durable-записи. Queue должна атомарно связывать
`Idempotency-Key` с одним `jobID`: повтор HTTP-запроса возвращает прежний job, а
не создаёт новый. Worker использует собственный контекст попытки, ограниченный
timeout, записывает terminal status и делает обработку идемпотентной по `jobID`.
Повторная доставка сообщения тогда не создаёт два разных отчёта.

Нужно отдельно определить атомарность: если job и бизнес-изменение записываются
в одну БД, для согласованной публикации может понадобиться transactional outbox.

---

## Когда допустим in-process executor

In-process выполнение подходит для best-effort работы, которую разрешено
потерять при рестарте: например, обновление необязательной метрики. Даже тогда
executor должен иметь:

- контекст уровня сервиса, отменяемый при shutdown;
- конечный timeout каждой задачи;
- bounded concurrency и явный ответ при переполнении;
- `WaitGroup`, в котором `Add` согласован с началом shutdown;
- panic boundary вокруг пользовательской функции;
- обработку и метрики ошибок.

`Submit` и `Stop` требуют единого перехода состояния. Проверка `stopped` перед
`wg.Add` без mutex недостаточна: `Stop` может начать `Wait`, а конкурентный
`Submit` увеличить счётчик после этой проверки. Обычно mutex защищает состояние
`accepting/stopped` и участок до `Add`; после перехода в `stopped` новые `Add`
запрещены.

Буферизированный канал или semaphore задаёт ёмкость, но контракт переполнения
остаётся продуктовым решением: блокировать handler, вернуть `503`, отбросить
задачу либо записать её во внешнюю очередь.

---

## Пример теста

Тест HTTP-границы проверяет, что handler передаёт idempotency key и входные
данные в очередь до ответа `202`, а клиент получает сохранённый `jobID`.

```go
type queueFunc func(
    context.Context,
    string,
    ReportJob,
) (string, error)

func (function queueFunc) Enqueue(
    ctx context.Context,
    key string,
    job ReportJob,
) (string, error) {
    return function(ctx, key, job)
}

func TestStartReport_EnqueuesBeforeAccepted(test *testing.T) {
    var calls atomic.Int32
    handler := Handler{
        queue: queueFunc(func(
            ctx context.Context,
            key string,
            job ReportJob,
        ) (string, error) {
            calls.Add(1)
            if err := ctx.Err(); err != nil {
                test.Fatalf("enqueue context: %v", err)
            }
            if key != "request-7" {
                test.Fatalf("key = %q", key)
            }
            if job.UserID != "user-42" {
                test.Fatalf("user = %q", job.UserID)
            }
            return "job-123", nil
        }),
    }

    request := httptest.NewRequest(
        http.MethodPost,
        "/users/user-42/reports",
        nil,
    )
    request.SetPathValue("userID", "user-42")
    request.Header.Set("Idempotency-Key", "request-7")
    response := httptest.NewRecorder()

    handler.StartReport(response, request)

    if response.Code != http.StatusAccepted {
        test.Fatalf("status = %d", response.Code)
    }
    if calls.Load() != 1 {
        test.Fatalf("enqueue calls = %d, want 1", calls.Load())
    }

    var body struct {
        JobID string `json:"job_id"`
    }
    if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
        test.Fatalf("decode response: %v", err)
    }
    if body.JobID != "job-123" {
        test.Fatalf("job id = %q", body.JobID)
    }
}
```

`httptest.NewRecorder` проверяет handler как функцию. Для свойств реального
HTTP-сервера, например отмены context после возврата `ServeHTTP`, нужен
`httptest.Server` или integration test: прямой вызов handler не воспроизводит
server lifecycle автоматически.

---

## Что проверить тестами

- Возврат handler не отменяет уже принятую durable job.
- Ошибка enqueue возвращает не `202`, а явную ошибку сервиса.
- Повтор с тем же idempotency key не создаёт вторую job.
- Worker ограничивает число одновременно выполняющихся отчётов.
- Shutdown прекращает приём, отменяет или доделывает in-flight работу согласно
  контракту и имеет deadline.
- Panic одной best-effort задачи не завершает процесс.
- Ошибка сохранения переводит job в `failed`, а не теряется только в логе.

---

## Interview-ready answer

**1. Почему нельзя использовать request context после ответа?**

- Lifecycle — server request context связан с жизнью запроса и отменяется после
  завершения handler.
- Следствие — долгоживущая работа получает отмену почти сразу после `202`.

**2. Достаточно ли заменить его на `Background()`?**

- Нет владельца — такой context не связан с shutdown и не имеет deadline.
- Нет ёмкости — goroutines всё ещё создаются без ограничения.

**3. Когда нужна внешняя очередь?**

- Durability — принятую работу нельзя терять при crash или рестарте.
- Состояние — клиенту нужны job ID, retry и наблюдаемый результат.
- Нагрузка — очередь даёт bounded consumption и явный backlog.

**4. Что требуется от in-process executor?**

- Lifecycle — общий service context и согласованный `Submit`/`Stop`.
- Ограничение — bounded concurrency и политика переполнения.
- Завершение — deadline задачи, ожидание workers, panic boundary и ошибки.

---

## Связанные материалы

- [Background Task Processor](./02-background-task-processor.md)
- [Context patterns](../../../01-go-core/concurrency-and-performance/04-context-patterns.md)
- [`http.Request.Context`](https://pkg.go.dev/net/http#Request.Context)
