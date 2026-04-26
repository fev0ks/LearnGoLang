# Транзакции и блокировки

## Содержание

- [Isolation levels в PostgreSQL](#isolation-levels-в-postgresql)
- [Аномалии и как PG их предотвращает](#аномалии-и-как-pg-их-предотвращает)
- [Типы блокировок таблиц](#типы-блокировок-таблиц)
- [Row-level locks](#row-level-locks)
- [SELECT FOR UPDATE / FOR SHARE](#select-for-update--for-share)
- [SKIP LOCKED — паттерн очереди](#skip-locked--паттерн-очереди)
- [Advisory locks](#advisory-locks)
- [Deadlock: как возникает и как лечить](#deadlock-как-возникает-и-как-лечить)
- [DDL и locks](#ddl-и-locks)
- [Мониторинг блокировок](#мониторинг-блокировок)
- [Interview-ready answer](#interview-ready-answer)

---

## Isolation levels в PostgreSQL

PostgreSQL реализует три уровня изоляции (четвёртый `READ UNCOMMITTED` работает как `READ COMMITTED`):

| Уровень | Dirty read | Non-repeatable read | Phantom read | Write skew |
|---|---|---|---|---|
| `READ COMMITTED` (default) | нет | возможен | возможен | возможен |
| `REPEATABLE READ` | нет | нет | нет\* | возможен |
| `SERIALIZABLE` | нет | нет | нет | нет |

\* В PostgreSQL phantom read тоже предотвращается на REPEATABLE READ из-за snapshot.

Как задать:
```sql
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ;
-- или
BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;
```

---

## Аномалии и как PG их предотвращает

### Dirty Read

Читаем незакоммиченные данные другой транзакции. В PostgreSQL **никогда не происходит** — MVCC всегда показывает только закоммиченные данные.

### Non-repeatable Read

```sql
-- Транзакция A (READ COMMITTED):
BEGIN;
SELECT balance FROM accounts WHERE id = 1;  -- получили 100

-- Транзакция B коммитит UPDATE: balance = 200

SELECT balance FROM accounts WHERE id = 1;  -- получили 200 (!)
COMMIT;
```

На `REPEATABLE READ` второй SELECT возвращает 100 — снапшот зафиксирован в начале транзакции.

### Phantom Read

```sql
-- Транзакция A (READ COMMITTED):
BEGIN;
SELECT COUNT(*) FROM orders WHERE user_id = 5;  -- 3

-- Транзакция B коммитит INSERT нового заказа

SELECT COUNT(*) FROM orders WHERE user_id = 5;  -- 4 (!)
COMMIT;
```

На `REPEATABLE READ` в PostgreSQL оба COUNT вернут 3.

### Write Skew

Два читают одни данные, принимают решение независимо, и оба пишут — нарушая инвариант.

```sql
-- Инвариант: в смене должен быть хотя бы один врач дежурный
-- Транзакция A: доктор Иванов видит дежурит Петров → снимает себя
-- Транзакция B: доктор Петров видит дежурит Иванов → снимает себя
-- Оба закоммитили → никто не дежурит
```

Решения:
- `SERIALIZABLE` — SSI обнаружит конфликт и откатит одну из транзакций.
- `SELECT FOR UPDATE` — явная блокировка строк.
- Пессимистическая блокировка через advisory lock.

### Serialization anomaly (SSI)

`SERIALIZABLE` в PostgreSQL реализован через SSI (Serializable Snapshot Isolation) — не блокирует, а отслеживает зависимости и при обнаружении цикла откатывает транзакцию с ошибкой `40001 serialization_failure`. Приложение должно повторять транзакцию.

```go
for {
    err := doSerializableTransaction(ctx, pool)
    if isSerializationFailure(err) {
        continue // retry
    }
    break
}

func isSerializationFailure(err error) bool {
    var pgErr *pgconn.PgError
    return errors.As(err, &pgErr) && pgErr.Code == "40001"
}
```

---

## Типы блокировок таблиц

PostgreSQL использует таблицу `pg_locks` для отслеживания. Для таблиц — 8 уровней (от слабого к сильному):

| Lock Mode | Совместим с | Получается при |
|---|---|---|
| `ACCESS SHARE` | всеми, кроме ACCESS EXCLUSIVE | SELECT |
| `ROW SHARE` | всеми, кроме EXCLUSIVE, ACCESS EXCLUSIVE | SELECT FOR UPDATE/SHARE |
| `ROW EXCLUSIVE` | ACCESS SHARE, ROW SHARE | INSERT, UPDATE, DELETE |
| `SHARE UPDATE EXCLUSIVE` | ACCESS SHARE, ROW SHARE, ROW EXCLUSIVE | VACUUM, CREATE INDEX CONCURRENTLY |
| `SHARE` | ACCESS SHARE, ROW SHARE, SHARE | CREATE INDEX (без CONCURRENTLY) |
| `SHARE ROW EXCLUSIVE` | ACCESS SHARE, ROW SHARE | некоторые DDL |
| `EXCLUSIVE` | ACCESS SHARE | редко в MVCC |
| `ACCESS EXCLUSIVE` | никем | ALTER TABLE, DROP TABLE, TRUNCATE, LOCK TABLE |

Практический вывод: обычные DML (SELECT/INSERT/UPDATE/DELETE) совместимы друг с другом. `ALTER TABLE` блокирует всё.

---

## Row-level locks

PostgreSQL поддерживает 4 режима блокировки строк:

| Режим | Конфликтует с | Когда используется |
|---|---|---|
| `FOR KEY SHARE` | FOR UPDATE, FOR NO KEY UPDATE | Foreign key checks |
| `FOR SHARE` | FOR UPDATE, FOR NO KEY UPDATE | Чтение с защитой от UPDATE |
| `FOR NO KEY UPDATE` | FOR UPDATE, FOR NO KEY UPDATE, FOR SHARE | UPDATE без изменения PK |
| `FOR UPDATE` | всеми | Явная блокировка для update |

---

## SELECT FOR UPDATE / FOR SHARE

`SELECT FOR UPDATE` — захватить строку как "буду обновлять". Блокирует другие `FOR UPDATE` до COMMIT/ROLLBACK.

```sql
BEGIN;
SELECT balance FROM accounts WHERE id = $1 FOR UPDATE;
-- другие транзакции с FOR UPDATE на ту же строку ждут
UPDATE accounts SET balance = balance - $2 WHERE id = $1;
COMMIT;
```

`FOR UPDATE OF table` — в join блокировать строки только из конкретной таблицы:

```sql
SELECT o.id, u.email
FROM orders o
JOIN users u ON u.id = o.user_id
WHERE o.id = $1
FOR UPDATE OF o;  -- блокируем только orders, не users
```

`FOR SHARE` — "буду читать, не удаляйте". Блокирует DELETE/UPDATE, но не другие FOR SHARE.

Timeout для блочащего ожидания:

```sql
SET lock_timeout = '5s';
SELECT * FROM orders WHERE id = $1 FOR UPDATE;
-- ERROR: canceling statement due to lock timeout
```

---

## SKIP LOCKED — паттерн очереди

`SKIP LOCKED` — пропустить заблокированные строки вместо ожидания. Идеален для очередей задач с несколькими воркерами.

```sql
-- воркер берёт одну задачу из очереди
BEGIN;
SELECT id, payload
FROM tasks
WHERE status = 'pending'
ORDER BY created_at
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- если нашли — обрабатываем
UPDATE tasks SET status = 'processing', worker_id = $1 WHERE id = $2;
COMMIT;
```

Несколько воркеров могут параллельно забирать задачи — SKIP LOCKED гарантирует, что разные воркеры получат разные строки.

```sql
-- взять батч из 10 задач
SELECT id FROM tasks
WHERE status = 'pending'
ORDER BY priority DESC, created_at
LIMIT 10
FOR UPDATE SKIP LOCKED;
```

---

## Advisory locks

Advisory locks — приложение само управляет захватом/освобождением. PostgreSQL только хранит состояние. Полезны для распределённых mutex'ов.

```sql
-- захватить блокировку (блокирующий вариант)
SELECT pg_advisory_lock(12345);

-- попробовать захватить (неблокирующий)
SELECT pg_try_advisory_lock(12345);  -- TRUE если захвачено

-- освободить
SELECT pg_advisory_unlock(12345);

-- session-level: держится до конца сессии или явного unlock
-- transaction-level: освобождается при COMMIT/ROLLBACK
SELECT pg_advisory_xact_lock(12345);  -- transaction-level
```

Пример в Go — глобальный mutex для cron-задачи:

```go
const lockKey = 42

func runOnce(ctx context.Context, pool *pgxpool.Pool) error {
    conn, err := pool.Acquire(ctx)
    if err != nil {
        return err
    }
    defer conn.Release()

    var locked bool
    err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&locked)
    if err != nil {
        return err
    }
    if !locked {
        return nil // другой инстанс выполняет
    }
    defer conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockKey)

    return doWork(ctx)
}
```

---

## Deadlock: как возникает и как лечить

Deadlock — A ждёт B, B ждёт A.

```sql
-- Транзакция A:
BEGIN;
UPDATE accounts SET balance = balance - 100 WHERE id = 1;  -- блокирует строку 1
UPDATE accounts SET balance = balance + 100 WHERE id = 2;  -- ждёт строку 2

-- Транзакция B (одновременно):
BEGIN;
UPDATE accounts SET balance = balance - 100 WHERE id = 2;  -- блокирует строку 2
UPDATE accounts SET balance = balance + 100 WHERE id = 1;  -- ждёт строку 1 → DEADLOCK
```

PostgreSQL автоматически обнаруживает deadlock (через `deadlock_timeout`, default 1s) и откатывает одну из транзакций с ошибкой `40P01 deadlock_detected`.

Как избежать:
1. **Фиксированный порядок блокировки** — всегда обновлять строки в одном порядке (по id).
2. **SELECT FOR UPDATE** в начале транзакции — зафиксировать все нужные строки до обработки.
3. **Короткие транзакции** — меньше времени держат блокировки.

```go
func transfer(ctx context.Context, tx pgx.Tx, fromID, toID int64, amount int) error {
    // всегда блокировать в порядке возрастания id
    ids := []int64{fromID, toID}
    sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

    for _, id := range ids {
        _, err := tx.Exec(ctx, "SELECT 1 FROM accounts WHERE id = $1 FOR UPDATE", id)
        if err != nil {
            return err
        }
    }
    // теперь оба ряда заблокированы безопасно
    _, err := tx.Exec(ctx, "UPDATE accounts SET balance = balance - $1 WHERE id = $2", amount, fromID)
    if err != nil {
        return err
    }
    _, err = tx.Exec(ctx, "UPDATE accounts SET balance = balance + $1 WHERE id = $2", amount, toID)
    return err
}
```

---

## DDL и locks

ALTER TABLE требует `ACCESS EXCLUSIVE` — блокирует все запросы до завершения.

Безопасные DDL без долгих блокировок:
- `ADD COLUMN` с nullable и без default — мгновенно (только каталог).
- `ADD COLUMN` с DEFAULT (PG 11+) — мгновенно (default сохраняется в каталоге).
- `CREATE INDEX CONCURRENTLY` — не блокирует DML.
- `ADD CONSTRAINT ... NOT VALID` + отдельный `VALIDATE CONSTRAINT` — минимальная блокировка.

Опасные операции:
- `ADD COLUMN ... NOT NULL DEFAULT expr` (PG < 11) — перезаписывает все строки.
- `ALTER COLUMN TYPE` — перезаписывает таблицу.
- `ADD CONSTRAINT FOREIGN KEY` (без `NOT VALID`) — долгий scan + lock.

```sql
-- безопасное добавление NOT NULL с default
-- шаг 1: nullable column — мгновенно
ALTER TABLE users ADD COLUMN phone TEXT;

-- шаг 2: заполнить данные батчами
UPDATE users SET phone = '' WHERE phone IS NULL AND id BETWEEN 1 AND 10000;
-- ... батчи

-- шаг 3: SET NOT NULL с проверкой constraint (PG 12+: fast если все ненулевые)
ALTER TABLE users ALTER COLUMN phone SET NOT NULL;
```

```sql
-- безопасное добавление FK
ALTER TABLE orders ADD CONSTRAINT fk_orders_user
    FOREIGN KEY (user_id) REFERENCES users(id) NOT VALID;

-- отдельно: VALIDATE берёт ShareUpdateExclusiveLock (не блокирует DML)
ALTER TABLE orders VALIDATE CONSTRAINT fk_orders_user;
```

---

## Мониторинг блокировок

Текущие ожидающие блокировки:

```sql
SELECT
    blocked.pid AS blocked_pid,
    blocked.query AS blocked_query,
    blocking.pid AS blocking_pid,
    blocking.query AS blocking_query,
    now() - blocked.query_start AS wait_duration
FROM pg_stat_activity AS blocked
JOIN pg_stat_activity AS blocking
    ON blocking.pid = ANY(pg_blocking_pids(blocked.pid))
WHERE blocked.wait_event_type = 'Lock';
```

Детальный просмотр pg_locks:

```sql
SELECT
    l.pid, l.mode, l.granted, l.relation::regclass AS table,
    a.query, a.state
FROM pg_locks l
JOIN pg_stat_activity a ON a.pid = l.pid
WHERE NOT l.granted
ORDER BY l.pid;
```

---

## Interview-ready answer

PostgreSQL поддерживает три уровня изоляции: READ COMMITTED (default) — снапшот per-statement, REPEATABLE READ — снапшот per-transaction (нет non-repeatable read и phantom read), SERIALIZABLE — SSI, нет write skew но нужен retry на `40001`. Row locks: FOR UPDATE блокирует строку для конкурентного UPDATE, SKIP LOCKED пропускает заблокированные строки — стандартный паттерн для queue workers с PostgreSQL. DDL `ALTER TABLE` требует ACCESS EXCLUSIVE — для production использовать `ADD COLUMN` без DEFAULT, `CREATE INDEX CONCURRENTLY`, `NOT VALID` FK. Deadlock: PostgreSQL обнаруживает автоматически за `deadlock_timeout` (1s), откатывает одну транзакцию. Предотвращение: фиксированный порядок блокировки строк. Advisory locks — для distributed mutex без дополнительных систем.
