# Connection Pooling

## Содержание

- [Почему PostgreSQL дорого обходятся соединения](#почему-postgresql-дорого-обходятся-соединения)
- [PgBouncer: режимы и настройка](#pgbouncer-режимы-и-настройка)
- [Prepared statements и PgBouncer](#prepared-statements-и-pgbouncer)
- [pgxpool: пул на стороне Go](#pgxpool-пул-на-стороне-go)
- [PgBouncer vs pgxpool: когда что](#pgbouncer-vs-pgxpool-когда-что)
- [Расчёт числа соединений](#расчёт-числа-соединений)
- [Мониторинг пула](#мониторинг-пула)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

---

## Почему PostgreSQL дорого обходятся соединения

PostgreSQL использует **process-per-connection** модель: каждое соединение порождает отдельный процесс (~5–10MB RSS). При 500 соединениях это 2.5–5GB только на процессы.

Дополнительные накладные расходы:
- Context switching между процессами.
- Каждый backend-процесс держит собственный кеш каталога.
- Семафоры и shared memory locks.

**PostgreSQL не предназначен для работы с тысячами прямых соединений.** Рекомендуется: 20–50 реальных соединений к PostgreSQL, перед которыми стоит пул.

---

## PgBouncer: режимы и настройка

PgBouncer — легковесный proxy (одна C-программа, ~1MB RAM на 1000 соединений клиентов).

### Режимы работы

| Режим | Соединение с PG | Когда освобождается | Поддержка |
|---|---|---|---|
| `session` | Одно на сессию клиента | При disconnect клиента | Полная |
| `transaction` | Одно на транзакцию | После COMMIT/ROLLBACK | Частичная |
| `statement` | Одно на statement | После каждого запроса | Минимальная |

**Рекомендуется: `transaction` mode** — максимальная эффективность мультиплексирования.

В transaction mode **нельзя** использовать:
- `SET` / `RESET` — настройки теряются (каждый раз может быть другое соединение).
- `PREPARE` / `EXECUTE` именованных prepared statements — они привязаны к соединению.
- `LISTEN` / `NOTIFY` — привязаны к соединению.
- `SAVEPOINT` вне транзакции.
- `pg_advisory_lock` без transaction-level variant.

### pgbouncer.ini

```ini
[databases]
mydb = host=postgres port=5432 dbname=mydb

[pgbouncer]
listen_port = 5432
listen_addr = *

pool_mode = transaction
max_client_conn = 2000        ; соединений клиентов
default_pool_size = 20        ; реальных соединений к PG per database/user

min_pool_size = 5
reserve_pool_size = 5         ; extra в пиках
reserve_pool_timeout = 3      ; секунд ждать резервный пул

auth_type = scram-sha-256
auth_file = /etc/pgbouncer/userlist.txt

server_idle_timeout = 600     ; закрыть idle соединение к PG через N секунд
client_idle_timeout = 0       ; не закрывать idle клиентские (0 = off)
server_lifetime = 3600        ; максимальное время жизни соединения к PG

log_connections = 0
log_disconnections = 0
log_pooler_errors = 1

; пул per user/database (переопределяет default_pool_size)
[mydb/analytics_user]
pool_size = 5
pool_mode = session           ; analytics user нужны сессионные features
```

---

## Prepared statements и PgBouncer

В `transaction mode` именованные prepared statements не работают, т.к. привязаны к серверному соединению, а каждая транзакция может идти через разное соединение.

**Решения:**

1. **Unnamed prepared statements** (protocol-level, `$1`, `$2`) — pgx использует их автоматически, они не кешируются на сервере между транзакциями.

2. **Disable prepared statements в pgx:**

```go
config, _ := pgxpool.ParseConfig(dsn)
config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
```

3. **Использовать session mode для нужных соединений** — отдельный пул PgBouncer.

4. **pgBouncer `prepare_threshold`** — в PgBouncer 1.22+ добавили поддержку protocol-level prepared statements в transaction mode. Проверить версию.

В Go с pgx v5 — по умолчанию используется extended query protocol с cache на уровне соединения. При работе через PgBouncer в transaction mode рекомендуется:

```go
config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
// или
config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
```

---

## pgxpool: пул на стороне Go

`pgxpool` — встроенный пул соединений в pgx. Работает без PgBouncer.

```go
import "github.com/jackc/pgx/v5/pgxpool"

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
    config, err := pgxpool.ParseConfig(dsn)
    if err != nil {
        return nil, err
    }

    config.MaxConns = 20              // максимум соединений
    config.MinConns = 2               // держать минимум всегда открытыми
    config.MaxConnLifetime = 1 * time.Hour   // пересоздавать соединения
    config.MaxConnIdleTime = 30 * time.Minute
    config.HealthCheckPeriod = 1 * time.Minute

    // хук после создания соединения
    config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
        // например: SET search_path = myschema
        return nil
    }

    return pgxpool.NewWithConfig(ctx, config)
}
```

### Acquire и Release

```go
// Acquire: получить соединение из пула (возвращает его при Release)
conn, err := pool.Acquire(ctx)
if err != nil {
    return err
}
defer conn.Release()

// conn.Conn() — доступ к *pgx.Conn
rows, err := conn.Query(ctx, "SELECT ...")
```

Обычно acquire не нужен — методы `pool.Query`, `pool.Exec`, `pool.QueryRow` сами берут и возвращают соединение.

### Stats

```go
stats := pool.Stat()
fmt.Printf("acquired: %d, idle: %d, total: %d\n",
    stats.AcquiredConns(), stats.IdleConns(), stats.TotalConns())
```

---

## PgBouncer vs pgxpool: когда что

| Сценарий | Рекомендация |
|---|---|
| Один Go сервис, 1–3 инстанса | pgxpool достаточно |
| Много инстансов (10+) | PgBouncer: агрегирует соединения |
| Разные сервисы к одной БД | PgBouncer: общий пул |
| Нужны SET / advisory locks | PgBouncer в session mode или pgxpool |
| Kubernetes с частым scale in/out | PgBouncer поглощает connection churn |
| Максимальная простота | pgxpool, без PgBouncer |

Типичная production архитектура:

```
[App instances x10] → [PgBouncer x2] → [PostgreSQL Primary]
                                     → [PostgreSQL Replica]
```

PgBouncer в transaction mode держит 20–30 реальных соединений к PG, принимает 2000+ клиентских.

---

## Расчёт числа соединений

Формула для `MaxConns` в pgxpool или `default_pool_size` в PgBouncer:

```
N = max(2, num_cpu_cores * 2)  # для OLTP workload
```

Пример для сервера с 8 ядрами:
- `max_connections = 200` (PostgreSQL)
- PgBouncer `default_pool_size = 20` (реальные соединения к PG)
- PgBouncer `max_client_conn = 2000` (соединения от приложений)
- pgxpool `MaxConns = 10` per инстанс приложения

Не нужно устанавливать большой пул в надежде на параллелизм — PostgreSQL не может параллельно использовать 200 соединений эффективнее, чем 20. Bottleneck — диск и CPU, а не число соединений.

### Ловушка: пул считается на под, а не на сервис

`pgxpool.MaxConns` — лимит **на процесс** (на под). Поды не знают друг о друге, каждый держит свой пул к Postgres. Поэтому суммарное число соединений к базе:

```
всего к PG = MaxConns × число подов
20         × 100 подов = 2000 соединений
```

А `MaxConns = 20` в манифесте выглядит невинно. Но каждое соединение к Postgres — это отдельный backend-**процесс** на сервере БД (память + свой `work_mem`), и их число жёстко ограничено `max_connections` (по умолчанию ~100). 2000 попыток → `FATAL: sorry, too many clients already`, и падают **все** поды, а не только «лишний».

Почему на многих подах pgxpool не спасает в принципе: формула `cores × 2` даёт **суммарно полезное** число соединений на всю базу (для 8 ядер ~16–20). Разделить эти 20 между 100 подами нельзя — вышло бы <1 на под. Отсюда правило:

- **1–3 инстанса** — pgxpool сам по себе ок, следи лишь, чтобы `MaxConns × инстансы < max_connections`;
- **10+ подов / частый autoscale** — pgxpool в одиночку не масштабируется, нужен **PgBouncer** в transaction mode: поды коннектятся к нему (дёшево, `max_client_conn = 2000+`), а он мультиплексирует их на маленький пул реальных backend'ов (`default_pool_size = 20–30`).

То есть PgBouncer не «ускоряет», а **развязывает** число подов от числа backend-процессов Postgres.

---

## Мониторинг пула

PgBouncer stats (подключиться к pgbouncer database):

```sql
-- соединить к pgbouncer (порт 6432 обычно)
SHOW POOLS;
-- cl_active, cl_waiting, sv_active, sv_idle, sv_used, maxwait

SHOW STATS;
-- total_requests, total_received, total_sent, avg_query

SHOW CLIENTS;
-- список клиентских соединений

SHOW SERVERS;
-- список реальных соединений к PG
```

Сигналы проблем:
- `cl_waiting > 0` долго — не хватает пула, увеличить `pool_size`.
- `maxwait` растёт — запросы ждут свободного соединения.
- `sv_idle` = 0 — все соединения заняты.

PostgreSQL: число активных соединений:

```sql
SELECT count(*) FROM pg_stat_activity WHERE state != 'idle';
SELECT count(*), state FROM pg_stat_activity GROUP BY state ORDER BY count DESC;
```

---

## Типичные ошибки

- **Слишком большой MaxConns** на каждом инстансе приложения — при горизонтальном масштабировании суммарное число соединений превышает `max_connections`.
- **Не закрывать rows** в pgx — соединение остаётся занятым, пул истощается.
- **Использовать PgBouncer transaction mode с SET** — настройки теряются.
- **Не задавать MaxConnLifetime** — старые соединения могут иметь проблемы (сеть, auth, etc).
- **Игнорировать pool exhaustion** — при timeout(`pgxpool.Pool.Acquire`) приложение возвращает ошибку вместо ожидания. Нужно мониторить `AcquiredConns / MaxConns`.

---

## Interview-ready answer

**1. Почему соединения в PostgreSQL дорогие?**

- Process-per-connection: каждое соединение — отдельный процесс ~5–10 MB + context switching и свой кеш каталога; при сотнях прямых соединений overhead значителен.

**2. Что даёт PgBouncer в transaction mode?**

- Принимает тысячи клиентских соединений, держит к PG лишь 20–30 реальных, возвращая соединение в пул после каждой транзакции — максимальное мультиплексирование.

**3. Чего нельзя в transaction mode?**

- Сессионных вещей: `SET`/`RESET`, именованные prepared statements, `LISTEN`/`NOTIFY`, session-level advisory locks — они привязаны к соединению.

**4. Как настроить pgxpool?**

- `MaxConns` ≈ 10–20, `MinConns`, `MaxConnLifetime`, `MaxConnIdleTime`, `HealthCheckPeriod`; для PgBouncer transaction mode — `QueryExecModeCacheDescribe`/`SimpleProtocol`.

**5. Как считать число соединений?**

- `MaxConns ≈ CPU_cores × 2`; bottleneck — диск и CPU, а не число соединений, 200+ к PG не нужны; суммарный пул всех инстансов не должен превышать `max_connections`.

**6. pgxpool или PgBouncer?**

- Один-три инстанса — хватит pgxpool; много инстансов / общий доступ нескольких сервисов / k8s с частым scale — PgBouncer агрегирует пулы перед базой.
