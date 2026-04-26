# Репликация в PostgreSQL

## Содержание

- [WAL: основа репликации](#wal-основа-репликации)
- [Streaming Replication (физическая)](#streaming-replication-физическая)
- [Logical Replication](#logical-replication)
- [Synchronous vs Asynchronous](#synchronous-vs-asynchronous)
- [Read Replicas](#read-replicas)
- [Replication Slots](#replication-slots)
- [HA и Failover: Patroni](#ha-и-failover-patroni)
- [Типичные проблемы](#типичные-проблемы)
- [Interview-ready answer](#interview-ready-answer)

---

## WAL: основа репликации

WAL (Write-Ahead Log) — журнал всех изменений в PostgreSQL. Каждое изменение (INSERT, UPDATE, DELETE, DDL) записывается в WAL-файлы перед применением к таблицам.

Зачем WAL нужен:
- **Crash recovery** — после падения восстановление из WAL.
- **Репликация** — реплика получает WAL и применяет изменения.
- **PITR** — Point-in-Time Recovery через архив WAL.

WAL хранится в `$PGDATA/pg_wal/`. Файлы по 16MB (по умолчанию). Параметр `wal_level` определяет детализацию:

| wal_level | Что включает |
|---|---|
| `minimal` | Только crash recovery, нельзя реплицировать |
| `replica` (default) | Физическая репликация, streaming standby |
| `logical` | Logical replication (включает всё из replica) |

```sql
SHOW wal_level;
```

LSN (Log Sequence Number) — монотонно растущий указатель позиции в WAL:

```sql
-- текущая позиция WAL на primary
SELECT pg_current_wal_lsn();

-- на replica: отставание
SELECT pg_last_wal_receive_lsn(), pg_last_wal_replay_lsn();
```

---

## Streaming Replication (физическая)

Физическая репликация передаёт WAL-байты. Реплика является точной копией primary на уровне блоков.

Характеристики:
- Реплицирует весь инстанс целиком (нельзя реплицировать отдельную таблицу).
- Реплика read-only (hot standby).
- Минимальная задержка (обычно миллисекунды).
- Версия PostgreSQL на реплике ≥ версия на primary.

Настройка primary (`postgresql.conf`):

```
wal_level = replica
max_wal_senders = 5        # максимум одновременных реплик
wal_keep_size = 1GB        # хранить WAL для реплик без слотов
```

`pg_hba.conf`:
```
host  replication  replicator  replica_ip/32  md5
```

Настройка replica (`postgresql.conf`):
```
hot_standby = on           # read-only запросы пока replication идёт
primary_conninfo = 'host=primary_ip port=5432 user=replicator password=...'
```

Инициализация реплики через `pg_basebackup`:

```bash
pg_basebackup -h primary_host -U replicator -D /var/lib/postgresql/data \
  --wal-method=stream --checkpoint=fast --progress
```

---

## Logical Replication

Логическая репликация передаёт изменения на уровне строк (не байтов WAL). Позволяет:
- Реплицировать отдельные таблицы.
- Между разными мажорными версиями PostgreSQL.
- В разные схемы / с трансформацией данных.
- Bi-directional репликация (осторожно).

Модель: **Publication** (что публикуем) + **Subscription** (кто подписан).

На primary:
```sql
-- публиковать изменения таблицы orders
CREATE PUBLICATION orders_pub FOR TABLE orders;

-- или все таблицы
CREATE PUBLICATION all_pub FOR ALL TABLES;
```

На replica (subscriber):
```sql
CREATE SUBSCRIPTION orders_sub
    CONNECTION 'host=primary_host port=5432 dbname=mydb user=replicator password=...'
    PUBLICATION orders_pub;
```

Мониторинг:
```sql
-- на primary
SELECT * FROM pg_stat_replication;

-- на subscriber
SELECT * FROM pg_stat_subscription;
```

Ограничения logical replication:
- DDL не реплицируется — нужно применять вручную на обоих сторонах.
- Sequences не реплицируются.
- Нельзя реплицировать таблицы без PRIMARY KEY (по умолчанию).

---

## Synchronous vs Asynchronous

### Asynchronous (default)

Primary коммитит транзакцию сразу после записи в локальный WAL. Реплика применяет изменения с небольшой задержкой (lag).

Риск: при failover возможна потеря последних транзакций (не отреплицированных).

### Synchronous

Primary ждёт подтверждения от реплики перед возвратом commit клиенту. Нулевая потеря данных, но дополнительная latency.

```
synchronous_commit = on
synchronous_standby_names = 'FIRST 1 (replica1, replica2)'
```

Режимы `synchronous_commit`:
| Значение | Гарантия |
|---|---|
| `off` | Не ждём даже локального WAL flush |
| `local` | Локальный WAL flush, не ждём реплику |
| `remote_write` | Реплика записала в буфер (не fsync) |
| `on` (default sync) | Реплика сделала WAL flush |
| `remote_apply` | Реплика применила транзакцию |

Компромисс — `remote_write`: меньше latency чем `on`, но небольшой риск потери при сбое реплики.

На уровне транзакции:
```sql
-- эта транзакция без синхронной репликации
SET LOCAL synchronous_commit = off;
INSERT INTO logs ...;
```

---

## Read Replicas

Типичная архитектура: primary для записи, одна-две реплики для чтения.

Что нужно учитывать:
- **Replication lag** — данные на реплике могут быть несвежими (обычно < 100ms, но при нагрузке может быть секунды).
- **Нельзя читать сразу после записи с реплики** — после INSERT на primary, SELECT на реплике может не увидеть данные.
- **Транзакции на реплике** — BEGIN/COMMIT работают, но только для read-only.

Паттерн в Go с pgx — два пула:

```go
type DB struct {
    primary *pgxpool.Pool
    replica *pgxpool.Pool
}

func (db *DB) Write(ctx context.Context) pgx.Tx {
    tx, _ := db.primary.Begin(ctx)
    return tx
}

func (db *DB) ReadReplica(ctx context.Context) *pgxpool.Pool {
    return db.replica  // приложение само решает что читать с реплики
}
```

Проверить lag реплики:

```sql
-- на primary: отставание каждой реплики
SELECT client_addr, state, sent_lsn, write_lsn, flush_lsn, replay_lsn,
       pg_size_pretty(pg_wal_lsn_diff(sent_lsn, replay_lsn)) AS replay_lag
FROM pg_stat_replication;
```

---

## Replication Slots

Слот гарантирует, что WAL не будет удалён, пока реплика не применила его. Нужен когда реплика может временно отключиться.

```sql
-- создать физический слот
SELECT pg_create_physical_replication_slot('replica1_slot');

-- создать логический слот
SELECT pg_create_logical_replication_slot('my_slot', 'pgoutput');

-- посмотреть слоты и их lag
SELECT slot_name, active, xmin, catalog_xmin,
       pg_size_pretty(pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn)) AS wal_lag
FROM pg_replication_slots;
```

**Опасность**: неактивный слот удерживает WAL-файлы — диск может переполниться.

Мониторинг: алертить если lag > порог (например, 10GB):

```sql
SELECT slot_name, 
       pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn) AS lag_bytes
FROM pg_replication_slots
WHERE active = false;
```

Удалить брошенный слот:
```sql
SELECT pg_drop_replication_slot('old_slot');
```

---

## HA и Failover: Patroni

Patroni — стандартный инструмент для High Availability PostgreSQL. Использует Etcd/Consul/ZooKeeper для distributed consensus.

Что делает Patroni:
- Автоматический failover: при падении primary выбирает новый primary из реплик.
- Fencing: предотвращает "split-brain" — старый primary не может писать после failover.
- REST API для управления кластером.

Архитектура:
```
[Etcd cluster]
      |
[Patroni] -- [PostgreSQL primary] -- [Patroni] -- [PostgreSQL replica]
```

Базовый конфиг (`patroni.yml`):

```yaml
scope: postgres-cluster
name: node1

etcd3:
  hosts: etcd1:2379,etcd2:2379,etcd3:2379

bootstrap:
  dcs:
    ttl: 30
    loop_wait: 10
    retry_timeout: 10
    maximum_lag_on_failover: 1048576  # 1MB — реплика не становится primary если отстала

postgresql:
  listen: 0.0.0.0:5432
  data_dir: /var/lib/postgresql/data
  parameters:
    max_connections: 200
    wal_level: replica
    max_wal_senders: 10
```

Управление через `patronictl`:
```bash
patronictl -c patroni.yml list           # статус кластера
patronictl -c patroni.yml switchover     # ручной переключатель
patronictl -c patroni.yml failover       # принудительный failover
```

Перед primary обычно ставят HAProxy или pgbouncer, который по health check направляет трафик на текущего primary.

---

## Типичные проблемы

**Replication lag растёт:**
- Тяжёлые запросы на primary конкурируют с WAL sender.
- Тяжёлые запросы на реплике (hot standby query conflicts): `max_standby_streaming_delay`.
- Сеть между primary и replica.

**Replica conflict (query cancellation):**
```sql
-- на реплике, запрос отменяется из-за конфликта с vacuum на primary
ERROR: canceling statement due to conflict with recovery

-- увеличить timeout
hot_standby_feedback = on  -- replica сообщает primary о своём xmin
max_standby_streaming_delay = 30s
```

**WAL накапливается на primary:**
- Неактивный replication slot.
- Синхронная реплика недоступна (primary ждёт).
- `wal_keep_size` слишком большой.

---

## Interview-ready answer

PostgreSQL репликация строится на WAL. Физическая (streaming replication) — точная копия инстанса, реплика read-only, минимальный lag. Логическая — репликация на уровне строк, можно реплицировать отдельные таблицы, работает между мажорными версиями, DDL не реплицируется. Synchronous commit — primary ждёт подтверждения от реплики, нулевая потеря данных но дополнительная latency. Read replicas: нельзя читать с реплики сразу после записи из-за lag. Replication slots гарантируют сохранение WAL но опасны если реплика отключилась — нужен мониторинг lag. Для HA — Patroni с Etcd: автоматический failover, fencing split-brain.
