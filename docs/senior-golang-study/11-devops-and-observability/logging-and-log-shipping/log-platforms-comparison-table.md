# Log Platforms Comparison Table

Эта заметка нужна как быстрый practical reference: что вообще есть на рынке кроме `Loki` и `Elasticsearch`, и где какой стек обычно встречается.

## Содержание

- [Короткий вывод](#короткий-вывод)
- [Сравнительная таблица](#сравнительная-таблица)
- [Как выбирать на практике](#как-выбирать-на-практике)
- [Что реально популярно в компаниях](#что-реально-популярно-в-компаниях)
- [Как не ошибиться с выбором](#как-не-ошибиться-с-выбором)
- [Короткие рекомендации](#короткие-рекомендации)
- [Когда пора дополнять или менять стек](#когда-пора-дополнять-или-менять-стек)

## Короткий вывод

Если сильно упростить:
- `Loki` часто выбирают cloud-native и Kubernetes-команды;
- `Elasticsearch` и `OpenSearch` берут, когда нужен сильный log search и self-hosted стек;
- `Graylog` встречается как удобный self-hosted log management layer поверх search backend;
- `Datadog` часто выбирают SaaS и product-команды, которым нужен managed all-in-one observability;
- `Splunk` до сих пор очень распространён в enterprise и security-heavy среде;
- `CloudWatch Logs` и `Google Cloud Logging` часто используют как дефолтный managed entry point внутри облака.

## Сравнительная таблица

| Platform | Модель | Сильные стороны | Слабые стороны | Где часто встречается |
| --- | --- | --- | --- | --- |
| `Grafana Loki` | label-based logs, Grafana-centric | дешевле индексации full-text, хорошо для Kubernetes, удобно рядом с `Prometheus` и `Tempo` | нельзя бездумно делать high-cardinality labels, ad-hoc search слабее чем у `Elasticsearch` | platform teams, Kubernetes, cloud-native компании |
| `Elasticsearch` | document search engine | сильный full-text search, фильтры, агрегации, привычный log investigation workflow | дорого на больших объёмах, требует внимания к mappings, shards, lifecycle | self-hosted observability, enterprise, команды с сильным search use case |
| `OpenSearch` | document search engine | очень похож на `Elasticsearch`, удобен как OSS/AWS-friendly вариант, хорош для log analytics | всё ещё требует ops-экспертизу, capacity planning и дисциплину по индексам | AWS-heavy компании, self-hosted команды, замена `Elastic` |
| `Graylog` | log management platform поверх backend storage | удобный UI, routing, pipelines, streams, алерты, хороший centralized log management experience | сам по себе не магический storage engine, часто требует backend вроде `OpenSearch`; меньше cloud-native momentum, чем у `Loki`/`Datadog` | self-hosted команды, классические ops-heavy environments, внутренние платформы |
| `Datadog Log Management` | managed SaaS observability | быстрый старт, хороший UX, тесная связка логов, метрик и трассировок, мало ops-нагрузки | стоимость может быстро вырасти, vendor lock-in, меньше контроля над internals | product/SaaS компании, fast-moving teams, команды без желания self-host stack |
| `Splunk` | enterprise log analytics platform | очень зрелый enterprise продукт, мощный поиск, security/SIEM ecosystem, сильная роль в больших организациях | дорого, тяжёлый и сложный стек, высокий operational и licensing overhead | крупный enterprise, regulated environments, security-heavy компании |
| `CloudWatch Logs` | managed AWS logging | нативно для `AWS`, просто стартовать, удобно для `ECS`, `EKS`, `Lambda` | менее удобен как основной долгосрочный log analytics tool, цена и UX могут стать проблемой на масштабе | AWS-first компании, небольшие и средние cloud команды |
| `Google Cloud Logging` | managed Google Cloud logging | нативно для `GKE`, `Cloud Run`, `GCE`, легко подключается к `BigQuery`, `Pub/Sub`, `GCS` | сложные расследования и долгий retention могут подтолкнуть к отдельному backend | Google Cloud-first компании |

## Как выбирать на практике

| Ситуация | Что чаще всего подходит |
| --- | --- |
| Много `Kubernetes`, нужен OSS и контроль стоимости | `Loki` |
| Нужен сильный поиск по structured logs и full-text | `Elasticsearch` или `OpenSearch` |
| Нужен self-hosted UI и centralized log management без ухода полностью в cloud SaaS | `Graylog` |
| Нужен быстрый старт и единый SaaS для logs, metrics, traces | `Datadog` |
| Большой enterprise с security и compliance-heavy landscape | `Splunk` |
| Вся инфраструктура уже в `AWS` и хочется минимальный operational overhead | `CloudWatch Logs`, иногда потом плюс `OpenSearch` или `S3` |
| Вся инфраструктура уже в `Google Cloud` | `Cloud Logging`, иногда потом плюс `BigQuery`, `GCS`, `Loki` или `OpenSearch` |

## Что реально популярно в компаниях

Часто встречается такой pattern:
- cloud-native stack: `Grafana`, `Prometheus`, `Loki`, `Tempo`, иногда `Grafana Alloy`;
- managed product stack: `Datadog`;
- classic enterprise stack: `Splunk`, `Elastic`;
- AWS-first stack: `CloudWatch Logs` плюс потом `OpenSearch` или архив в `S3`;
- pragmatic self-hosted stack: `OpenSearch` или `Graylog`.

## Как не ошибиться с выбором

Обычно решают пять вопросов:
- какой объем логов и retention;
- нужен ли сильный full-text search;
- кто будет эксплуатировать платформу;
- нужен SaaS или self-hosted;
- насколько важна correlation с metrics и traces.

Практические правила:
- если нужен document-style поиск по полям и full-text, чаще смотри на `Elasticsearch`, `OpenSearch` или `Splunk`;
- если главное это дешево собирать логи из Kubernetes и быстро искать по сервису, часто хватает `Loki`;
- если команда маленькая и не хочет ops-нагрузку, SaaS вроде `Datadog` или native cloud logging часто лучше;
- если searchable retention длинный и объемы большие, почти всегда нужен separate archive layer вроде `S3` или `GCS`.

## Короткие рекомендации

`Kubernetes`, много сервисов, OSS, `Grafana`-стек:
- чаще всего начинай с `Loki`.

Нужен сильный поиск по structured logs:
- смотри на `OpenSearch` или `Elasticsearch`.

Нужен managed all-in-one observability:
- часто это `Datadog`.

Всё уже в `AWS`:
- обычно начинают с `CloudWatch Logs`.

Всё уже в `Google Cloud`:
- обычно начинают с `Cloud Logging`.

Большой enterprise и security-heavy landscape:
- очень вероятно встретить `Splunk` или `Elastic`.

## Когда пора дополнять или менять стек

Признаки, что текущего решения уже не хватает:
- поиск слишком медленный;
- ingest и retention стали слишком дорогими;
- слишком много ручной боли вокруг индексов или labels;
- расследование инцидентов требует прыжков между несколькими системами;
- не хватает correlation между logs, metrics и traces.

Практическое правило:
- не всегда нужно полностью менять платформу;
- часто достаточно добавить hot searchable layer и отдельный archive layer.
