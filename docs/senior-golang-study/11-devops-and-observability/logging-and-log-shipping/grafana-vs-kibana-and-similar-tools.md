# Grafana vs Kibana And Similar Tools

Эта заметка нужна, потому что `Grafana` и `Kibana` часто звучат рядом, но на практике это разные по природе инструменты.

## Содержание

- [Самая короткая разница](#самая-короткая-разница)
- [Сравнение в таблице](#сравнение-в-таблице)
- [Как мыслить про различие](#как-мыслить-про-различие)
- [Как это выглядит в реальной жизни](#как-это-выглядит-в-реальной-жизни)
- [Когда выбирать Grafana](#когда-выбирать-grafana)
- [Когда выбирать Kibana](#когда-выбирать-kibana)
- [Что ещё похоже и популярно сейчас](#что-ещё-похоже-и-популярно-сейчас)
- [Короткое сравнение с похожими инструментами](#короткое-сравнение-с-похожими-инструментами)
- [Practical Rule](#practical-rule)
- [Что полезно уметь сказать на интервью](#что-полезно-уметь-сказать-на-интервью)

## Самая короткая разница

Если совсем упростить:

- `Grafana` = "универсальная observability-витрина"
- `Kibana` = "Elastic-native интерфейс для поиска, анализа и управления данными в Elasticsearch"

## Сравнение в таблице

| Критерий | `Grafana` | `Kibana` |
| --- | --- | --- |
| Базовая роль | observability UI и платформа | интерфейс экосистемы `Elastic` |
| Родной мир | `Prometheus`, `Loki`, `Tempo`, `Mimir`, много внешних data sources | `Elasticsearch` и `Elastic Stack` |
| Основная сила | dashboards, correlation, multi-source observability | search, document exploration, log analysis внутри `Elastic` |
| Работа с разными источниками | очень сильная | есть интеграции, но центр тяжести всё равно `Elasticsearch` |
| Логи | хороша, особенно с `Loki` и корреляцией с traces/metrics | очень сильна, если логи уже в `Elasticsearch` |
| Метрики | очень сильна | есть, но это не тот use case, за который её чаще всего любят |
| Трейсы | сильна в составе `Grafana`-стека | возможны, если всё живёт в `Elastic`, но ecosystem fit слабее |
| Dashboards | одна из главных сильных сторон | тоже умеет, но это не единственная и не главная причина выбора |
| Search experience | нормальный, но не search-first продукт | сильный search/document workflow |
| Когда обычно выбирают | нужен единый UI поверх разных систем | уже выбран `Elastic Stack`, и нужно жить внутри него |

## Как мыслить про различие

### Grafana выросла из мира observability

У `Grafana` сильная сторона обычно в том, что она собирает в одном интерфейсе:
- metrics;
- logs;
- traces;
- alerts;
- dashboards;
- correlation между сигналами.

Поэтому `Grafana` очень естественна, когда:
- уже есть `Prometheus`;
- есть `Loki`;
- есть `Tempo`;
- нужен единый operational UI для SRE и platform teams.

### Kibana выросла из мира search и document analytics

`Kibana` особенно естественна, когда:
- данные уже лежат в `Elasticsearch`;
- нужны фильтры, search и drill-down по документам;
- нужен Elastic-native workflow для logs, search, security или analytics.

Поэтому `Kibana` часто воспринимается не просто как dashboard tool, а как интерфейс к `Elastic`-данным вообще.

## Как это выглядит в реальной жизни

Частые пары:
- `Loki -> Grafana`
- `Prometheus -> Grafana`
- `Tempo -> Grafana`
- `Elasticsearch -> Kibana`

При этом важно:
- `Grafana` умеет работать с `Elasticsearch` как data source;
- но если центр мира это `Elastic Stack`, чаще "роднее" и удобнее именно `Kibana`.

## Когда выбирать Grafana

`Grafana` чаще подходит, если:
- нужен единый UI поверх разных источников данных;
- observability строится вокруг `Prometheus`, `Loki`, `Tempo`, `Mimir`;
- важны correlation и переходы между metrics, logs и traces;
- команда хочет более open, composable и multi-backend подход.

Практическое правило:
- если вопрос звучит как "как собрать всё observability в одной витрине", очень часто ответ будет начинаться с `Grafana`.

## Когда выбирать Kibana

`Kibana` чаще подходит, если:
- основное хранилище логов и событий это `Elasticsearch`;
- нужен сильный Elastic-native search workflow;
- команда уже живёт в `Elastic Observability` или `Elastic Security`;
- важнее не multi-source UI, а глубокая работа внутри одного search backend.

Практическое правило:
- если вопрос звучит как "у нас уже всё в Elasticsearch, чем с этим жить", почти наверняка смотришь в сторону `Kibana`.

## Что ещё похоже и популярно сейчас

Если смотреть на соседние популярные продукты в том же пространстве:
- `Datadog`
- `New Relic`
- `Splunk`
- `Graylog`

Но важно:
- это не всегда прямые аналоги "один в один";
- часть из них ближе к managed observability platform;
- часть ближе к enterprise log/search platform;
- часть ближе к self-hosted log management.

## Короткое сравнение с похожими инструментами

| Инструмент | На что больше похож | Когда всплывает |
| --- | --- | --- |
| `Datadog` | ближе к managed all-in-one observability platform | когда хотят SaaS и минимум self-hosting |
| `New Relic` | тоже ближе к full observability platform | когда нужен managed platform approach |
| `Splunk` | ближе к enterprise log/search/observability platform | enterprise, security, large organizations |
| `Graylog` | ближе к centralized log management и search UI | self-hosted log management use cases |

### Datadog

`Datadog` сегодня часто выбирают, когда:
- нужен managed продукт;
- не хочется поддерживать свой observability stack;
- важен быстрый старт и единая SaaS-платформа.

Это ближе не к "ещё одной `Grafana`" и не к "ещё одной `Kibana`", а к более широкому managed observability продукту.

### New Relic

`New Relic` тоже относится скорее к широким observability platforms:
- monitoring;
- APM;
- logs;
- traces;
- dashboards;
- investigation workflows.

То есть его корректнее ставить рядом с `Datadog`, а не как чистую замену `Kibana`.

### Splunk

`Splunk` исторически очень силён в enterprise:
- search;
- logs;
- observability;
- security-adjacent сценарии.

По ощущению use case он часто ближе к `Elastic`-миру, чем к classic `Grafana`-миру, хотя продуктовая линия у него шире.

### Graylog

`Graylog` стоит воспринимать как self-hosted centralized log management platform:
- ingest;
- parsing;
- routing;
- pipelines;
- search;
- dashboards;
- alerts.

По ощущению это ближе к log management / SIEM-adjacent миру, чем к "универсальной observability-витрине" в стиле `Grafana`.

## Practical Rule

Если нужно выбрать быстро:

- много разных data sources и нужен единый observability UI: `Grafana`
- всё живёт в `Elastic`: `Kibana`
- нужен managed all-in-one SaaS: смотри на `Datadog` или `New Relic`
- нужен enterprise-heavy search/security/log platform: смотри на `Splunk` или `Elastic`
- нужен self-hosted log management слой: смотри на `Graylog`

## Что полезно уметь сказать на интервью

- `Grafana` и `Kibana` пересекаются по dashboards и observability use cases, но выросли из разных экосистем.
- `Grafana` сильна как multi-source observability UI.
- `Kibana` сильна как Elastic-native интерфейс к данным в `Elasticsearch`.
- `Datadog`, `New Relic`, `Splunk` и `Graylog` тоже важно знать, но это не просто "ещё одна Grafana" или "ещё одна Kibana", а соседние платформы с другими trade-offs.
