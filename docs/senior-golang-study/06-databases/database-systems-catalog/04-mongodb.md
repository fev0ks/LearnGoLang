# MongoDB

MongoDB это document database, где данные хранятся как JSON-like documents.

## Где используется

- document-centric applications;
- product catalogs;
- user profiles;
- content management;
- event-like documents;
- systems where schema evolves quickly.

## Сильные стороны

- flexible document model;
- удобно хранить вложенные структуры;
- быстро менять форму данных;
- хорошо подходит для aggregate-oriented design;
- есть indexes, aggregation pipeline, replica sets, sharding.

## Слабые стороны

- не лучший выбор для сложных relational joins;
- schema flexibility может привести к хаосу;
- data duplication требует discipline;
- transactions есть, но mental model не должен превращать MongoDB в SQL DB.

## Когда выбирать

Выбирай MongoDB, если:
- данные естественно документные;
- объект часто читается и пишется целиком;
- схема активно меняется;
- нет сложных relational constraints.

## Когда не выбирать

Лучше подумать о SQL, если:
- много связей many-to-many;
- важны foreign keys and strict relational integrity;
- нужны сложные ad-hoc joins;
- business invariants лучше выражаются constraints.

## Типичные ошибки

- выбирать MongoDB только потому, что "не хочется миграций";
- делать коллекции как SQL tables и потом страдать без joins;
- не проектировать документы под access patterns;
- не контролировать schema evolution.

## Interview-ready answer

MongoDB хороша, когда данные естественно живут документами и читаются как агрегаты. Она слабее там, где domain relational и важны жесткие связи, constraints и сложные joins.

## Query examples

Коллекция создается не так явно, как SQL table. Обычно документы просто вставляются в collection.

Вставить документ:

```javascript
db.users.insertOne({
  email: "user@example.com",
  status: "active",
  profile: {
    name: "Mikhail",
    city: "Tbilisi"
  },
  created_at: new Date()
})
```

Найти один документ:

```javascript
db.users.findOne({ email: "user@example.com" })
```

Найти активных пользователей:

```javascript
db.users
  .find({ status: "active" })
  .sort({ created_at: -1 })
  .limit(50)
```

Выбрать только часть полей:

```javascript
db.users.find(
  { status: "active" },
  { email: 1, status: 1, _id: 0 }
)
```

Индекс:

```javascript
db.users.createIndex({ status: 1, created_at: -1 })
```
