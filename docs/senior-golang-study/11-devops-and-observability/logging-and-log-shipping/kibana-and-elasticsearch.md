# Kibana And Elasticsearch

Эта тема важна не только для search-платформ, но и для обычного backend production support. На практике Kibana и Elasticsearch часто используются как основной инструмент для расследования инцидентов по логам.

## Содержание

- [Что нужно понимать](#что-нужно-понимать)
- [Базовая модель данных](#базовая-модель-данных)
- [`text` vs `keyword`](#text-vs-keyword)
- [`match` vs `term` vs `wildcard`](#match-vs-term-vs-wildcard)
- [Как работать в Kibana](#как-работать-в-kibana)
- [KQL And DSL In Practice](#kql-and-dsl-in-practice)
- [Что искать в реальном инциденте](#что-искать-в-реальном-инциденте)
- [Что важно логировать](#что-важно-логировать)
- [Частые ошибки](#частые-ошибки)
- [Когда Elasticsearch хорош, а когда нет](#когда-elasticsearch-хорош-а-когда-нет)
- [Что могут спросить на интервью](#что-могут-спросить-на-интервью)
- [Связанные темы](#связанные-темы)

## Что нужно понимать

`Elasticsearch`:
- хранит и индексирует документы;
- хорошо подходит для полнотекстового поиска, фильтрации и агрегаций;
- часто используется как log storage для structured logs.

`Kibana`:
- UI поверх Elasticsearch;
- позволяет искать, фильтровать, строить визуализации и dashboards;
- обычно это первая точка входа, когда нужно понять, что сломалось в проде.

## Базовая модель данных

Для логов обычно документ выглядит примерно так:

```json
{
  "@timestamp": "2026-04-10T12:00:00Z",
  "service.name": "payments-api",
  "service.version": "1.8.4",
  "env": "prod",
  "host.name": "pod-7f8c9",
  "log.level": "error",
  "message": "redis timeout while reading bucket state",
  "trace.id": "7c4b2d...",
  "span.id": "e12af9...",
  "user.id": "42",
  "http.request.method": "POST",
  "url.path": "/v1/payments",
  "http.response.status_code": 500,
  "event.duration": 3200000000,
  "error.type": "timeout",
  "error.message": "context deadline exceeded"
}
```

Что важно:
- structured logs почти всегда лучше неструктурированного текста;
- для поиска критичны поля вроде `service.name`, `env`, `trace.id`, `user.id`, `log.level`, `status_code`;
- если все полезное лежит только в `message`, расследование становится медленным и хрупким.

## `text` vs `keyword`

Это одна из самых важных практических тем в Elasticsearch.

`text`:
- поле анализируется;
- хорошо подходит для полнотекстового поиска;
- плохо подходит для точных фильтров, агрегаций и сортировки.

`keyword`:
- поле хранится как точное значение;
- хорошо подходит для `term`, `terms`, grouping, sorting, exact match;
- это обычный выбор для `service.name`, `env`, `user.id`, `trace.id`, `log.level`.

Практическое правило:
- человекочитаемое сообщение ищем через `message`;
- технические идентификаторы и dimensions почти всегда должны быть `keyword`.

### Наглядный пример

Пусть в документе есть поле:

```json
{
  "service.name": "url-shortener"
}
```

Если это `text`:
- поле анализируется;
- значение обычно разбивается на токены;
- искать можно по словам или частям, зависящим от analyzer.

Если это `keyword`:
- хранится полное значение целиком;
- exact match идет по строке `"url-shortener"`.

Идея такая:
- `text` отвечает на вопрос "найди по словам";
- `keyword` отвечает на вопрос "найди ровно это значение".

## `match` vs `term` vs `wildcard`

Это самый удобный способ понять разницу на практике.

`match`:
- используется полнотекстовый поиск;
- запрос анализируется;
- Elasticsearch пытается найти документы, где в `text`-поле есть такой токен.

Хорошо подходит для:
- `message`;
- `error.message`;
- search-like полей.

`term`:
- ищется точное значение;
- без анализа и разбиения на слова;
- это обычный выбор для exact filters и aggregations.

Хорошо подходит для:
- `service.name.keyword`;
- `trace.id.keyword`;
- `user.id.keyword`;
- `env.keyword`.

`wildcard`:
- это поиск по шаблону;
- он не равен нормальному full-text search;
- часто дороже exact match и может быть тяжелым на больших объемах.

Подходит, когда:
- нужно быстро найти по маске;
- exact match не подходит;
- поле небольшое и сценарий редкий.

Не стоит делать default choice:
- для больших индексов;
- для частых production queries;
- там, где можно использовать `term` или нормальный `match`.

### Короткое правило

`match`:
- искать по словам в `text`.

`term`:
- искать ровно значение в `keyword`.

`wildcard`:
- искать по шаблону, но осторожно из-за цены.

Готовые примеры `match`, `term` и `wildcard` вынесены в [Kibana And Elasticsearch Cheatsheet](./kibana-and-elasticsearch-cheatsheet.md).

## Как работать в Kibana

Частый workflow в инциденте:

1. Ограничить time range.
2. Отфильтровать `env` и `service.name`.
3. Посмотреть всплеск по `log.level:error` или по `status_code >= 500`.
4. Найти конкретный `trace.id`, `user.id`, `request_id` или endpoint.
5. Перейти от общей картины к конкретным документам.
6. Проверить, это единичный кейс, hot key, rollout issue или системная деградация.

Главная ошибка:
- искать по всему кластеру без time range и без фильтров по сервису.

## KQL And DSL In Practice

Для ежедневной работы обычно хватает двух режимов:
- KQL в Kibana Discover, когда нужно быстро сузить выборку;
- Query DSL в Dev Tools, когда нужны точные filters, `term` queries и aggregations.

Типовые сценарии:
- найти все ошибки сервиса за последние 15 минут;
- собрать события по конкретному `trace.id`;
- найти медленные запросы;
- посмотреть top endpoints по числу `500`;
- сравнить поведение между двумя версиями сервиса.

Готовые KQL и DSL примеры вынесены в [Kibana And Elasticsearch Cheatsheet](./kibana-and-elasticsearch-cheatsheet.md).

## Что искать в реальном инциденте

### Всплеск `500`

Смотри:
- какой сервис;
- какой endpoint;
- какой `service.version`;
- есть ли корреляция с rollout;
- одинаковый ли `error.type`;
- это массовая ошибка или конкретный tenant/user.

### Рост latency

Ищи:
- большие `event.duration`;
- correlation с `trace.id`;
- ошибки Redis/Postgres/HTTP downstream;
- конкретные пути и tenants;
- различия между версиями сервиса.

### Проблемы после деплоя

Часто полезно сравнить:
- новую `service.version`;
- старую `service.version`;
- уровень ошибок и latency на одинаковом time range.

## Что важно логировать

Чтобы Kibana реально помогала, в логах обычно нужны:
- `@timestamp`;
- `service.name`;
- `service.version`;
- `env`;
- `log.level`;
- `trace.id` и `span.id`;
- `request_id` при наличии;
- `user.id` или `tenant.id`, если это допустимо;
- `http.method`, `url.path`, `status_code`;
- `error.type`, `error.message`;
- `duration`.

Если этих полей нет, даже хороший Elasticsearch не спасет.

## Частые ошибки

- хранить все только в `message`;
- использовать full-text там, где нужен exact match;
- не отделять `text` и `keyword`;
- искать без time range;
- строить wildcard-запросы по огромным полям без необходимости;
- не логировать `trace.id`, а потом пытаться склеить цепочку руками;
- слать в Elasticsearch слишком шумные debug-логи без sampling и retention strategy.

## Когда Elasticsearch хорош, а когда нет

Подходит, когда:
- нужен поиск по логам;
- нужны фильтры и агрегации по событиям;
- нужен near real-time operational search;
- важны dashboards и incident investigation.

Плохо подходит, когда:
- нужен OLTP workload как в PostgreSQL;
- нужны сложные транзакции и жесткая консистентность;
- Elasticsearch пытаются использовать как primary database "на все случаи".

## Что могут спросить на интервью

- чем `keyword` отличается от `text`;
- как бы ты искал причину всплеска `500` в Kibana;
- какие поля обязательно должны быть в structured logs;
- как найти все события одного запроса через `trace.id`;
- почему wildcard-поиск может быть дорогим;
- когда Elasticsearch хорош как log/search storage, а когда лучше взять другую систему.

## Связанные темы

- [Logging And Log Shipping README](./README.md)
- [DevOps And Observability README](../README.md)
