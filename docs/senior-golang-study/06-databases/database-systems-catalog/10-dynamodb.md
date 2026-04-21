# DynamoDB

DynamoDB это fully managed NoSQL database в AWS для key-value и document access patterns.

## Содержание

- [Где используется](#где-используется)
- [Модель данных: partition key и sort key](#модель-данных-partition-key-и-sort-key)
- [Single-table design](#single-table-design)
- [Global Secondary Index (GSI)](#global-secondary-index-gsi)
- [Capacity modes и cost model](#capacity-modes-и-cost-model)
- [Сильные стороны](#сильные-стороны)
- [Слабые стороны](#слабые-стороны)
- [Когда выбирать](#когда-выбирать)
- [Когда не выбирать](#когда-не-выбирать)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)
- [Query examples](#query-examples)

## Где используется

- AWS-native backend;
- high-scale key-value workloads;
- shopping carts, user preferences, session-like state;
- metadata stores;
- serverless applications.

## Модель данных: partition key и sort key

Каждый item идентифицируется комбинацией `partition key` (PK) + `sort key` (SK, опционально).

`Partition key` определяет физический раздел, на котором хранится item. Должен равномерно распределять нагрузку — иначе hot partition.

`Sort key` позволяет хранить множество связанных items под одним PK, делать range queries и prefix queries.

```text
PK              SK              Данные
USER#42         PROFILE         { email, name, ... }
USER#42         ORDER#2026-01   { amount, status, ... }
USER#42         ORDER#2026-02   { amount, status, ... }
```

Так читается вся история заказов пользователя одним Query по PK=USER#42, SK begins_with ORDER#.

## Single-table design

В DynamoDB часто используют одну таблицу для всех entity types. PK и SK — generic (`PK`, `SK`), конкретный тип кодируется в значении.

Плюсы: все данные для одной бизнес-операции можно получить одним Query или TransactGetItems.

Минусы: сложнее читать схему; нет FK; сложнее ad-hoc queries.

Подходит для: хорошо известных access patterns с высоким объемом. Плохо подходит для: сложных analytics, меняющихся access patterns.

## Global Secondary Index (GSI)

GSI позволяет делать Query по атрибутам, которые не являются PK/SK основной таблицы.

```text
Основная таблица: PK=USER#42, SK=ORDER#001
GSI: PK=STATUS#pending, SK=CREATED_AT
```

Query по GSI: "все pending заказы за последние 24 часа".

GSI — это асинхронная реплика с другим ключом. Eventual consistency: данные в GSI появляются не мгновенно после записи в основную таблицу.

Local Secondary Index (LSI) — другой SK для того же PK. Синхронный, но создается только при создании таблицы и не масштабируется отдельно.

## Capacity modes и cost model

**On-demand**: платишь за каждый read/write unit. Автоматически масштабируется. Хорошо для непредсказуемой нагрузки, новых сервисов.

**Provisioned**: задаешь RCU (read capacity units) и WCU (write capacity units). Дешевле при предсказуемой нагрузке. Можно настроить auto-scaling.

1 RCU = 1 strongly consistent read или 2 eventually consistent reads для items до 4 KB.  
1 WCU = 1 write для items до 1 KB.

Hot partition throttling: если один partition key получает слишком много трафика, DynamoDB throttles запросы даже при достаточных общих capacity units.

## Сильные стороны

- fully managed, zero ops;
- serverless operational model;
- predictable low-latency key-value access;
- scales without managing servers;
- transactions (TransactWriteItems) для multi-item atomicity;
- tight AWS integration (Lambda triggers, Streams).

## Слабые стороны

- data modeling требует заранее знать access patterns;
- нет joins;
- ad-hoc queries ограничены — только по PK или GSI;
- vendor lock-in;
- неправильный partition key → hot partition → throttling;
- cost model нелинеен при больших items.

## Когда выбирать

Выбирай DynamoDB, если:
- проект AWS-native и нужен zero-ops storage;
- access patterns хорошо известны и стабильны;
- нужен high-scale key-value/document storage;
- serverless architecture (Lambda + DynamoDB).

## Когда не выбирать

Лучше подумать о PostgreSQL/MongoDB, если:
- access patterns меняются или неизвестны заранее;
- нужны сложные ad-hoc queries;
- domain strongly relational;
- нужен переносимый open-source deployment.

## Типичные ошибки

- проектировать таблицы как SQL schema (нормализация без учета access patterns);
- не думать о partition key → hot partition → throttling;
- ожидать joins;
- не учитывать eventual consistency GSI при чтении сразу после записи;
- не считать cost model при большом объеме больших items.

## Interview-ready answer

DynamoDB — managed key-value/document storage для AWS-native workloads. Главное архитектурное решение — partition key и sort key: от них зависят все access patterns. Single-table design позволяет читать связанные items одним Query, но требует дисциплины. GSI добавляет дополнительные access patterns, но eventual consistency. На горизонт: hot partition throttling — если один PK получает непропорциональный трафик, DynamoDB ограничивает его даже при достаточной общей capacity. Vendor lock-in — реальная цена за zero-ops.

## Query examples

Item model:

```json
{
  "PK": "USER#42",
  "SK": "ORDER#2026-04-20",
  "amount": 150.00,
  "status": "pending",
  "created_at": "2026-04-20T10:00:00Z"
}
```

Получить item по ключу:

```bash
aws dynamodb get-item \
  --table-name AppTable \
  --key '{"PK": {"S": "USER#42"}, "SK": {"S": "ORDER#2026-04-20"}}'
```

Query все заказы пользователя (SK begins_with ORDER#):

```bash
aws dynamodb query \
  --table-name AppTable \
  --key-condition-expression "PK = :pk AND begins_with(SK, :prefix)" \
  --expression-attribute-values '{
    ":pk": {"S": "USER#42"},
    ":prefix": {"S": "ORDER#"}
  }'
```

Conditional write (optimistic concurrency):

```bash
aws dynamodb put-item \
  --table-name AppTable \
  --item '{"PK": {"S": "USER#42"}, "SK": {"S": "PROFILE"}, "version": {"N": "2"}}' \
  --condition-expression "version = :expected" \
  --expression-attribute-values '{":expected": {"N": "1"}}'
```
