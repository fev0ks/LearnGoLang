# ClickHouse SQL: практические паттерны

## Содержание

- [Чем SQL ClickHouse отличается на практике](#чем-sql-clickhouse-отличается-на-практике)
- [Комбинаторы агрегатных функций](#комбинаторы-агрегатных-функций)
- [uniq и quantile](#uniq-и-quantile)
- [Массивы и ARRAY JOIN](#массивы-и-array-join)
- [LIMIT BY и top N в группе](#limit-by-и-top-n-в-группе)
- [WITH FILL: заполнение пропусков времени](#with-fill-заполнение-пропусков-времени)
- [Воронки и retention](#воронки-и-retention)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)
- [Официальная документация](#официальная-документация)

Диалект ClickHouse похож на обычный SQL, но ориентирован на работу с колонками, массивами и промежуточными состояниями агрегатов. Senior-level навык здесь — не помнить сотни функций, а понимать, какая конструкция сохраняет точность, сколько строк она порождает и можно ли объединять её результат между blocks и shards.

---

## Чем SQL ClickHouse отличается на практике

Четыре идеи встречаются почти в каждом аналитическом запросе:

- условная агрегация выражается комбинатором `-If`, а не набором подзапросов;
- агрегат можно превратить в сериализуемое состояние через `-State` и объединить через `-Merge`;
- одна строка часто содержит массив значений, который обрабатывают lambda-функциями без нормализации;
- time-series запросы умеют строить buckets и заполнять отсутствующие интервалы.

Оптимизация начинается с access pattern: `WHERE` и `PREWHERE` должны отсечь данные до тяжёлых функций. Красивый SQL, который сначала разворачивает миллиард массивов, а потом фильтрует, останется дорогим.

---

## Комбинаторы агрегатных функций

Комбинатор — suffix, который меняет поведение обычной агрегатной функции.

| Комбинатор | Пример | Что делает |
| --- | --- | --- |
| `-If` | `countIf(status = 'paid')` | учитывает строки по условию |
| `-Array` | `uniqArray(tags)` | агрегирует элементы массивов |
| `-Map` | `sumMap(metrics)` | агрегирует значения map по ключам |
| `-State` | `avgState(value)` | возвращает промежуточное состояние |
| `-Merge` | `avgMerge(value_state)` | объединяет состояния и возвращает результат |
| `-MergeState` | `avgMergeState(value_state)` | объединяет состояния и оставляет состояние |
| `-SimpleState` | `sumSimpleState(value)` | возвращает значение для `SimpleAggregateFunction` |

Один проход вместо нескольких подзапросов:

```sql
SELECT
    toDate(event_time)                    AS day,
    count()                               AS events,
    countIf(event_type = 'purchase')      AS purchases,
    sumIf(value, event_type = 'purchase') AS revenue,
    uniqIf(user_id, event_type = 'view')  AS viewers
FROM events
WHERE event_date >= today() - 6
GROUP BY day
ORDER BY day;
```

`-State` — не финальное число и не стабильный внешний формат для приложения. Это внутреннее бинарное состояние конкретной функции и типов аргументов. Его хранят в `AggregateFunction(...)`, переносят между ClickHouse tables и читают соответствующим `-Merge`.

Названия должны совпадать: `uniqCombined64State(UInt64)` объединяется через `uniqCombined64Merge`, а не произвольный `uniqMerge`.

---

## uniq и quantile

`count(DISTINCT user_id)` не отвечает, какую цену приложение готово платить за точность. В ClickHouse выбор делается явно.

| Функция | Точность | Память | Когда выбирать |
| --- | --- | --- | --- |
| `uniqExact` | точная | растёт с числом уникальных значений | небольшой набор, billing, reconciliation |
| `uniq` | приближённая | ограниченная | быстрый общий estimate |
| `uniqCombined64` | приближённая, стабильнее на больших UInt64 | ограниченная | продуктовая аналитика большого масштаба |
| `uniqHLL12` | приближённая HyperLogLog | компактная | когда допустима известная погрешность |

Нельзя обещать точность approximate-функции без теста на распределении своих ключей. Для финансового инварианта используют точный расчёт или сверку в OLTP-системе; для дашборда несколько десятых процента часто приемлемы.

Несколько квантилей лучше считать одной функцией — она делит подготовительную работу между уровнями:

```sql
SELECT
    quantilesTDigest(0.50, 0.90, 0.99)(latency_ms) AS q
FROM request_events
WHERE event_date = today();
```

Результат `q` — массив `[p50, p90, p99]`. Три независимых вызова `quantileTDigest` могут выполнить больше работы. `quantileExact` нужен только когда точность оправдывает память и CPU; для latency dashboards обычно выбирают TDigest.

---

## Массивы и ARRAY JOIN

Lambda-функции позволяют преобразовать массив внутри строки, не увеличивая число строк:

```sql
SELECT
    event_id,
    arrayFilter(tag -> startsWith(tag, 'team:'), tags) AS team_tags,
    arrayMap(x -> lowerUTF8(x), tags)                  AS normalized_tags
FROM events
WHERE event_date = today();
```

`arrayJoin` делает обратное: размножает строку по числу элементов.

```sql
SELECT tag, count() AS events
FROM events
ARRAY JOIN tags AS tag
WHERE event_date = today()
GROUP BY tag
ORDER BY events DESC;
```

Если после фильтра осталось 100 млн строк и в среднем по 12 tags, `ARRAY JOIN` породит около `100 000 000 × 12 = 1,2 млрд` промежуточных строк. Это оценка capacity, а не деталь синтаксиса. Фильтры по обычным колонкам следует применять до разворачивания, а для простой проверки наличия элемента использовать `has`, `hasAny` или подходящий index вместо `ARRAY JOIN`.

Пустой массив порождает ноль строк. Если строку нужно сохранить, применяют `emptyArrayToSingle` либо `LEFT ARRAY JOIN`, осознанно выбирая default semantics.

---

## LIMIT BY и top N в группе

`LIMIT n BY key` оставляет первые `n` строк для каждого значения key после применённого порядка.

```sql
SELECT user_id, event_time, event_type
FROM events
WHERE event_date = today()
ORDER BY user_id, event_time DESC
LIMIT 3 BY user_id;
```

Это «последние три события каждого пользователя», а не глобальные три строки. Итоговый `LIMIT` можно добавить отдельно:

```sql
...
LIMIT 3 BY user_id
LIMIT 1000;
```

Без детерминированного `ORDER BY` набор строк нестабилен. Если `event_time` может совпасть, добавляют уникальный tie-breaker: `ORDER BY user_id, event_time DESC, event_id DESC`.

Для current-state модели `LIMIT 1 BY` обязан сначала выбрать последнюю версию, а потом применить tombstone filter — готовый пример находится в [04-deduplication.md](./04-deduplication.md).

---

## WITH FILL: заполнение пропусков времени

Обычный `GROUP BY` не создаёт buckets без событий. `WITH FILL` добавляет их после сортировки:

```sql
SELECT
    toStartOfInterval(event_time, INTERVAL 5 MINUTE) AS bucket,
    count()                                           AS events
FROM events
WHERE event_time >= toDateTime('2026-08-21 10:00:00')
  AND event_time <  toDateTime('2026-08-21 11:00:00')
GROUP BY bucket
ORDER BY bucket
WITH FILL
    FROM toDateTime('2026-08-21 10:00:00')
    TO   toDateTime('2026-08-21 11:00:00')
    STEP INTERVAL 5 MINUTE;
```

Интервал `[10:00, 11:00)` содержит 60 минут, значит ожидается `60 / 5 = 12` buckets: от `10:00` до `10:55`. В `WITH FILL` граница `FROM` включается, а `TO` не включается. Если нужен bucket ровно на конечной метке, `TO` сдвигают на один `STEP`; для `DateTime64` важно сохранить нужную точность.

`WITH FILL` не заменяет calendar table для сложных business calendars, часовых поясов и праздников. При переходах DST час может повториться или отсутствовать; хранить события лучше в UTC, а локальное представление формировать на краю отчёта.

---

## Воронки и retention

`windowFunnel` проверяет порядок условий в заданном временном окне:

```sql
SELECT
    user_id,
    windowFunnel(3600)(
        event_time,
        event_type = 'view',
        event_type = 'cart',
        event_type = 'purchase'
    ) AS step
FROM events
WHERE event_date BETWEEN '2026-08-01' AND '2026-08-07'
GROUP BY user_id;
```

Результат `3` означает, что пользователь последовательно прошёл все три шага не более чем за 3600 секунд. Три `countIf` не эквивалентны: пользователь мог купить раньше просмотра. Для одинаковых timestamps заранее определяют semantics и используют `DateTime64` либо отдельную sequence column.

Комбинатор `retention` возвращает массив признаков: первое условие — принадлежность cohort, следующие — наступление событий относительно него.

```sql
SELECT
    cohort,
    sumForEach(retention_flags) AS retained
FROM
(
    SELECT
        user_id,
        signup_time,
        toStartOfWeek(signup_time) AS cohort,
        retention(
            event_date = toDate(signup_time),
            event_date = toDate(signup_time) + 1,
            event_date = toDate(signup_time) + 7
        ) AS retention_flags
    FROM user_activity
    GROUP BY user_id, signup_time, cohort
)
GROUP BY cohort
ORDER BY cohort;
```

На практике signup и activity часто лежат в разных таблицах. Тогда сначала формируют корректный набор `user_id + signup_time + activity`, контролируя стоимость `JOIN`, либо заранее денормализуют cohort.

---

## Типичные ошибки

- использовать `uniqExact` на миллиардах высококардинальных ключей без memory estimate;
- складывать готовые averages или quantiles вместо хранения aggregate states;
- вызывать `arrayJoin` до селективного фильтра и не пересчитывать рост промежуточных строк;
- использовать `LIMIT BY` без полного `ORDER BY` и получать нестабильного победителя;
- считать funnel через независимые `countIf`, игнорируя порядок;
- смешивать локальное время и UTC на границах buckets;
- применять approximate aggregate там, где результат является billing-инвариантом.

---

## Interview-ready answer

**1. Что дают комбинаторы агрегатных функций?**

- `-If` добавляет условие без отдельного подзапроса.
- `-State` сохраняет промежуточное состояние, `-Merge` объединяет states и возвращает итог.
- Это основа `AggregatingMergeTree` и incremental materialized views.

**2. Как выбрать функцию подсчёта уникальных значений?**

- `uniqExact` — точный, но память растёт с кардинальностью.
- `uniqCombined64` или другая approximate-функция — ограниченная память и контролируемая погрешность для аналитики.
- Выбор определяется требованием точности и проверяется на реальном распределении ключей.

**3. Чем ARRAY JOIN опасен?**

- Он превращает каждый элемент массива в строку и умножает промежуточный набор на среднюю длину массива.
- Фильтровать обычные колонки нужно до разворачивания; для membership часто достаточно `has`/`hasAny`.

**4. Чем LIMIT BY отличается от LIMIT?**

- `LIMIT n BY key` оставляет `n` строк в каждой группе key, обычный `LIMIT` ограничивает весь результат.
- Победитель определён только при стабильном `ORDER BY` с tie-breaker.

**5. Как посчитать funnel?**

- `windowFunnel` проверяет порядок условий и временное окно.
- Независимые `countIf` считают наличие этапов, но не доказывают последовательность.

---

## Официальная документация

- [Aggregate function combinators](https://clickhouse.com/docs/sql-reference/aggregate-functions/combinators)
- [uniq functions](https://clickhouse.com/docs/sql-reference/aggregate-functions/reference/uniq)
- [arrayJoin](https://clickhouse.com/docs/sql-reference/functions/array-join)
- [LIMIT BY](https://clickhouse.com/docs/sql-reference/statements/select/limit-by)
- [WITH FILL](https://clickhouse.com/docs/sql-reference/statements/select/order-by#order-by-expr-with-fill-modifier)
- [windowFunnel](https://clickhouse.com/docs/sql-reference/aggregate-functions/parametric-functions#windowfunnel)
