# Elasticsearch And OpenSearch

Elasticsearch и OpenSearch — search and analytics engines на основе Apache Lucene.

## Содержание

- [Где используется](#где-используется)
- [Как устроено: inverted index и mapping](#как-устроено-inverted-index-и-mapping)
- [Ключевые понятия: морфология, фасеты, facet counts](#ключевые-понятия-морфология-фасеты-facet-counts)
- [Derived model: не primary storage](#derived-model-не-primary-storage)
- [Как данные попадают в индекс (Indexer)](#как-данные-попадают-в-индекс-indexer)
- [Сильные стороны](#сильные-стороны)
- [Слабые стороны](#слабые-стороны)
- [Когда выбирать](#когда-выбирать)
- [Когда не выбирать](#когда-не-выбирать)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)
- [Query examples](#query-examples)

## Где используется

- full-text search;
- log search (ELK stack, OpenSearch Dashboards);
- observability;
- filtering и faceted search;
- search relevance scoring;
- security/event analytics.

## Как устроено: inverted index и mapping

`Inverted index` — основа поиска. Для каждого слова хранится список документов, в которых оно встречается. Поиск "kubernetes" → мгновенно находим все документы с этим словом.

`Mapping` — описание типов полей. Критически важно для производительности:

- `text` — токенизируется, анализируется; подходит для full-text search; НЕ подходит для точного match и aggregation;
- `keyword` — хранится as-is; для точного match, aggregation, sorting.

Для поля, по которому нужен и full-text search, и aggregation/exact match, нужны оба подтипа:

```json
"title": {
  "type": "text",
  "fields": {
    "keyword": { "type": "keyword" }
  }
}
```

**Refresh interval**: по умолчанию документ становится searchable через ~1 секунду после записи (refresh). Это eventual consistency для поиска — не подходи к ES/OS с ожиданием мгновенной видимости записи.

**Shards и replicas**: индекс делится на shards (горизонтальное шардирование). Replicas — копии для HA и read throughput. Количество primary shards фиксируется при создании индекса — меняется через reindex.

## Ключевые понятия: морфология, фасеты, facet counts

Три термина, которые постоянно звучат рядом с full-text search и faceted search. Разбор на примере доски объявлений (поиск товаров с фильтрами).

### Морфология (стемминг / лемматизация)

**Что это:** поиск понимает разные формы одного слова. Запрос «куплю **столы**» находит объявления, где написано «**стол**», «**столу**», «**столами**». Для русского языка это обязательно — падежи и числа меняют слово, а смысл один.

**Как работает:** при индексации поле типа `text` проходит через *analyzer*, который разбивает строку на токены (токенизация) и приводит каждое слово к основе. Два подхода:
- **стемминг** — грубо отрезает окончание до «основы» (`столами` → `стол`), быстро, но иногда неточно (`университет` и `универсальный` могут схлопнуться);
- **лемматизация** — приводит к словарной форме по словарю (`шёл` → `идти`), точнее, но дороже.

```text
Без морфологии:  запрос "столы" ищет точную строку "столы"
                 → объявление "продам стол" НЕ найдётся
С морфологией:   и запрос, и документ приводятся к основе "стол"
                 → совпадение есть
```

Для русского в ES подключают analyzer с морфологией (например `russian` analyzer или плагин). Без него поиск находит только точные словоформы — для пользователя это выглядит как «ничего не нашлось», хотя товар есть.

### Фасеты и фасетный поиск (faceted search)

**Что это:** «фасет» (facet) — одна грань/характеристика товара, по которой можно отфильтровать выдачу. Это те самые галочки и ползунки слева в результатах поиска:

```text
Коробка передач:  ☐ Автомат   ☐ Механика
Год выпуска:       [ от ___ до ___ ]
Цена:              [ от ___ до ___ ]
```

**Фасетный поиск** = поиск, где результат можно сужать по нескольким фасетам одновременно. В ES каждый фасет — это поле документа, по которому идёт `filter` (term для перечислимых значений, range для диапазонов). Фасеты часто зависят от категории: у авто свои (пробег, КПП), у квартиры свои (этаж, площадь).

### Facet counts (счётчики фасетов)

**Что это:** числа в скобках рядом с каждым вариантом фильтра — сколько объявлений найдётся, если выбрать этот вариант, **ещё не выбирая его**:

```text
Коробка передач:
  ☐ Автомат  (1240)
  ☐ Механика (430)
```

Пользователь видит «по автомату 1240 вариантов» до клика — это снимает «пустые» фильтры и подсказывает, куда жать.

**Как считается:** одним запросом через *aggregations* — ES в рамках текущей выборки группирует документы по полю и считает количество в каждой группе. В обычной БД это были бы десятки отдельных `SELECT COUNT(*) ... GROUP BY` на каждый фильтр при каждом запросе — под нагрузкой нереалистично. Именно ради facet counts на больших объёмах берут Elasticsearch, а не PostgreSQL full-text search.

```text
Поисковый запрос возвращает разом:
  1) список найденных объявлений (страница результатов)
  2) facet counts по каждому фасету (для отрисовки фильтров с числами)
```

> Практический разбор этих механизмов в составе highload-системы — в кейсе
> [13. Avito / Classifieds](../../05-system-design/interview-cases/13-avito-classifieds.md) (раздел «Поиск с фасетными фильтрами»).

## Derived model: не primary storage

Elasticsearch/OpenSearch — derived read model, не source of truth:

```text
Primary DB (PostgreSQL/MongoDB) -> CDC / event stream -> ES index
```

Преимущества:
- если ES упадет, данные в primary DB целые;
- можно перестроить индекс заново из primary DB;
- primary DB хранит дорогой состояние транзакционно, ES — дешевый индекс для поиска.

Хранить в ES как единственный источник данных нельзя: нет transactions, нет FK, eventual consistency при индексировании.

## Как данные попадают в индекс (Indexer)

ES **сам ничего не «забирает»** из БД или Kafka — он лишь принимает запросы на индексацию (`POST /index/_doc`). Данные в индекс кладёт **внешний процесс** (его называют indexer / sync worker). Это не компонент Elasticsearch, а отдельный код/инструмент, разворачиваемый отдельно. Два подхода:

### 1. Самописный consumer (свой сервис)

Сервис (например на Go) читает поток изменений (Kafka-топик от CDC/outbox), денормализует запись в ES-документ и пишет батчами через **Bulk API** (по одному документу слать нельзя — ES захлебнётся).

```go
// go-elasticsearch + esutil.BulkIndexer — сам батчит документы и шлёт _bulk
bi, _ := esutil.NewBulkIndexer(esutil.BulkIndexerConfig{
    Client:        es,
    Index:         "listings",
    FlushInterval: time.Second,
})
for msg := range consumer.Messages() {       // consumer group по топику изменений
    ev := decode(msg.Value)
    bi.Add(ctx, esutil.BulkIndexerItem{
        Action:     "index",                  // или "delete"
        DocumentID: ev.ID,
        Body:       buildDoc(ev),             // денормализация / обогащение
    })
}
```

«Workers» во множественном числе — это **consumer group**: партиции топика делятся между репликами, индексация идёт параллельно и масштабируется числом реплик (до числа партиций).

### 2. Готовые коннекторы (без кода)

| Инструмент | Что делает |
|---|---|
| **Kafka Connect + Elasticsearch Sink** | конфигом льёт из топика в индекс |
| **Debezium** | CDC: читает WAL Postgres/MySQL → Kafka (часто в паре с ES Sink) |
| **Logstash** | pipeline «вход → фильтр → ES output» |

Минус коннекторов — ограниченная трансформация: кладут документ ~как есть. Как только нужна нетривиальная денормализация (склейка таблиц, вычисляемые поля, per-category схема атрибутов) — пишут свой consumer. Частый гибрид: **Debezium для CDC + свой consumer для трансформации**.

### Почему отдельный процесс, а не писать в ES прямо из сервиса

- запись в primary не должна зависеть от доступности ES (упал ES → приложение всё ещё пишет в БД);
- всплески записи буферизуются в Kafka, а не бьют в ES напрямую;
- индекс можно перестроить, переиграв поток (или full reindex из primary).

**Никогда не dual-write** — не писать в БД и в ES двумя отдельными вызовами: при сбое между ними получишь рассинхрон (в БД есть, в индексе нет). Источник изменений должен быть один: либо CDC (чтение WAL), либо transactional outbox. См. [Saga и Outbox](../../04-architecture-and-patterns/patterns/09-saga-and-outbox.md).

## Сильные стороны

- full-text search с relevance scoring;
- inverted index — молниеносный поиск по тексту;
- мощные aggregations (facets, histograms);
- distributed search по большим объемам;
- удобен для log investigation.

## Слабые стороны

- не primary transactional storage;
- eventual consistency при индексировании (refresh interval);
- mapping design важен — переделка требует reindex;
- cluster operations нетривиальны;
- storage cost при хранении raw logs растет быстро.

## Когда выбирать

Elasticsearch/OpenSearch подходит, если:
- нужен full-text search с relevance;
- нужны фильтры, facets, поиск по логам;
- PostgreSQL full-text search уже не справляется;
- нужен ELK/observability stack.

## Когда не выбирать

Не лучший выбор, если:
- нужен primary source of truth для payments/orders;
- нужны relational constraints;
- задача решается PostgreSQL `tsvector` full-text search.

## Типичные ошибки

- использовать как единственную базу для бизнес-сущностей;
- не понимать разницу `text` vs `keyword` → broken aggregations;
- не контролировать mappings → mapping explosion при динамических ключах;
- хранить бесконечные логи без index lifecycle management (ILM) → диск заканчивается;
- ожидать мгновенную видимость после записи.

## Interview-ready answer

**1. Что такое Elasticsearch/OpenSearch и какова их правильная роль?**

- Поисковые движки на инвертированном индексе (для каждого слова — список документов). Правильная роль — derived read model: source of truth остаётся в primary DB, ES/OS — перестраиваемый индекс для поиска, синхронизируемый асинхронно (events/CDC).

**2. Почему mapping критичен?**

- `text` анализируется для full-text поиска, `keyword` хранится как есть — для exact match, сортировок и агрегаций. Перепутать — получить либо неработающий точный фильтр, либо бессмысленный полнотекст; менять mapping задним числом — реиндекс.

**3. Какие эксплуатационные нюансы?**

- Индексирование eventually consistent (refresh ~1 c) — документ виден в поиске не сразу. Для логов обязателен ILM (index lifecycle management) с retention — иначе диск закончится.

## Query examples

Индексация документа:

```http
POST /products/_doc/42
Content-Type: application/json

{
  "title": "MacBook Pro 16-inch",
  "description": "Apple laptop with M3 chip",
  "status": "active",
  "price": 2499.00,
  "tags": ["laptop", "apple"]
}
```

Full-text search с фильтром:

```http
GET /products/_search
Content-Type: application/json

{
  "query": {
    "bool": {
      "must": {
        "match": { "title": "macbook" }
      },
      "filter": {
        "term": { "status.keyword": "active" }
      }
    }
  }
}
```

Aggregation (facets):

```http
GET /products/_search
Content-Type: application/json

{
  "size": 0,
  "aggs": {
    "by_tag": {
      "terms": { "field": "tags.keyword" }
    },
    "price_range": {
      "histogram": { "field": "price", "interval": 500 }
    }
  }
}
```
