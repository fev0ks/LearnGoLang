# DynamoDB

DynamoDB это fully managed NoSQL database в AWS для key-value and document access patterns.

## Содержание

- [Где используется](#где-используется)
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
- shopping carts;
- user preferences;
- session-like state;
- metadata stores;
- event or item lookup by key.

## Сильные стороны

- fully managed;
- serverless operational model;
- predictable low-latency key-value access;
- scales without managing servers;
- tight AWS integration.

## Слабые стороны

- data modeling требует заранее знать access patterns;
- нет joins;
- ad-hoc queries ограничены;
- vendor lock-in;
- неправильно выбранный partition key может создать hot partitions.

## Когда выбирать

Выбирай DynamoDB, если:
- проект AWS-native;
- access patterns хорошо известны;
- нужен high-scale key-value/document storage;
- не хочется управлять БД самостоятельно;
- schema relational constraints не нужны.

## Когда не выбирать

Лучше подумать о PostgreSQL/MongoDB/etc, если:
- нужны сложные ad-hoc queries;
- domain relational;
- нужен переносимый open-source deployment;
- команда не готова к access-pattern-first modeling.

## Типичные ошибки

- проектировать таблицы как SQL schema;
- не думать о partition key;
- ожидать joins;
- не считать cost model;
- менять access patterns после запуска без понимания последствий.

## Interview-ready answer

DynamoDB хорош для AWS-native key-value/document workloads с заранее известными access patterns. Его сила в managed scale, а цена - в моделировании данных под ключи, ограниченных ad-hoc queries и vendor lock-in.

## Query examples

DynamoDB обычно используют через SDK или AWS CLI. Модель строится вокруг partition key и sort key.

Пример item:

```json
{
  "PK": "USER#42",
  "SK": "PROFILE",
  "email": "user@example.com",
  "status": "active"
}
```

Получить item по ключу:

```bash
aws dynamodb get-item \
  --table-name AppTable \
  --key '{
    "PK": {"S": "USER#42"},
    "SK": {"S": "PROFILE"}
  }'
```

Query по partition key:

```bash
aws dynamodb query \
  --table-name AppTable \
  --key-condition-expression "PK = :pk" \
  --expression-attribute-values '{
    ":pk": {"S": "USER#42"}
  }'
```

Query по prefix sort key:

```bash
aws dynamodb query \
  --table-name AppTable \
  --key-condition-expression "PK = :pk AND begins_with(SK, :prefix)" \
  --expression-attribute-values '{
    ":pk": {"S": "USER#42"},
    ":prefix": {"S": "ORDER#"}
  }'
```
