# Шардирование PostgreSQL

Шардирование — горизонтальное масштабирование данных на несколько независимых узлов. Это не первый инструмент, а последний рубеж после того, как партиционирование, read replicas и connection pooling перестали справляться.

## Содержание

- [Партиционирование vs шардирование](#партиционирование-vs-шардирование)
- [Когда шардирование нужно](#когда-шардирование-нужно)
- [Application-level sharding](#application-level-sharding)
- [Consistent hashing](#consistent-hashing)
- [Citus: шардирование внутри PostgreSQL](#citus-шардирование-внутри-postgresql)
- [Foreign Data Wrappers](#foreign-data-wrappers)
- [Проблемы cross-shard операций](#проблемы-cross-shard-операций)
- [Глобальные ID без центральной последовательности](#глобальные-id-без-центральной-последовательности)
- [Resharding](#resharding)
- [Антипаттерны](#антипаттерны)

---

## Партиционирование vs шардирование

| | Партиционирование | Шардирование |
|---|---|---|
| Узлы | один PostgreSQL сервер | несколько независимых серверов |
| Управление | нативный PostgreSQL | приложение или Citus |
| ACID транзакции | да, полные | нет (только внутри одного шарда) |
| JOIN | да, любые | только внутри одного шарда |
| Сложность | низкая | высокая |
| Когда | таблица > 100GB, slow queries | один сервер не справляется |

Партиционирование решает проблему **размера индексов и скорости vacuum** — всё ещё один сервер. Шардирование решает проблему **write throughput и total storage** — данные физически на разных машинах.

Большинство систем не доходят до шардирования. Сначала исчерпай:
1. Индексы и оптимизация запросов
2. Партиционирование
3. Read replicas для read-heavy нагрузки
4. Connection pooling (PgBouncer)
5. Вертикальное масштабирование

---

## Когда шардирование нужно

- Write throughput превышает возможности одного мастера (> ~10-20k writes/sec)
- Объём данных > возможностей одного сервера (несколько TB с требованиями к IO)
- Нужна географическая изоляция данных (EU data → EU сервер, US data → US сервер)
- Latency требования диктуют близость к пользователю

---

## Application-level sharding

Самый распространённый вид в production — логика шардирования в приложении.

### Hash sharding

```go
const numShards = 4

func shardIndex(userID string) int {
    h := fnv.New32a()
    h.Write([]byte(userID))
    return int(h.Sum32()) % numShards
}

type ShardedDB struct {
    shards []*pgxpool.Pool
}

func (db *ShardedDB) GetUser(ctx context.Context, userID string) (*User, error) {
    shard := db.shards[shardIndex(userID)]
    // запрос только к нужному шарду
    return queryUser(ctx, shard, userID)
}
```

Проблема hash sharding: при добавлении шарда меняется маппинг почти всех ключей — нужен resharding.

### Range sharding

```go
type ShardRange struct {
    Min    string
    Max    string
    Pool   *pgxpool.Pool
}

// userID[0] → 'a'-'h' → shard0, 'i'-'p' → shard1, ...
var shardRanges = []ShardRange{
    {Min: "0", Max: "3fffffff", Pool: shard0Pool},
    {Min: "40000000", Max: "7fffffff", Pool: shard1Pool},
    {Min: "80000000", Max: "bfffffff", Pool: shard2Pool},
    {Min: "c0000000", Max: "ffffffff", Pool: shard3Pool},
}

func routeByRange(userID string) *pgxpool.Pool {
    for _, r := range shardRanges {
        if userID >= r.Min && userID <= r.Max {
            return r.Pool
        }
    }
    panic("no shard for id: " + userID)
}
```

Range sharding хорош для timestamp-based данных — можно архивировать старые шарды. Плохо для равномерного распределения — "горячие" диапазоны перегружают один шард.

---

## Consistent hashing

Consistent hashing минимизирует количество ключей которые нужно перемещать при добавлении/удалении шарда.

```
Обычный hashing: добавить шард → переместить ~(N-1)/N ключей
Consistent hashing: добавить шард → переместить ~1/N ключей
```

Принцип: ключи и узлы размещаются на кольце (0..2^32). Ключ маппится на ближайший узел по часовой стрелке.

```go
import "github.com/buraksezer/consistent"

cfg := consistent.Config{
    PartitionCount:    271,    // число виртуальных партиций
    ReplicationFactor: 20,     // виртуальных узлов на реальный
    Load:              1.25,
    Hasher:            hasher{},
}
c := consistent.New(nil, cfg)

// Добавить шарды
c.Add(consistent.NewMember("shard-0"))
c.Add(consistent.NewMember("shard-1"))
c.Add(consistent.NewMember("shard-2"))

// Маршрутизация
member, _ := c.GetClosestN([]byte(userID), 1)
shardName := member[0].String()  // "shard-1"
pool := shardPools[shardName]
```

Виртуальные узлы (virtual nodes) — каждый физический шард представлен несколькими точками на кольце. Это выравнивает распределение.

---

## Citus: шардирование внутри PostgreSQL

Citus — PostgreSQL extension (теперь часть Microsoft/Azure) для горизонтального шардирования. Выглядит как обычный PostgreSQL, шардирование прозрачно для большинства запросов.

```sql
-- Установить Citus
CREATE EXTENSION citus;

-- Добавить worker ноды
SELECT citus_add_node('worker1', 5432);
SELECT citus_add_node('worker2', 5432);
SELECT citus_add_node('worker3', 5432);

-- Создать обычную таблицу
CREATE TABLE users (
    id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name    TEXT NOT NULL,
    email   TEXT UNIQUE,
    plan    TEXT
);

-- Распределить по шардам по ключу user_id
-- Citus создаёт 32 шарда (по умолчанию) и распределяет их по workers
SELECT create_distributed_table('users', 'id');

-- Создать таблицу orders и колоцировать с users
-- Строки orders с тем же user_id → тот же шард что и user
CREATE TABLE orders (
    id      UUID DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    amount  NUMERIC(10,2)
);
SELECT create_distributed_table('orders', 'user_id',
    colocate_with => 'users');
```

С колокацией `JOIN users JOIN orders ON users.id = orders.user_id` выполняется локально на каждом шарде без сетевого hop'а.

```sql
-- Reference table — маленькая таблица реплицируется на все workers
CREATE TABLE countries (code CHAR(2) PRIMARY KEY, name TEXT);
SELECT create_reference_table('countries');
-- Теперь JOIN distributed_table с countries работает везде
```

Citus поддерживает большинство PostgreSQL запросов, но с ограничениями:
- `UNIQUE` constraints только на sharding column
- Foreign keys только между колоцированными таблицами
- Distributed transactions через 2PC (работает, но медленнее)

---

## Foreign Data Wrappers

FDW позволяет делать запросы к внешним PostgreSQL серверам как к локальным таблицам.

```sql
-- На координаторе
CREATE EXTENSION postgres_fdw;

CREATE SERVER shard1 FOREIGN DATA WRAPPER postgres_fdw
    OPTIONS (host 'db-shard1.internal', port '5432', dbname 'app');

CREATE SERVER shard2 FOREIGN DATA WRAPPER postgres_fdw
    OPTIONS (host 'db-shard2.internal', port '5432', dbname 'app');

CREATE USER MAPPING FOR app_user SERVER shard1
    OPTIONS (user 'app_user', password 'secret');

-- Создать foreign table — выглядит как локальная
CREATE FOREIGN TABLE users_shard1 (
    id    UUID,
    name  TEXT,
    email TEXT
) SERVER shard1 OPTIONS (table_name 'users');

-- Запрос прозрачно идёт к shard1
SELECT * FROM users_shard1 WHERE id = 'abc';
```

Использование FDW для ручного шардирования:
```sql
-- Партиция-обёртка для маршрутизации
CREATE TABLE users (id UUID, name TEXT) PARTITION BY HASH(id);

CREATE FOREIGN TABLE users_shard0 PARTITION OF users
    FOR VALUES WITH (MODULUS 4, REMAINDER 0)
    SERVER shard0;

CREATE FOREIGN TABLE users_shard1 PARTITION OF users
    FOR VALUES WITH (MODULUS 4, REMAINDER 1)
    SERVER shard1;

-- Теперь INSERT/SELECT на users автоматически маршрутизируется
```

FDW push-down: PostgreSQL умеет передавать WHERE conditions на удалённый сервер — данные фильтруются там, а не тянутся все.

---

## Проблемы cross-shard операций

### JOIN через шарды

```sql
-- Работает в Citus если таблицы колоцированы
SELECT u.name, o.amount FROM users u JOIN orders o ON u.id = o.user_id
WHERE u.id = 'abc';

-- Не работает эффективно если не колоцированы:
-- PostgreSQL тянет данные со всех шардов, JOIN делает на координаторе
SELECT u.name, p.title FROM users u JOIN products p ON u.last_product = p.id;
-- products не колоцирован с users → broadcast join или медленный gather
```

**Решение**: проектировать схему так, чтобы связанные данные лежали на одном шарде (colocate by tenant_id, user_id).

### Распределённые транзакции

Нет ACID across shards без 2PC (Two-Phase Commit):
```
Списать с user A на shard0 + начислить user B на shard1
→ нет атомарности без distributed transaction
```

2PC в Citus работает, но:
- Медленнее (два round-trip)
- Уязвим к partial failure (coordinator упал между Prepare и Commit)

Альтернативы:
- Sage pattern + компенсирующие транзакции
- Outbox pattern для eventual consistency
- Проектировать так, чтобы транзакции не пересекали шарды

### Aggregations

```sql
-- COUNT(*) в Citus — параллельно на всех шардах, потом суммируется
SELECT COUNT(*) FROM users;  -- работает

-- Точный COUNT DISTINCT через шарды дорог
SELECT COUNT(DISTINCT country) FROM users;  -- собирает все данные на координатор

-- HyperLogLog — приближённый COUNT DISTINCT без сбора всех данных
SELECT hll_cardinality(hll_union_agg(hll_add(hll_empty(), country::bytea)))
FROM users;
```

---

## Глобальные ID без центральной последовательности

`SERIAL` / `BIGSERIAL` на каждом шарде дадут коллизии. Нужны глобально уникальные ID:

**UUID v7** — рекомендуемый выбор: временная компонента + random, монотонно возрастают в пределах миллисекунды, хорошо для B-tree индексов:
```sql
-- PostgreSQL 17+
SELECT gen_random_uuid();  -- UUID v4

-- uuid-ossp
CREATE EXTENSION "uuid-ossp";
SELECT uuid_generate_v7();  -- UUID v7 (если доступно)
```

**Snowflake ID** — 64-bit int: timestamp (41 bit) + datacenter (5 bit) + worker (5 bit) + sequence (12 bit):
```go
import "github.com/bwmarrin/snowflake"

node, _ := snowflake.NewNode(1)  // worker ID = 1
id := node.Generate()            // уникален в пределах datacenter
fmt.Println(id.Int64())          // 7847291847362...
```

---

## Resharding

Добавить шард без downtime — сложнейшая операция:

**С Citus** (`rebalance_table_shards`):
```sql
-- Добавить новый worker
SELECT citus_add_node('worker4', 5432);

-- Перебалансировать шарды (онлайн, но нагружает сеть)
SELECT rebalance_table_shards('users');

-- Или только освободить конкретный worker
SELECT rebalance_table_shards('users', excluded_shard_list => ARRAY[102008, 102009]);
```

**Application-level resharding** — без Citus:
1. Двойная запись: пишем и в старый, и в новый шард
2. Backfill: копируем исторические данные в новый шард
3. Переключение чтения на новый шард
4. Прекращаем двойную запись
5. Удаляем данные из старого шарда

Этот процесс занимает дни-недели для больших объёмов.

---

## Антипаттерны

**Шардировать преждевременно** — главная ошибка. Шардирование добавляет огромную операционную сложность. Партиционирование + read replicas закрывают 95% случаев.

**Шардировать по неравномерному ключу** — шардирование по `country` даст огромный сhard для US и маленький для LU. Используй ключи с равномерным распределением.

**Кросс-шардовые транзакции везде** — если большинство бизнес-операций затрагивают несколько шардов, схема шардирования неправильная. Модель данных должна минимизировать cross-shard операции.

**Забыть про ID коллизии** — после шардирования `SERIAL` на разных шардах выдаёт одинаковые числа. Переходи на UUID или Snowflake до шардирования.

**Шардировать без мониторинга hotspot** — один шард может получать 80% трафика. Нужно мониторить распределение нагрузки по шардам.
