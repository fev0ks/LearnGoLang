# Elasticsearch And OpenSearch

Elasticsearch и OpenSearch чаще обсуждают как search and analytics engines, а не как обычные transactional databases.

## Где используется

- full-text search;
- log search;
- observability;
- filtering and faceting;
- search relevance;
- security/event analytics;
- document search.

## Сильные стороны

- full-text search;
- inverted indexes;
- relevance scoring;
- aggregations;
- distributed search;
- удобны для log and event investigation.

## Слабые стороны

- не primary transactional storage;
- eventual consistency aspects around indexing;
- mapping and indexing design важны;
- cluster operations can be non-trivial;
- storage cost can grow fast.

## Когда выбирать

Выбирай Elasticsearch/OpenSearch, если:
- нужен поиск по тексту;
- нужны фильтры, facets, relevance;
- надо искать по логам и событиям;
- SQL DB плохо подходит для search use case.

## Когда не выбирать

Не лучший выбор, если:
- нужен primary source of truth для payments/orders;
- нужны relational constraints;
- нужен простой transactional CRUD;
- workload точечно решается PostgreSQL index или full-text search.

## Типичные ошибки

- использовать как единственную базу для бизнес-сущностей;
- не понимать difference between indexed document and source of truth;
- не контролировать mappings;
- создавать слишком много high-cardinality fields;
- хранить бесконечные логи без retention policy.

## Interview-ready answer

Elasticsearch/OpenSearch стоит выбирать для search and log analytics. Для transactional state лучше держать primary database отдельно, а search index рассматривать как derived/read model.

## Query examples

Индексация документа:

```http
POST /users/_doc/42
Content-Type: application/json

{
  "email": "user@example.com",
  "status": "active",
  "created_at": "2026-04-16T10:00:00Z"
}
```

Получить документ по id:

```http
GET /users/_doc/42
```

Search по exact field:

```http
GET /users/_search
Content-Type: application/json

{
  "query": {
    "term": {
      "status.keyword": "active"
    }
  }
}
```

Full-text search:

```http
GET /users/_search
Content-Type: application/json

{
  "query": {
    "match": {
      "email": "example"
    }
  }
}
```

Aggregation:

```http
GET /users/_search
Content-Type: application/json

{
  "size": 0,
  "aggs": {
    "by_status": {
      "terms": {
        "field": "status.keyword"
      }
    }
  }
}
```
