# MongoDB

MongoDB это document database, где данные хранятся как JSON-like documents (BSON).

Конкретные production-сценарии с обоснованием выбора — в [04a-mongodb-real-scenarios.md](./04a-mongodb-real-scenarios.md).

## Содержание

- [Где используется](#где-используется)
- [Aggregation pipeline](#aggregation-pipeline)
- [Индексы](#индексы)
- [Read/write concerns](#readwrite-concerns)
- [Сильные стороны](#сильные-стороны)
- [Слабые стороны](#слабые-стороны)
- [Когда выбирать](#когда-выбирать)
- [Когда не выбирать](#когда-не-выбирать)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)
- [Query examples](#query-examples)

## Где используется

- document-centric applications;
- product catalogs;
- user profiles;
- content management;
- event-like documents;
- systems where schema evolves quickly.

## Aggregation pipeline

Aggregation pipeline — главный инструмент для сложных queries в MongoDB. Данные проходят через последовательность стадий (`$match`, `$group`, `$project`, `$sort`, `$lookup` и др.).

```javascript
// подсчет заказов по статусу за последние 30 дней
db.orders.aggregate([
  { $match: {
      created_at: { $gte: new Date(Date.now() - 30 * 24 * 3600 * 1000) }
  }},
  { $group: {
      _id: "$status",
      count: { $sum: 1 },
      total: { $sum: "$amount" }
  }},
  { $sort: { count: -1 } }
])
```

`$lookup` — аналог JOIN между коллекциями:

```javascript
db.orders.aggregate([
  { $match: { status: "pending" } },
  { $lookup: {
      from: "users",
      localField: "user_id",
      foreignField: "_id",
      as: "user"
  }},
  { $unwind: "$user" },
  { $project: { "user.email": 1, amount: 1 } }
])
```

`$lookup` работает, но при высоком объеме данных join'ы лучше денормализовывать на уровне документа — это filosofy MongoDB.

## Индексы

`Compound index` — для запросов по нескольким полям. Порядок полей важен: левый префикс используется при частичных запросах.

```javascript
// покрывает запросы по (status), (status, created_at), но не по (created_at) отдельно
db.orders.createIndex({ status: 1, created_at: -1 })
```

`Multikey index` — для массивов. Автоматически создается при индексации поля с массивом.

```javascript
db.products.createIndex({ tags: 1 })
// работает для запросов: db.products.find({ tags: "electronics" })
```

`Partial index` — индексирует только документы, удовлетворяющие условию.

```javascript
db.orders.createIndex(
  { user_id: 1, created_at: -1 },
  { partialFilterExpression: { status: "active" } }
)
```

`Text index` — для full-text search внутри MongoDB (ограниченный, не заменяет Elasticsearch).

```javascript
db.articles.createIndex({ title: "text", body: "text" })
db.articles.find({ $text: { $search: "kubernetes deployment" } })
```

## Read/write concerns

`Write concern` — сколько нод должны подтвердить запись до ответа клиенту:

- `w: 1` (default) — подтверждение от primary;
- `w: "majority"` — подтверждение от большинства replica set; safe для критичных данных;
- `w: 0` — fire and forget (очень быстро, без гарантий).

`Read concern` — какую версию данных читать:

- `local` (default) — читает с primary, может вернуть uncommitted данные;
- `majority` — только данные, подтвержденные большинством реплик;
- `linearizable` — строгая линеаризуемость, самый медленный.

Для financial/order данных: `w: "majority"` + `j: true` (journal) + `read concern: "majority"`.

Multi-document transactions (с MongoDB 4.0) — поддерживаются через replica set, но имеют overhead. Используй только когда действительно нужна атомарность между несколькими документами.

## Сильные стороны

- flexible document model (вложенные структуры, arrays);
- rich query language и aggregation pipeline;
- индексы: compound, multikey, partial, text, geospatial;
- replica sets для HA;
- horizontal sharding;
- быстро менять схему без миграций.

## Слабые стороны

- не лучший выбор для сложных relational joins (возможны, но дороги);
- schema flexibility → хаос без дисциплины;
- transactions есть, но overhead больше чем в PostgreSQL;
- data duplication требует discipline при denormalization.

## Когда выбирать

Выбирай MongoDB, если:
- данные естественно документные (объект читается и пишется целиком);
- схема активно меняется;
- нет сложных relational constraints;
- aggregation pipeline подходит для аналитики по документам.

## Когда не выбирать

Лучше подумать о PostgreSQL, если:
- много связей many-to-many;
- важны foreign keys и strict relational integrity;
- нужны сложные ad-hoc joins;
- business invariants лучше выражаются constraints.

## Типичные ошибки

- выбирать MongoDB только потому, что "не хочется миграций" (MongoDB тоже требует schema discipline);
- делать коллекции как SQL tables и потом страдать без joins;
- использовать `w: 1` для критичных данных;
- не проектировать документы под access patterns (денормализация);
- не контролировать schema evolution при росте команды.

## Interview-ready answer

MongoDB хороша для document-centric данных: когда объект читается и пишется целиком, схема меняется быстро, и joins не являются основным паттерном. Aggregation pipeline — мощный инструмент для server-side обработки данных. Индексы: compound (порядок полей важен), multikey для массивов, partial для частичной индексации. Write concern `majority` обязателен для критичных данных — иначе при failover потеряешь записи. Слабость: joins в MongoDB дороги, поэтому часто нужна денормализация, которая требует discipline при обновлениях.

## Query examples

Вставить документ:

```javascript
db.orders.insertOne({
  user_id: ObjectId("..."),
  status: "pending",
  amount: 150.00,
  items: [
    { sku: "ITEM-1", qty: 2, price: 50.00 },
    { sku: "ITEM-2", qty: 1, price: 50.00 }
  ],
  created_at: new Date()
})
```

Найти с фильтром:

```javascript
db.orders
  .find({ status: "pending", amount: { $gte: 100 } })
  .sort({ created_at: -1 })
  .limit(50)
```

Atomic update (findAndModify):

```javascript
db.orders.findOneAndUpdate(
  { _id: orderId, status: "pending" },
  { $set: { status: "processing", updated_at: new Date() } },
  { returnDocument: "after" }
)
```

Upsert:

```javascript
db.users.updateOne(
  { email: "user@example.com" },
  { $set: { status: "active" }, $setOnInsert: { created_at: new Date() } },
  { upsert: true }
)
```
