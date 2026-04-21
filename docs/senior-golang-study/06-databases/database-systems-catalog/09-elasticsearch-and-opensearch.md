# Elasticsearch And OpenSearch

Elasticsearch и OpenSearch — search and analytics engines на основе Apache Lucene.

## Содержание

- [Где используется](#где-используется)
- [Как устроено: inverted index и mapping](#как-устроено-inverted-index-и-mapping)
- [Derived model: не primary storage](#derived-model-не-primary-storage)
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

Выбирай Elasticsearch/OpenSearch, если:
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

Elasticsearch/OpenSearch — поисковые движки на инвертированном индексе: для каждого слова хранится список документов. Их правильная роль — derived read model: primary DB является source of truth, ES/OS — дешевый индекс для поиска. Mapping критичен: `text` для full-text search, `keyword` для exact match и aggregation. Eventual consistency при индексировании (refresh ~1s). Для logs: обязателен ILM для retention, иначе диск закончится.

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
