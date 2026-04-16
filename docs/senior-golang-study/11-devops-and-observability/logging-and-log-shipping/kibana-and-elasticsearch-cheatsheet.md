# Kibana And Elasticsearch Cheatsheet

Короткий practical cheatsheet для ежедневного поиска по логам в Kibana и через Elasticsearch Query DSL.

## Быстрый Workflow

1. Сузить `time range`.
2. Отфильтровать `env` и `service.name`.
3. Посмотреть `error`, `500`, latency spike или конкретный `trace.id`.
4. Найти pattern: endpoint, tenant, version, downstream.

## KQL: Базовые Фильтры

### Все ошибки сервиса

```text
service.name : "payments-api" and env : "prod" and log.level : "error"
```

### Все `500`

```text
service.name : "payments-api" and http.response.status_code >= 500
```

### Ошибки по конкретному endpoint

```text
service.name : "payments-api" and url.path : "/v1/payments" and http.response.status_code >= 500
```

### Поиск по `trace.id`

```text
trace.id : "7c4b2d123"
```

### Поиск по `user.id`

```text
service.name : "payments-api" and user.id : "42"
```

### Поиск по версии сервиса

```text
service.name : "payments-api" and service.version : "1.8.4"
```

### Поиск Redis timeout

```text
service.name : "payments-api" and message : "*redis*" and message : "*timeout*"
```

Если логи структурированы лучше:

```text
service.name : "payments-api" and error.type : "timeout" and message : "*redis*"
```

### Медленные запросы

Если `event.duration` в наносекундах:

```text
service.name : "payments-api" and event.duration > 1000000000
```

### Поле отсутствует

```text
service.name : "payments-api" and not trace.id : *
```

## KQL: Useful Patterns

### Сравнить две версии

```text
service.name : "payments-api" and service.version : "1.8.3"
```

```text
service.name : "payments-api" and service.version : "1.8.4"
```

### Найти конкретный tenant

```text
tenant.id : "acme"
```

### Найти только warning и error

```text
service.name : "payments-api" and (log.level : "warn" or log.level : "error")
```

## DSL: Базовый Search

```json
GET logs-*/_search
{
  "size": 50,
  "query": {
    "bool": {
      "filter": [
        { "term": { "service.name.keyword": "payments-api" } },
        { "term": { "env.keyword": "prod" } },
        { "range": { "@timestamp": { "gte": "now-15m" } } }
      ]
    }
  },
  "sort": [
    { "@timestamp": "desc" }
  ]
}
```

## DSL: Поиск По `trace.id`

```json
GET logs-*/_search
{
  "query": {
    "term": {
      "trace.id.keyword": "7c4b2d123"
    }
  },
  "sort": [
    { "@timestamp": "asc" }
  ]
}
```

## DSL: Ошибки По Endpoint

```json
GET logs-*/_search
{
  "query": {
    "bool": {
      "filter": [
        { "term": { "service.name.keyword": "payments-api" } },
        { "term": { "url.path.keyword": "/v1/payments" } },
        { "range": { "http.response.status_code": { "gte": 500 } } },
        { "range": { "@timestamp": { "gte": "now-1h" } } }
      ]
    }
  }
}
```

## DSL: Медленные Запросы

```json
GET logs-*/_search
{
  "query": {
    "bool": {
      "filter": [
        { "term": { "service.name.keyword": "payments-api" } },
        { "range": { "event.duration": { "gt": 1000000000 } } },
        { "range": { "@timestamp": { "gte": "now-30m" } } }
      ]
    }
  },
  "sort": [
    { "event.duration": "desc" }
  ]
}
```

## DSL: Top Endpoints By Errors

```json
GET logs-*/_search
{
  "size": 0,
  "query": {
    "bool": {
      "filter": [
        { "term": { "service.name.keyword": "payments-api" } },
        { "range": { "http.response.status_code": { "gte": 500 } } },
        { "range": { "@timestamp": { "gte": "now-15m" } } }
      ]
    }
  },
  "aggs": {
    "top_paths": {
      "terms": {
        "field": "url.path.keyword",
        "size": 10
      }
    }
  }
}
```

## DSL: Errors Over Time

```json
GET logs-*/_search
{
  "size": 0,
  "query": {
    "bool": {
      "filter": [
        { "term": { "service.name.keyword": "payments-api" } },
        { "term": { "log.level.keyword": "error" } },
        { "range": { "@timestamp": { "gte": "now-1h" } } }
      ]
    }
  },
  "aggs": {
    "errors_over_time": {
      "date_histogram": {
        "field": "@timestamp",
        "fixed_interval": "1m"
      }
    }
  }
}
```

## `text` vs `keyword`

`text`:
- full-text search;
- не лучший вариант для exact match.

`keyword`:
- exact match;
- aggregations;
- sorting;
- term queries.

Обычно:
- `message` ищут как `text`;
- `service.name`, `env`, `trace.id`, `user.id`, `url.path` нужны как `keyword`.

Пример:
- значение `service.name = "url-shortener"`
- `match` по `service.name` ищет по словам/токенам
- `term` по `service.name.keyword` ищет ровно `"url-shortener"`
- `wildcard` по `service.name.keyword` может искать `*shortener*`, но это дороже

