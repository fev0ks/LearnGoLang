# Loki Log Pipeline

`Loki` часто выбирают как более дешёвую и простую систему для хранения логов, особенно если команда уже живёт в `Grafana` и не хочет платить цену полнотекстовой индексации `Elasticsearch`.

## Главная идея Loki

В `Loki` обычно индексируются не все поля документа, а только labels.

Это значит:
- хранение дешевле;
- ingestion проще;
- хорошо работает поиск по потоку логов и labels;
- нужно аккуратно выбирать labels, иначе будет проблема с cardinality.

## Базовый путь

```text
application -> stdout/file -> Promtail/Fluent Bit/Vector/Otel Collector -> Loki -> Grafana
```

## Как Loki работает концептуально

1. Приложение пишет лог.
2. Collector читает лог.
3. Collector назначает labels:
   `service`, `namespace`, `pod`, `container`, `env`.
4. Log line вместе с timestamp и labels уходит в `Loki`.
5. `Grafana` делает запросы через `LogQL`.

## Labels против полей

Это самый важный practical point.

В `Loki` labels нужны для:
- быстрого отбора потока;
- навигации по сервису, namespace, pod, env.

Не стоит делать labels из:
- `request_id`
- `user_id`
- `trace_id`
- `order_id`
- любых почти уникальных значений

Почему:
- резко растёт cardinality;
- ingestion и query становятся дорогими и нестабильными.

Хорошие labels:
- `service`
- `env`
- `cluster`
- `namespace`
- `pod`
- `container`
- `level`

Остальное лучше оставлять внутри строки лога или parsed fields.

## Как искать в Loki

Обычно workflow такой:

1. Отобрать stream по labels.
2. Затем делать text search или parse внутри выбранного потока.

Примеры идей:
- сначала `{service="payments-api", env="prod"}`
- потом искать `"timeout"` или парсить JSON-поле `request_id`

## Когда Loki хорош

Подходит, когда:
- логов много;
- нужен дешёвый retention;
- основной UI это `Grafana`;
- расследование обычно начинается с сервиса, namespace, pod и времени;
- нет жёсткой потребности в Elasticsearch-like full-text index по множеству полей.

## Когда Loki неудобен

Плохо подходит, когда:
- команда хочет искать логи как документы с мощными агрегациями;
- нужен сложный ad-hoc analysis по многим полям;
- разработчики ожидают опыт, очень похожий на `Kibana`.

## Типичный pipeline в Kubernetes

```text
Go service -> stdout JSON
Promtail or Fluent Bit DaemonSet
Loki
Grafana
```

Иногда используют object storage под капотом:
- `S3`
- `GCS`
- `Azure Blob`

Это даёт дешёвый retention для chunks и index data, но сам query engine всё равно остаётся `Loki`.

## Частые ошибки

- превращать `request_id` в label;
- класть слишком много динамических полей в labels;
- ожидать от `Loki` поведения как у `Elasticsearch`;
- не ограничивать retention и query range;
- логировать неструктурированный мусор и потом пытаться его парсить на лету.

## Loki против Elasticsearch

`Loki`:
- дешевле по storage и ingestion;
- проще для Kubernetes/Grafana-centric stack;
- требует дисциплины по labels.

`Elasticsearch`:
- мощнее по search и aggregations;
- обычно дороже;
- требует больше внимания к mappings и capacity planning.

## Что важно объяснить на интервью

- почему low-cardinality labels критичны;
- чем label-based подход отличается от document indexing;
- когда `Loki` лучше `Elasticsearch`;
- почему `Grafana + Loki` часто выбирают для platform logs в Kubernetes.
