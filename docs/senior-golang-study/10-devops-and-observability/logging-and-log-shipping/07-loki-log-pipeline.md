# Loki Log Pipeline

`Loki` часто выбирают как более дешёвую и простую систему для хранения логов, особенно если команда уже живёт в `Grafana` и не хочет платить цену полнотекстовой индексации `Elasticsearch`.

## Содержание

- [Главная идея Loki](#главная-идея-loki)
- [Базовый путь](#базовый-путь)
- [Как Loki работает концептуально](#как-loki-работает-концептуально)
- [Labels против полей](#labels-против-полей)
- [Как искать в Loki](#как-искать-в-loki)
- [Когда Loki хорош](#когда-loki-хорош)
- [Когда Loki неудобен](#когда-loki-неудобен)
- [Типичный pipeline в Kubernetes](#типичный-pipeline-в-kubernetes)
- [Частые ошибки](#частые-ошибки)
- [Loki против Elasticsearch](#loki-против-elasticsearch)
- [Promtail и Grafana Alloy](#promtail-и-grafana-alloy)
- [Что важно объяснить на интервью](#что-важно-объяснить-на-интервью)

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

Исторически многие команды использовали именно `Promtail -> Loki`.

Но practically важно помнить:
- `Promtail` часто встретится в существующих кластерах и старых гайдах;
- для новых установок стоит знать про `Grafana Alloy` как более современный collector в экосистеме `Grafana`.

Поэтому в реальной жизни сегодня ты увидишь и такие варианты:

```text
application -> stdout/file -> Grafana Alloy -> Loki -> Grafana
application -> stdout/file -> Fluent Bit -> Loki -> Grafana
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

Если смотреть на более современную формулировку для новых стеков, часто уже уместнее думать так:

```text
Go service -> stdout JSON
Grafana Alloy or Fluent Bit DaemonSet
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

## Promtail и Grafana Alloy

`Promtail`:
- это loki-centric агент для сбора и отправки логов;
- его до сих пор важно знать, потому что он часто встречается в существующих инсталляциях и документации.

`Grafana Alloy`:
- это более новый collector в экосистеме `Grafana`;
- он лучше вписывается в современный unified observability-подход, где рядом живут logs, metrics, traces и profiling;
- если начинаешь новый стек с нуля, про `Alloy` стоит знать обязательно.

Практическое правило:
- старый или уже существующий `Loki`-стек очень вероятно будет с `Promtail`;
- новый дизайн логового пайплайна уже разумно сравнивать не только с `Promtail`, но и с `Grafana Alloy`, `Fluent Bit` и `OpenTelemetry Collector`.

## Что важно объяснить на интервью

- почему low-cardinality labels критичны;
- чем label-based подход отличается от document indexing;
- когда `Loki` лучше `Elasticsearch`;
- почему `Grafana + Loki` часто выбирают для platform logs в Kubernetes.