## `match` vs `term` vs `wildcard`

`match`:

```json
GET logs-*/_search
{
  "query": {
    "match": {
      "service.name": "shortener"
    }
  }
}
```

`term`:

```json
GET logs-*/_search
{
  "query": {
    "term": {
      "service.name.keyword": "url-shortener"
    }
  }
}
```

`wildcard`:

```json
GET logs-*/_search
{
  "query": {
    "wildcard": {
      "service.name.keyword": "*shortener*"
    }
  }
}
```

Коротко:
- `match` = full-text
- `term` = exact match
- `wildcard` = поиск по шаблону, обычно дороже

## Реальный Пример По Логам

Пусть документ выглядит так:

```json
{
  "service": "shortener",
  "level": "WARN",
  "msg": "async link.visited publish failed",
  "operation": "publish_link_visited",
  "error_kind": "publish_failed",
  "error": "publish link.visited: Kafka write errors (1/1), errors: [kafka.(*Client).Produce: dial tcp: lookup kafka on 127.0.0.11:53: no such host]",
  "event_id": "6236ed04-9e0d-4954-8042-8efbddfd184e",
  "link_id": "019d77a8-d868-7654-9995-4e12227ab1c2",
  "short_code": "oQCg7hp",
  "container_name": "/docker-compose-shortener-1"
}
```

И в индексированных полях есть:
- `service`
- `service.keyword`
- `msg`
- `msg.keyword`
- `error`
- `error.keyword`
- `operation.keyword`
- `event_id.keyword`
- `link_id.keyword`

Если написать:

```text
service.name.keyword: *shortener*
```

это не сработает, потому что поля `service.name.keyword` в документе нет.

Нужно использовать реальные поля из mapping/data view:
- `service`
- `service.keyword`

## Как Искать, Если Не Знаешь Точное Значение

### 1. Начни с `text`-поиска по словам

Если знаешь только часть значения или одно слово:

```text
service: shortener
```

```text
msg: publish
```

```text
error: kafka
```

```text
error: "no such host"
```

Это обычно лучший первый шаг.

### 2. Если знаешь точное значение, используй `.keyword`

```text
service.keyword: "shortener"
```

```text
operation.keyword: "publish_link_visited"
```

```text
event_id.keyword: "6236ed04-9e0d-4954-8042-8efbddfd184e"
```

### 3. Если нужен contains по строке, используй wildcard

```text
service.keyword: *short*
```

```text
msg.keyword: *publish failed*
```

```text
error.keyword: *no such host*
```

Важно:
- в KQL wildcard чаще пишут без кавычек;
- то есть `service.keyword: *short*`, а не `service.keyword: "*short*"`.

## Готовые KQL Запросы Для Этого Документа

Найти все WARN-логи сервиса:

```text
service.keyword: "shortener" and level.keyword: "WARN"
```

Найти все ошибки публикации:

```text
service.keyword: "shortener" and operation.keyword: "publish_link_visited"
```

Найти все Kafka-related ошибки:

```text
service.keyword: "shortener" and error: kafka
```

Найти DNS-проблему:

```text
error: "no such host"
```

Найти конкретное событие:

```text
event_id.keyword: "6236ed04-9e0d-4954-8042-8efbddfd184e"
```

Найти по `link_id`:

```text
link_id.keyword: "019d77a8-d868-7654-9995-4e12227ab1c2"
```

Найти по `short_code`:

```text
short_code.keyword: "oQCg7hp"
```

Найти по контейнеру:

```text
container_name.keyword: "/docker-compose-shortener-1"
```

Найти по части имени контейнера:

```text
container_name.keyword: *shortener*
```

Найти по сообщению без точного совпадения:

```text
msg: publish and msg: failed
```

Найти по точной строке сообщения:

```text
msg.keyword: "async link.visited publish failed"
```

## Правило На Практике

Если не знаешь точного значения:
- начинай с `service: shortener`, `msg: publish`, `error: kafka`.

Если знаешь точное значение:
- используй `.keyword`.

Если нужен contains:
- пробуй `*.keyword` с wildcard, но помни, что это обычно дороже exact match.

## Полезные Поля В Логах

- `@timestamp`
- `service.name`
- `service.version`
- `env`
- `log.level`
- `trace.id`
- `span.id`
- `request_id`
- `user.id` / `tenant.id`
- `http.method`
- `url.path`
- `http.response.status_code`
- `event.duration`
- `error.type`
- `error.message`

## Частые Ошибки

- искать без time range;
- искать по всему кластеру без `service.name`;
- хранить важные данные только в `message`;
- использовать wildcard там, где возможен exact match;
- не логировать `trace.id`;
- не различать `text` и `keyword`.

## Quick Incident Queries

Всплеск `500`:

```text
service.name : "payments-api" and http.response.status_code >= 500
```

Проблема после rollout:

```text
service.name : "payments-api" and service.version : "1.8.4" and log.level : "error"
```

Сломанная trace propagation:

```text
service.name : "payments-api" and not trace.id : *
```

## Related

- [Kibana And Elasticsearch](./kibana-and-elasticsearch.md)
- [Logging And Log Shipping README](./README.md)
