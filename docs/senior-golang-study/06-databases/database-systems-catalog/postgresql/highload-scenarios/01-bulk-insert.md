# Сценарий: массовая вставка

**Проблема**: нужно загрузить миллионы строк — импорт из файла, миграция, ночной ETL, бэкфилл новой колонки/таблицы. Наивный `INSERT` по одной строке на 10M записей идёт часами: каждый `INSERT` — отдельный round-trip, отдельная запись в WAL, отдельный fsync (при autocommit) и обновление всех индексов.

Здесь — как ускоряют такую загрузку на порядки и какой ценой. Базовый COPY-протокол из Go — в [11-go-patterns.md](../11-go-patterns.md), параметры сервера — в [08-performance-tuning.md](../08-performance-tuning.md).

## Содержание

- [Сравнение способов вставки](#сравнение-способов-вставки)
- [COPY — основной инструмент](#copy--основной-инструмент)
- [Multi-row INSERT, когда COPY нельзя](#multi-row-insert-когда-copy-нельзя)
- [Что тормозит вставку и как это снять](#что-тормозит-вставку-и-как-это-снять)
  - [Индексы: строить после, а не во время](#индексы-строить-после-а-не-во-время)
  - [WAL и synchronous_commit](#wal-и-synchronous_commit)
  - [UNLOGGED и загрузка в партицию](#unlogged-и-загрузка-в-партицию)
- [Идемпотентный перезапуск загрузки](#идемпотентный-перезапуск-загрузки)
- [Подводные камни](#подводные-камни)
- [Interview-ready answer](#interview-ready-answer)

---

## Сравнение способов вставки

Порядок величин (не абсолютные числа — зависят от железа, ширины строки, индексов), от худшего к лучшему:

| Способ | Относительная скорость | Когда |
|--------|------------------------|-------|
| `INSERT` по 1 строке, autocommit | 1× (база) | никогда для bulk |
| `INSERT` по 1 строке в одной транзакции | ~5–10× | если строк немного |
| Multi-row `INSERT` (1000 строк за оператор) | ~50–100× | когда нужен `ON CONFLICT`/`RETURNING` |
| `COPY` (`CopyFrom`) | ~200–500× | дефолт для массовой загрузки |
| `COPY` в `UNLOGGED`/новую партицию без индексов | ещё ×2–5 | разовый импорт, можно потерять при краше |

Главный вывод: **разница не в «настроить параметр», а в смене механизма**. Построчный INSERT упирается в количество round-trip'ов и fsync'ов; COPY передаёт всё одним потоком и пишет пачкой.

```mermaid
graph TB
    subgraph slow["INSERT по строке (autocommit)"]
        S1["round-trip"] --> S2["WAL + fsync"] --> S3["обновить все индексы"] --> S1
    end
    subgraph fast["COPY"]
        F1["один поток данных"] --> F2["пачка в heap"] --> F3["один WAL-flush на транзакцию"]
    end
```

---

## COPY — основной инструмент

`COPY` (в pgx — `CopyFrom`) — это отдельный протокол потоковой загрузки: клиент шлёт строки потоком, сервер кладёт их в heap минуя обычный путь парсинга `INSERT`. В 5–50× быстрее построчного INSERT (детали API — в [11-go-patterns.md](../11-go-patterns.md)).

```go
// загрузка из среза через CopyFromRows
func bulkInsert(ctx context.Context, pool *pgxpool.Pool, users []User) (int64, error) {
    rows := make([][]any, len(users))
    for i, u := range users {
        rows[i] = []any{u.Email, u.Status, u.CreatedAt}
    }
    n, err := pool.CopyFrom(ctx,
        pgx.Identifier{"users"},
        []string{"email", "status", "created_at"},
        pgx.CopyFromRows(rows),
    )
    return n, err   // n — число вставленных строк
}
```

**Потоковая загрузка** (не держим все строки в памяти — важно для гигабайтных файлов): реализуем `pgx.CopyFromSource`, читая по строке из файла/курсора.

```go
// источник, который читает CSV построчно — память O(1), а не O(N)
type csvSource struct {
    r   *csv.Reader
    row []any
    err error
}

func (s *csvSource) Next() bool {
    rec, err := s.r.Read()
    if err != nil {           // io.EOF или ошибка парсинга
        if err != io.EOF {
            s.err = err
        }
        return false
    }
    qty, _ := strconv.Atoi(rec[1])
    s.row = []any{rec[0], qty}
    return true
}
func (s *csvSource) Values() ([]any, error) { return s.row, nil }
func (s *csvSource) Err() error             { return s.err }

func loadCSV(ctx context.Context, pool *pgxpool.Pool, f io.Reader) (int64, error) {
    src := &csvSource{r: csv.NewReader(f)}
    return pool.CopyFrom(ctx,
        pgx.Identifier{"orders"},
        []string{"sku", "qty"},
        src,                       // pgx тянет Next()/Values() по мере отправки
    )
}
```

> COPY — **одна транзакция целиком**: если на 9-миллионной строке ошибка, откатываются все. Для очень больших файлов load бьют на чанки (по N строк = N отдельных COPY), чтобы ошибка не стоила всей работы и чтобы был прогресс. Это же даёт точку идемпотентного перезапуска (ниже).

---

## Multi-row INSERT, когда COPY нельзя

COPY не умеет `ON CONFLICT`, `RETURNING`, выражения и DEFAULT-генерацию на лету. Если они нужны — берут **multi-row INSERT** (один оператор, много кортежей): это всё ещё один round-trip и один разбор плана на пачку.

```go
// вставка пачкой с ON CONFLICT — строим один INSERT с N группами VALUES
func upsertBatch(ctx context.Context, pool *pgxpool.Pool, users []User) error {
    const cols = 3
    args := make([]any, 0, len(users)*cols)
    var b strings.Builder
    b.WriteString("INSERT INTO users (email, status, created_at) VALUES ")
    for i, u := range users {
        if i > 0 {
            b.WriteByte(',')
        }
        // $1,$2,$3),($4,$5,$6),...
        fmt.Fprintf(&b, "($%d,$%d,$%d)", i*cols+1, i*cols+2, i*cols+3)
        args = append(args, u.Email, u.Status, u.CreatedAt)
    }
    b.WriteString(" ON CONFLICT (email) DO UPDATE SET status = EXCLUDED.status")
    _, err := pool.Exec(ctx, b.String(), args...)
    return err
}
```

Подвох: у Postgres лимит **65535 параметров** на запрос (`$1..$65535`). При 3 колонках это ≤ 21845 строк за оператор — чанк нужно держать ниже. `pgx.Batch` тут не помогает с лимитом плана, но экономит round-trip'ы, отправляя пачку операторов сразу.

---

## Что тормозит вставку и как это снять

### Индексы: строить после, а не во время

Каждый существующий индекс обновляется на **каждую** вставленную строку — на больших загрузках это доминирующая стоимость. Для разовой массовой загрузки выгоднее: снять индексы → COPY → построить заново (build по готовым данным сильно быстрее, чем N точечных вставок в индекс).

```sql
-- разовый импорт в (полу)пустую таблицу
DROP INDEX idx_orders_user_id;          -- или начать с таблицы вообще без вторичных индексов
-- ... COPY ...
CREATE INDEX CONCURRENTLY idx_orders_user_id ON orders (user_id);
-- поднять maintenance_work_mem перед build — ускоряет сортировку индекса
SET maintenance_work_mem = '2GB';
```

Подвох: на **живой** таблице (не пустой) снимать индексы нельзя — запросы деградируют до Seq Scan. Этот приём — для начальной загрузки/изолированной таблицы, которую потом подключают (см. ATTACH ниже).

### WAL и synchronous_commit

Каждая транзакция по умолчанию ждёт fsync WAL на диск (`synchronous_commit = on`). Для bulk-load это лишняя гарантия — данные всё равно перезагрузим при сбое:

```sql
SET synchronous_commit = off;   -- не ждём fsync каждой транзакции (риск — потеря последних ~200мс при краше)
SET work_mem = '256MB';         -- если в загрузке есть сортировки/дедуп
```

`synchronous_commit = off` не нарушает целостность (никакого corruption), теряются лишь последние закоммиченные транзакции при падении — для перезапускаемой загрузки приемлемо.

### UNLOGGED и загрузка в партицию

Два более агрессивных приёма:

- **`UNLOGGED`-таблица** — не пишется в WAL вообще (ещё быстрее), но **не переживает краш** и не реплицируется. Подходит для стейджинга: грузим в `UNLOGGED`, обрабатываем, затем `SET LOGGED` или переливаем в основную.
- **Загрузка в отдельную таблицу + ATTACH** — для партиционированных: грузим в обычную таблицу (быстро, без routing-оверхеда и под своими индексами), затем атомарно подключаем как партицию (механика и почему нет `ATTACH ... CONCURRENTLY` — в [05-partitioning.md](../05-partitioning.md)).

```sql
-- staging-таблица как клон, без участия в партиционировании
CREATE UNLOGGED TABLE orders_import (LIKE orders INCLUDING DEFAULTS);
-- ... COPY orders_import ...
CREATE INDEX ON orders_import (user_id);
ALTER TABLE orders_import SET LOGGED;             -- теперь данные в WAL
-- CHECK заранее → ATTACH без скана
ALTER TABLE orders_import ADD CONSTRAINT ck CHECK (created_at >= '2025-02-01' AND created_at < '2025-03-01');
ALTER TABLE orders ATTACH PARTITION orders_import FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');
```

---

## Идемпотентный перезапуск загрузки

Большая загрузка падает на середине (сеть, OOM, рестарт пода) — её нужно безопасно **перезапустить, не задвоив** уже вставленное. Подходы:

1. **Чанки + журнал прогресса.** Бьём вход на чанки с детерминированными границами (по диапазону id/строк файла) и отмечаем выполненные:

```go
// каждый чанк грузим в своей транзакции и фиксируем offset
func loadChunked(ctx context.Context, pool *pgxpool.Pool, chunks []Chunk) error {
    for _, c := range chunks {
        done, err := chunkDone(ctx, pool, c.ID)   // SELECT из таблицы прогресса
        if err != nil {
            return err
        }
        if done {
            continue                               // уже загружен — пропускаем (идемпотентность)
        }
        tx, err := pool.Begin(ctx)
        if err != nil {
            return err
        }
        if err := copyChunk(ctx, tx, c); err != nil {
            tx.Rollback(ctx)
            return err
        }
        // отметка прогресса — в той же транзакции, что и данные
        if _, err := tx.Exec(ctx,
            "INSERT INTO import_progress (chunk_id) VALUES ($1)", c.ID); err != nil {
            tx.Rollback(ctx)
            return err
        }
        if err := tx.Commit(ctx); err != nil {
            return err
        }
    }
    return nil
}
```

2. **COPY в staging + `INSERT ... ON CONFLICT DO NOTHING` в основную** — повторный прогон не задвоит за счёт уникального ключа (паттерн upsert — в [03-upsert-at-scale.md](./03-upsert-at-scale.md)).

Ключевая идея: отметка «чанк загружен» должна коммититься **в одной транзакции** с самими данными — иначе возможен зазор «данные есть, отметки нет» (задвоение) или наоборот.

---

## Подводные камни

- **COPY — всё или ничего**: ошибка в одной строке откатывает весь COPY. Валидируй данные до загрузки или грузи чанками.
- **Лимит 65535 параметров** у multi-row INSERT — держи размер чанка с запасом (`строк × колонок < 65535`).
- **Триггеры и FK срабатывают на каждую строку** и при COPY тоже — на bulk это дорого; иногда триггеры временно отключают (`ALTER TABLE ... DISABLE TRIGGER`) и проверяют инварианты пакетно после.
- **Autovacuum/ANALYZE после загрузки**: статистика после массовой вставки устаревает → планировщик ошибается. После bulk обязательно `ANALYZE` (или `VACUUM ANALYZE`).
- **`UNLOGGED` молча теряет данные при краше** — только для стейджинга/восстановимых данных, не для основной таблицы.
- **Снятие индексов на живой таблице** деградирует прод-запросы — приём только для изолированной/начальной загрузки.
- **Раздувание WAL и репликация**: большая загрузка генерирует много WAL → лаг реплик и риск переполнения слотов; помогает `wal_compression`, разбиение на чанки и контроль за лагом (см. [06-replication.md](../06-replication.md)).

---

## Interview-ready answer

**1. Как быстро вставить миллионы строк в PostgreSQL?**

- Не построчным INSERT, а сменой механизма: `COPY` (`pgx.CopyFrom`) — потоковый протокол, в 5–50× быстрее; если нужен `ON CONFLICT`/`RETURNING` — multi-row INSERT пачками (≤ 65535 параметров на оператор).

**2. Что доминирует в стоимости bulk-вставки?**

- Обновление индексов на каждую строку, round-trip'ы и fsync WAL; снимаются: строить индексы после загрузки, `synchronous_commit = off`, повышенный `maintenance_work_mem` для build.

**3. Когда UNLOGGED и загрузка в партицию?**

- `UNLOGGED` — стейджинг (не пишется в WAL, но теряется при краше); для партиционированных таблиц — грузить в отдельную таблицу под своими индексами и атомарно `ATTACH` (с заранее навешенным `CHECK`, чтобы без скана).

**4. Почему COPY рискован на большом файле и как страховаться?**

- COPY — одна транзакция: ошибка откатывает всё; грузят чанками, фиксируя прогресс в той же транзакции, что и данные, — это даёт идемпотентный перезапуск без задвоения.

**5. Что не забыть после массовой загрузки?**

- `ANALYZE` (статистика устарела → плохие планы), вернуть `synchronous_commit`/`UNLOGGED`-таблицу в нормальный режим, проверить лаг реплик (bulk генерирует много WAL).
