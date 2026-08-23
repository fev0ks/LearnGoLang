# Работа из Go

## Содержание

- [Выбор драйвера](#выбор-драйвера)
- [Соединение](#соединение)
- [Батчевая вставка](#батчевая-вставка)
- [Идемпотентная вставка с токеном](#идемпотентная-вставка-с-токеном)
- [Асинхронная вставка из приложения](#асинхронная-вставка-из-приложения)
- [Чтение](#чтение)
- [Timeout, retry и наблюдаемость](#timeout-retry-и-наблюдаемость)
- [Ожидание мутации](#ожидание-мутации)
- [Что ломается в Go-коде поверх ClickHouse чаще всего](#что-ломается-в-go-коде-поверх-clickhouse-чаще-всего)
- [Interview-ready answer](#interview-ready-answer)
- [Официальная документация](#официальная-документация)

Отличие от привычной работы с PostgreSQL начинается не с API, а с единицы работы: там она — запрос или транзакция, здесь — батч. Всё остальное в этой статье следует из этого различия.

---

## Выбор драйвера

Основной драйвер — `github.com/ClickHouse/clickhouse-go/v2`: поддерживает родной протокол (порт 9000) и HTTP (8123), даёт как собственный API, так и совместимость с `database/sql`.

| Вариант | Когда выбирать |
| --- | --- |
| `clickhouse-go/v2`, собственный API | обычный выбор: батчи, настройки на запрос, стриминг результата |
| `clickhouse-go/v2` через `database/sql` | нужен единый интерфейс с остальными базами или готовые обёртки |
| `github.com/ClickHouse/ch-go` | предельная пропускная способность: колоночный API без построчного преобразования |

Сравнение подходов к работе с базами из Go — в [go-database-libraries](../../go-database-libraries/README.md).

---

## Соединение

```go
import (
    "time"

    "github.com/ClickHouse/clickhouse-go/v2"
    "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func newConn() (driver.Conn, error) {
    return clickhouse.Open(&clickhouse.Options{
        Addr: []string{"ch-1:9000", "ch-2:9000"},
        Auth: clickhouse.Auth{
            Database: "analytics",
            Username: "writer",
            Password: password,
        },
        // настройки уровня соединения применяются ко всем запросам
        Settings: clickhouse.Settings{
            "max_execution_time": 60,
        },
        Compression: &clickhouse.Compression{
            Method: clickhouse.CompressionLZ4,
        },
        DialTimeout:  5 * time.Second,
        MaxOpenConns: 10,
        MaxIdleConns: 5,
    })
}
```

В примере `password` приходит из secret storage и намеренно не определён в snippet. В production также настраивают TLS, `ConnMaxLifetime`, стратегию выбора адресов (`in_order`, `round_robin`, `random`) и вызывают `Ping` при старте. Список `Addr` даёт failover соединения, но не делает чтение с разных реплик линейно согласованным — это отдельная настройка ClickHouse.

Сжатие включают осознанно: `LZ4` заметно уменьшает объём трафика при вставке широких batches, платя за это временем CPU на обеих сторонах.

---

## Батчевая вставка

`PrepareBatch` резервирует соединение, а `Append` буферизует строки на стороне клиента. До `Flush` или `Send` показанный код ничего не отправляет на сервер, поэтому ошибка `Append` не требует rollback. `Send` отправляет остаток и завершает INSERT; потеря ответа после `Send` означает неизвестный результат, который закрывается идемпотентным retry.

```go
func insertEvents(ctx context.Context, conn driver.Conn, events []Event) error {
    batch, err := conn.PrepareBatch(ctx, "INSERT INTO events")
    if err != nil {
        return err
    }
    defer batch.Close()

    for _, e := range events {
        err = batch.Append(e.Date, e.Time, e.UserID, e.Type, e.Value)
        if err != nil {
            return err
        }
    }

    return batch.Send()
}
```

Размер `events` — то самое место, где решается судьба таблицы. Вызов этой функции на каждое событие создаст до одного part на событие и затронутую партицию; накопление до 10 000–100 000 строк или до истечения таймера обычно даёт гораздо меньше parts.

`Flush` нужен только для long-lived batch и меняет failure model: уже отправленные blocks не откатываются. Такой batch не стоит держать на обычном pool connection без понимания `WithReleaseConnection` и `WithCloseOnFlush`.

---

## Идемпотентная вставка с токеном

Токен передаётся как настройка запроса через контекст. Ключевое требование — токен должен воспроизводиться при повторе, то есть выводиться из данных, а не генерироваться заново:

```go
// token строится из источника данных: topic-partition-offset, id файла,
// id задачи ETL. Случайный uuid здесь бесполезен — при повторе он изменится.
func insertBatchIdempotent(
    ctx context.Context,
    conn driver.Conn,
    token string,
    events []Event,
) error {
    ctx = clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
        "insert_deduplication_token": token,
    }))

    batch, err := conn.PrepareBatch(ctx, "INSERT INTO events")
    if err != nil {
        return err
    }
    defer batch.Close()

    for _, e := range events {
        if err := batch.Append(e.Date, e.Time, e.UserID, e.Type, e.Value); err != nil {
            return err
        }
    }
    return batch.Send()
}

func consumeKafka(ctx context.Context, conn driver.Conn, msgs []kafka.Message) error {
    if len(msgs) == 0 {
        return nil
    }

    first := msgs[0]
    last := msgs[len(msgs)-1]

    events := make([]Event, 0, len(msgs))
    for i, m := range msgs {
        if m.Topic != first.Topic || m.Partition != first.Partition {
            return fmt.Errorf("batch mixes Kafka partitions")
        }
        if i > 0 && m.Offset != msgs[i-1].Offset+1 {
            return fmt.Errorf("non-contiguous Kafka offsets")
        }

        event, err := decode(m)
        if err != nil {
            return fmt.Errorf("decode offset %d: %w", m.Offset, err)
        }
        events = append(events, event)
    }

    token := fmt.Sprintf(
        "kafka:%s:%d:%d:%d",
        first.Topic, first.Partition, first.Offset, last.Offset,
    )

    // retry обязан повторить тот же диапазон offsets и тот же порядок строк.
    return retry(ctx, func() error {
        return insertBatchIdempotent(ctx, conn, token, events)
    })
}
```

Ограничения этого приёма нужно помнить целиком:

- повтор обязан содержать те же строки в том же порядке и с тем же разбиением на батчи;
- журнал дедупликации конечен: старые identifiers вытесняются по числу и времени относительно новых inserts;
- на нереплицированной таблице механизм выключен: нужен `Replicated`-движок либо `non_replicated_deduplication_window` больше нуля;
- token описывает весь batch: одинаковый first offset для разных диапазонов недопустим, потому что token имеет приоритет над хешем данных;
- на 25.8 async/materialized-view флаги задаются отдельно; на 26.2+ основной переключатель — `deduplicate_insert`.

---

## Асинхронная вставка из приложения

```go
// wait = true: ответ приходит после вставки буфера в целевую таблицу,
// поэтому ошибка вставки доходит до приложения
asyncCtx := clickhouse.Context(ctx, clickhouse.WithAsync(true))
err := conn.Exec(asyncCtx,
    "INSERT INTO events VALUES (?, ?, ?, ?, ?)",
    e.Date, e.Time, e.UserID, e.Type, e.Value)
```

`WithAsync(true)` включает async insert с ожиданием server flush. Старый метод `AsyncInsert()` deprecated. Вариант `WithAsync(false)` подтверждает запрос до flush и допускает тихую потерю при падении сервера.

Такой вызов уместен, когда экземпляров сервиса много, каждый порождает редкие события, и накопить осмысленный batch на стороне приложения нечем. Если сервис уже обрабатывает поток, client-side batching даёт более явные границы идемпотентности.

---

## Чтение

```go
type OrderRow struct {
    OrderID   uint64    `ch:"order_id"`
    Status    string    `ch:"status"`
    UpdatedAt time.Time `ch:"updated_at"`
}

func currentOrders(ctx context.Context, conn driver.Conn, from, to uint64) ([]OrderRow, error) {
    var rows []OrderRow

    // Один argMax(tuple(...), version) берёт поля из одной версии строки.
    const q = `
        SELECT
            order_id,
            tupleElement(current, 1) AS status,
            tupleElement(current, 2) AS updated_at
        FROM
        (
            SELECT
                order_id,
                argMax(tuple(status, updated_at, is_deleted), version) AS current
            FROM orders
            WHERE order_id BETWEEN ? AND ?
            GROUP BY order_id
        )
        WHERE tupleElement(current, 3) = 0`

    err := conn.Select(ctx, &rows, q, from, to)
    return rows, err
}
```

`Select` собирает весь результат в память — это приемлемо для дашбордного запроса на тысячи строк и неприемлемо для выгрузки. Для больших выборок используется `Query` с построчным чтением:

```go
rows, err := conn.Query(ctx, q, from, to)
if err != nil {
    return err
}
defer rows.Close()

for rows.Next() {
    var r OrderRow
    if err := rows.ScanStruct(&r); err != nil {
        return err
    }
    // обработка строки
}
return rows.Err()
```

---

## Timeout, retry и наблюдаемость

У client timeout и server `max_execution_time` разные роли. Context ограничивает, сколько приложение готово ждать; server setting ограничивает выполнение на ClickHouse. Context cancellation нужно передавать в каждый вызов и всегда закрывать `rows`, иначе соединение и server query могут жить дольше handler.

Не любую ошибку следует повторять:

- network reset, timeout и временная недоступность replica могут быть retriable;
- syntax error, type mismatch, превышение квоты и некорректный DDL не исправятся backoff;
- timeout после отправки INSERT означает unknown commit status: retry безопасен только с тем же batch identity;
- backoff должен иметь jitter и прекращаться при `ctx.Done()`.

Каждому важному запросу полезно назначать `query_id` и прокидывать trace context:

```go
queryID := "orders-dashboard:" + requestID
queryCtx := clickhouse.Context(ctx, clickhouse.WithQueryID(queryID))

rows, err := conn.Query(queryCtx, q, from, to)
```

По `query_id` запрос связывается с `system.query_log`, application log и trace. Драйвер также поддерживает OpenTelemetry, progress/profile callbacks и структурированный `slog`. Минимальные метрики приложения: latency и errors по operation, batch rows/bytes, retry count, pool saturation и время ожидания connection.

---

## Ожидание мутации

Мутация асинхронна, и код, который сразу после `ALTER` читает данные, увидит старое состояние. Дождаться завершения можно настройкой `mutations_sync` либо опросом `system.mutations`:

```go
ctx = clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
    "mutations_sync": 2, // 1 — ждать на текущем сервере, 2 — на всех репликах
}))

err := conn.Exec(ctx, "ALTER TABLE events UPDATE value = 0 WHERE user_id = ?", userID)
```

Ждать так можно только заведомо короткие мутации: тяжёлая перезапись партиции удержит запрос на часы и упрётся в таймаут. Для остальных случаев естественнее запустить мутацию асинхронно и отслеживать `is_done` в `system.mutations`.

---

## Что ломается в Go-коде поверх ClickHouse чаще всего

- **вставка по одной строке.** Наиболее частая причина `Too many parts`. Батч накапливается по размеру и по таймеру, а не по каждому событию;
- **новый батч на каждую строку вместо одного `PrepareBatch`.** Батч держит соединение: цикл из тысяч коротких батчей вырождается в тысячи вставок;
- **случайный `insert_deduplication_token`.** Токен, сгенерированный заново при повторе, не совпадёт с исходным и не даст никакой защиты;
- **token только из первого Kafka offset.** Другой диапазон с тем же началом будет принят за повтор; в token включают topic, partition, first и last offsets;
- **`FINAL` в горячем запросе по привычке.** На таблице с большим числом parts он превращает дашбордный запрос в полное слияние; `argMax` обычно решает ту же задачу дешевле;
- **`time.Time` без учёта часового пояса колонки.** `DateTime` в ClickHouse хранит момент времени, а отображается в зоне колонки или сервера; расхождение проявляется как сдвиг данных на часы в отчётах;
- **слепой retry любой ошибки.** Детерминированная ошибка создаёт retry storm, а неизвестный результат INSERT без стабильного token создаёт дубликаты;
- **ожидание общего rollback.** Один block в одной partition атомарен, но несколько partitions, shards, `Flush` и мутации не образуют одну прикладную транзакцию.

---

## Interview-ready answer

**1. Какой драйвер и почему?**

- `clickhouse-go/v2` — обычный выбор: родной протокол, батчи, настройки на запрос, совместимость с `database/sql` при необходимости.
- `ch-go` — низкоуровневый колоночный API, нужен когда упираются в пропускную способность вставки.
- Единица работы — батч, а не запрос: `PrepareBatch`, затем `Append` в цикле, затем `Send`.

**2. Как сделать вставку идемпотентной?**

- Настройка `insert_deduplication_token` передаётся через `clickhouse.Context` с `WithSettings`.
- Токен обязан воспроизводиться при повторе и описывать весь batch: например topic, partition, first и last Kafka offsets.
- Повтор должен содержать те же строки в том же порядке и с тем же разбиением на батчи.

**3. Чем Select отличается от Query?**

- `Select` собирает весь результат в память — годится для дашбордного запроса на тысячи строк.
- `Query` читает построчно и подходит для выгрузок.

**4. Как дождаться мутации из кода?**

- Настройкой `mutations_sync = 1` или `2` в контексте запроса, но только для заведомо коротких операций.
- Иначе мутация запускается асинхронно, а прогресс отслеживается по `system.mutations`.

**5. Чего ожидать от ClickHouse в Go-коде не стоит?**

- Общего rollback нескольких partitions, shards или уже отправленных через `Flush` blocks.
- Безусловно безопасного retry: unknown insert status требует той же batch identity и включённой дедупликации.

**6. Что нужно для production observability?**

- Context deadlines и cancellation на каждом запросе, обязательный `Close`/`Err` для streaming rows.
- Стабильный `query_id`, связывающий application log с `system.query_log`, и trace context.
- Метрики latency/errors, размера batches, retries и насыщения connection pool.

---

## Официальная документация

- [clickhouse-go](https://github.com/ClickHouse/clickhouse-go)
- [Go integration](https://clickhouse.com/docs/integrations/language-clients/go)
