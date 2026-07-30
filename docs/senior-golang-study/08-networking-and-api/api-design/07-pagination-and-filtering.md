# 07. Пагинация и фильтрация

## Содержание

- [Пагинация: cursor-based (AIP-158)](#пагинация-cursor-based-aip-158)
- [Фильтрация](#фильтрация)
- [Сортировка](#сортировка)
- [Field selection / partial response](#field-selection--partial-response)
- [Включение связанных ресурсов](#включение-связанных-ресурсов)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные документы](#связанные-документы)

Пагинацию и фильтрацию надо проектировать с первого дня, на каждой
коллекции. Дописывать её потом — больно, потому что меняет контракт. Если на
endpoint'е `GET /v1/orders` сейчас 50 заказов и нет pagination, завтра у одного
пользователя их будет 10000 — endpoint просто ляжет.

---

## Пагинация: cursor-based (AIP-158)

Стандарт Google AIP-158, такой же у Stripe, GitHub, Slack, Square.

```protobuf
message ListOrdersRequest {
  // Размер страницы. Если 0 — сервер выбирает дефолт.
  // Сервер обязан ограничивать сверху (max page size).
  int32 pageSize = 1;

  // Непрозрачный токен из предыдущего ответа.
  // На первой странице — пустая строка.
  string pageToken = 2;

  // Фильтры — обычные поля.
  repeated OrderStatus status = 3;
  google.type.Date fromDate = 4;
}

message ListOrdersResponse {
  repeated Order orders = 1;

  // Токен для следующей страницы. Пустая строка = больше нет.
  string nextPageToken = 2;
}
```

Использование:

```text
GET /v1/orders?pageSize=50
→ { "orders": [...], "nextPageToken": "eyJsYXN0SWQiOiJhYmMtMTIzIn0=" }

GET /v1/orders?pageSize=50&pageToken=eyJsYXN0SWQiOiJhYmMtMTIzIn0=
→ { "orders": [...], "nextPageToken": "..." }

# Последняя страница:
GET /v1/orders?pageSize=50&pageToken=...
→ { "orders": [...], "nextPageToken": "" }
```

### Что такое pageToken

Это opaque-токен (для клиента — чёрный ящик), который сервер декодирует во
внутреннее состояние «откуда продолжить».

Типичные реализации:

- **Base64-кодированный JSON** с последним id и сортировкой:
  `{"lastId": "abc-123", "sort": "createdAt"}` → base64.
- **Подписанный токен** (HMAC) — если важно, чтобы клиент не подделал.
- **Sequence-based** для append-only логов: монотонный offset.

Главное — клиент не парсит токен. Он его просто шлёт назад. Можно менять
реализацию токена без breaking change для клиентов.

### Почему cursor, а не page-based

Page-based (`?page=3&pageSize=20`) — самый привычный, но имеет проблемы:

- **Hop on insert/delete.** Если между запросом страницы 1 и 2 кто-то вставил
  запись — клиент пропустит элемент или увидит дубль.
- **Дорогой COUNT.** Для `?page=N` сервер обычно вынужден считать `total`. На
  больших таблицах `SELECT COUNT(*)` — это full scan.
- **Дорогой OFFSET.** На странице 1000 запрос делает `OFFSET 20000` — БД
  пропускает 20000 строк, чтобы вернуть 20.

Cursor-based:

- Стабилен под изменениями (новые записи не сдвигают существующие токены).
- Не требует COUNT.
- Дешевле в БД: `WHERE id > <last_id> ORDER BY id LIMIT N` — index seek.

### Когда page-based оправдан

- UI требует «страница 5 из 27» (например, админка с jump-to-page).
- Данные не меняются (статичный справочник).
- Размер коллекции маленький, и `COUNT(*)` дешёвый.

Тогда:

```protobuf
message ListOrdersRequest {
  int32 page = 1;
  int32 pageSize = 2;
  // фильтры
}

message ListOrdersResponse {
  repeated Order orders = 1;
  int32 page = 2;        // echo
  int32 pageSize = 3;    // echo
  int32 totalItems = 4;
  int32 totalPages = 5;
}
```

Можно совмещать — page-based для admin, cursor для public.

### maxPageSize обязателен

Без верхнего ограничения клиент пришлёт `pageSize=100000` и положит сервис.

Google-style:

```protobuf
message ListOrdersRequest {
  // Максимум 100. Если 0 — дефолт 20.
  int32 pageSize = 1;
  string pageToken = 2;
}
```

И в коде сервера:

```go
const maxPageSize = 100
const defaultPageSize = 20

if req.PageSize <= 0 {
    req.PageSize = defaultPageSize
}
if req.PageSize > maxPageSize {
    req.PageSize = maxPageSize  // молча clamp, не ошибка
}
```

Документировать оба числа в API docs.

---

## Фильтрация

Три стиля. Один из них применять стабильно на всём API.

### Стиль A. Query-параметры стандартного вида

Самый простой и привычный:

```text
GET /v1/orders?status=paid&from=2026-01-01&category=ski
```

В proto:

```protobuf
message ListOrdersRequest {
  int32 pageSize = 1;
  string pageToken = 2;

  // Фильтры — обычные поля
  repeated OrderStatus status = 3;
  google.type.Date fromDate = 4;
  google.type.Date toDate = 5;
  string category = 6;
}
```

В HTTP query через grpc-gateway:

```text
?status=ORDER_STATUS_PAID&status=ORDER_STATUS_PENDING&fromDate=2026-01-01
```

Плюсы: простой, прозрачный, документируется в OpenAPI как обычные params.

Минусы: при сложной логике (AND/OR/NOT, ranges) получается длинная подпись
запроса. Для большинства публичных API этого хватает.

### Стиль B. Один `filter` string с DSL (AIP-160)

Google-style — одно поле с маленьким языком фильтров:

```protobuf
message ListOrdersRequest {
  int32 pageSize = 1;
  string pageToken = 2;

  // Filter expression, AIP-160 syntax.
  // Example: "status = paid AND createdAt > 2026-01-01"
  string filter = 3;
}
```

Использование:

```text
GET /v1/orders?filter=status%3Dpaid+AND+createdAt%3E2026-01-01
```

Плюсы: универсально, расширяемо, поддерживает сложные выражения.

Минусы: требует парсера DSL на сервере, сложнее документировать, клиенту
надо знать синтаксис.

Применяется у Google Cloud, AWS Resource Groups Tagging API, GitHub Search API.

### Стиль C. Структурированный filter object

JSON-объект с явной структурой:

```protobuf
message ListOrdersRequest {
  int32 pageSize = 1;
  string pageToken = 2;
  OrderFilter filter = 3;
}

message OrderFilter {
  repeated OrderStatus status = 1;
  DateRange createdAt = 2;
  StringMatch contactEmail = 3;
}

message StringMatch {
  string value = 1;
  MatchType type = 2;
  enum MatchType {
    MATCH_TYPE_UNSPECIFIED = 0;
    MATCH_TYPE_EQUALS = 1;
    MATCH_TYPE_CONTAINS = 2;
    MATCH_TYPE_STARTS_WITH = 3;
  }
}
```

Тогда search — это POST с body:

```text
POST /v1/orders:search
Body: { "filter": { "status": ["paid"], "createdAt": {...} }, "pageSize": 50 }
```

Плюсы: типобезопасно, нет парсинга DSL, явная схема.

Минусы: каждый new filter — обновление proto, не помещается в GET.

### Выбор

Для большинства публичных API — стиль A (query params). Простой и понятный.

Для admin/internal API с сложными запросами — стиль B или C.

Главное — единый стиль по всему API. Не мешать: `find` с `KeyFilter` в одном
сервисе, плоские query в другом, structured filter в третьем — клиент сходит с
ума.

---

## Сортировка

Опционально:

```protobuf
message ListOrdersRequest {
  int32 pageSize = 1;
  string pageToken = 2;

  // Sort: "createdAt desc", "total asc, createdAt desc"
  string orderBy = 3;
}
```

Использование:

```text
GET /v1/orders?orderBy=createdAt%20desc
```

AIP-132. Альтернатива — отдельные поля `sortBy` + `sortDir`, но `orderBy` string
гибче.

### Стабильность сортировки

При сортировке по неуникальному полю нужен дополнительный ключ:

```sql
ORDER BY createdAt DESC, id DESC
```

Иначе при одинаковых `createdAt` порядок недетерминированный, и pageToken может
дать дубли/пропуски.

---

## Field selection / partial response

Иногда клиент хочет получить только часть полей (мобильный клиент экономит
трафик):

```protobuf
message GetOrderRequest {
  string orderId = 1;
  google.protobuf.FieldMask readMask = 2;
}
```

Использование:

```text
GET /v1/orders/abc?readMask=id,status,total
```

Сервер возвращает только указанные поля. Это AIP-157.

Для GraphQL это родное, для REST — необязательно, но полезно для тяжёлых
ресурсов.

---

## Включение связанных ресурсов

JSON:API стиль (`?include=`) для подгрузки связей:

```text
GET /v1/orders/abc?include=payments,members
```

Альтернатива — отдельные endpoint'ы для подресурсов:

```text
GET /v1/orders/abc
GET /v1/orders/abc/payments
GET /v1/orders/abc/members
```

Выбор: если клиенту почти всегда нужны «order + payments» вместе, делают
`include` (один round-trip). Если иногда — отдельные endpoint'ы.

Для proto API — можно через `view` enum:

```protobuf
message GetOrderRequest {
  string orderId = 1;
  OrderView view = 2;
  enum OrderView {
    ORDER_VIEW_UNSPECIFIED = 0;
    ORDER_VIEW_BASIC = 1;
    ORDER_VIEW_FULL = 2;        // включая payments, members
  }
}
```

---

## Interview-ready answer

**1. Почему курсорная пагинация лучше постраничной?**

- Устойчивость к изменениям: вставка или удаление записи между запросами не сдвигает границы страниц, поэтому нет ни пропусков, ни дублей.
- Стоимость в базе: `WHERE id > :last ORDER BY id LIMIT n` это переход по индексу, тогда как `OFFSET 20000` заставляет базу пропустить двадцать тысяч строк.
- Отсутствие подсчёта: постраничная навигация обычно требует `COUNT(*)`, который на больших таблицах превращается в полное сканирование.
- Цена — нельзя прыгнуть на произвольную страницу и показать «5 из 27», поэтому для админок постраничный вариант остаётся оправданным.

**2. Что должно лежать в курсоре?**

- Позиция последней отданной записи по ключу сортировки плюс сам ключ сортировки и фильтры, чтобы следующая страница не поехала при смене параметров.
- Курсор непрозрачен для клиента: кодируется, а не собирается им вручную, иначе он станет частью публичного контракта.
- Обязательный дополнительный ключ — при сортировке по неуникальному полю нужен tiebreaker (`ORDER BY created_at DESC, id DESC`), иначе порядок недетерминирован и страницы разъезжаются.
- Срок жизни — курсор стоит считать недолговечным и корректно отвечать на устаревший, а не отдавать случайные данные.

**3. Какие ограничения обязательны на списочных методах?**

- Верхний предел размера страницы: без него клиент запросит сто тысяч записей и положит сервис.
- Значение по умолчанию, когда размер не передан, и явное поведение при превышении — молча урезать до максимума, а не возвращать ошибку.
- Предел глубины или срока жизни курсора для защиты от бесконечного обхода.
- Ограничение набора полей сортировки и фильтрации: произвольные поля означают произвольные планы запросов и отсутствие индексов.

---

## Связанные документы

- [03-http-methods.md](./03-http-methods.md) — `POST :search` для большого тела
  filter'а.
- [04-resource-modeling.md](./04-resource-modeling.md) — view/readMask для
  альтернативных представлений.
- [05-payloads-and-types.md](./05-payloads-and-types.md) — FieldMask.
- [13-references.md](./13-references.md) — AIP-158, AIP-160, AIP-132.
