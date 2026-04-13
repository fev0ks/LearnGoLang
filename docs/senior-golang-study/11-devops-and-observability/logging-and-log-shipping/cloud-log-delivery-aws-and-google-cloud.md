# Cloud Log Delivery: AWS And Google Cloud

Облака обычно дают managed-слой для логов, но паттерны всё равно те же:

```text
application -> collector or runtime integration -> cloud logging layer -> search or archive
```

Важно понимать, что `AWS` и `Google Cloud` это облачные платформы одного уровня.

При этом:
- `AWS` даёт managed logging и managed search через свои сервисы;
- `Google Cloud` это облачная платформа, аналогичная `AWS`;
- `GCS` это `Google Cloud Storage`, то есть object storage внутри `Google Cloud`, а не название самого облака.

## AWS: Частые паттерны

### ECS или EKS -> CloudWatch Logs

Самый частый managed-вариант:

```text
application -> stdout -> awslogs / FireLens / Fluent Bit -> CloudWatch Logs
```

Что дальше:
- искать прямо в `CloudWatch Logs Insights`;
- подпиской отправлять в `Lambda`, `Kinesis`, `Firehose`;
- дальше грузить в `OpenSearch` или `S3`.

### ECS FireLens

`FireLens` это интеграция маршрутизации логов для ECS, часто через `Fluent Bit`.

Пайплайн:

```text
application -> stdout -> FireLens(Fluent Bit) -> CloudWatch / OpenSearch / S3 / partner sink
```

Это удобно, потому что:
- приложение ничего не знает о destination;
- можно fan-out в несколько мест;
- есть buffering и routing.

### CloudWatch -> OpenSearch

Когда нужна более сильная search-платформа:

```text
application -> CloudWatch Logs -> subscription / Firehose / Lambda -> OpenSearch
```

Подходит, когда:
- `CloudWatch Logs Insights` уже мало;
- нужны dashboards и более мощный поиск;
- нужна привычная модель `Elasticsearch`.

### CloudWatch -> S3

Когда нужен архив:

```text
application -> CloudWatch Logs -> export/subscription -> S3
```

`S3` хорош для:
- long retention;
- compliance;
- дешёвого хранения.

`S3` плох как primary interactive search backend без дополнительного слоя типа:
- `Athena`
- `OpenSearch`
- `Spark`

## Google Cloud и GCS: Частые паттерны

### GKE или Cloud Run -> Cloud Logging

Managed-путь на Google Cloud обычно такой:

```text
application -> stdout -> Cloud Logging agent/runtime integration -> Cloud Logging
```

Дальше можно:
- искать в `Logs Explorer`;
- настроить sink в `BigQuery`;
- настроить sink в `Pub/Sub`;
- настроить sink в `GCS`.

### Cloud Logging -> GCS

Это классический архивный сценарий:

```text
application -> Cloud Logging -> sink -> GCS
```

Здесь важно понимать:
- `GCS` не заменяет `Elasticsearch` или `Loki`;
- это дешёвое и надёжное хранилище объектов;
- для интерактивного анализа потом нужен дополнительный инструмент.

Типичные варианты поверх `GCS`:
- `BigQuery`
- `Dataflow`
- внешняя загрузка в `Elasticsearch` или `Loki`

### Direct shipping в Elasticsearch или Loki

Если команда не хочет зависеть только от managed logging:

```text
application -> stdout -> Fluent Bit / OTel Collector -> Elasticsearch or Loki
```

Такой путь встречается и в `AWS`, и в `Google Cloud`.

## Что выбирать на практике

### Когда хватит managed logging

Подходит, если:
- команда маленькая;
- нужен быстрый старт;
- логов умеренно;
- расследования не слишком сложные;
- хочется меньше operational burden.

### Когда нужен отдельный search backend

Подходит, если:
- много сервисов и много логов;
- нужен более сильный поиск;
- нужны сложные dashboards;
- облачный query layer стал дорогим или неудобным.

### Когда нужен archive-first подход

Подходит, если:
- retention нужен длинный;
- расследование по старым логам случается редко;
- важнее снизить стоимость хранения.

Тогда часто делают:
- `7-14` дней searchable logs;
- остальные данные в `S3` или `GCS`.

## S3 и GCS: что важно помнить

`S3` и `GCS`:
- дешёвые;
- надёжные;
- хороши для retention и compliance;
- не являются сами по себе полноценным log search UI.

То есть:
- `S3` и `GCS` это storage layer;
- `Elasticsearch`, `Loki`, `CloudWatch Logs Insights`, `BigQuery`, `Athena` это query layer.

## Частые ошибки

- путать archive storage с log search system;
- слать всё в дорогое searchable storage на месяцы;
- не разделять hot retention и cold retention;
- не продумать экспорт и replay path.

## Что важно объяснить на интервью

- как бы ты собрал дешёвый и надёжный pipeline в `AWS`;
- зачем `CloudWatch` часто комбинируют с `OpenSearch` или `S3`;
- почему `Google Cloud` не равно `GCS`, и почему `GCS` не равно `Loki` или `Elasticsearch`;
- как разделить searchable retention и archive retention.
