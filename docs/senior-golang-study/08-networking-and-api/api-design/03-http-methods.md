# 03. HTTP-методы

HTTP-методы — это не просто разные «глаголы». У каждого есть формальные
свойства, на которые опираются CDN, прокси, gateway, retry-middleware, кэши и
все стандартные инструменты. Когда ты используешь метод не по назначению —
ломаешь не только эстетику, но и реальное поведение системы.

## Таблица свойств методов

| Метод | Безопасный (no side effects) | Идемпотентный | Тело в запросе | Кэшируется |
|---|---|---|---|---|
| GET | да | да | формально нет | да |
| HEAD | да | да | нет | да |
| OPTIONS | да | да | нет | нет |
| POST | нет | нет | да | редко (только с явными заголовками) |
| PUT | нет | да | да | нет |
| PATCH | нет | нет (обычно) | да | нет |
| DELETE | нет | да | формально нет (RFC 9110 разрешает, но клиенты ненадёжно) | нет |

Определения:

- **Безопасный** — повтор не имеет побочного эффекта на сервере.
- **Идемпотентный** — повтор того же запроса приводит к тому же состоянию.
  Не означает «возвращает то же», означает «состояние сервера то же».

## Когда какой метод

- `GET` — чтение без сайд-эффектов. Кешируемо, retry-безопасно.
- `POST /collection` — создание нового ресурса. Ответ: `201 Created` + ресурс.
- `POST /resource/{id}:verb` — custom action, в т.ч. `:search` для большого тела.
- `PUT /collection/{id}` — полная замена / идемпотентный «toggle on».
- `PATCH /collection/{id}` — частичное обновление, тело — дельта или `FieldMask`.
- `DELETE /collection/{id}` — удаление. Без тела.

Дальше — почему именно так, и где обычно ошибаются.

## POST вместо GET — самая частая ошибка

Антипаттерн: «фильтры большие, в URL не помещаются, делаем POST с телом».

```text
POST /v1/bundles                     # «поиск рекомендаций»
POST /v1/bundles/refresh             # обновить view
POST /v1/bundles/{id}/acc-details    # явное чтение деталей
POST /v1/quiz-sessions/{id}/page-info # чтение шага квиза
```

Что ломается:

1. **Кэширование.** CDN, browser, edge-кэш не кэшируют POST. На GET — можно
   выставить `Cache-Control`, `ETag`, `If-None-Match`. На POST — никак.
2. **Retry-policy.** Стандартные клиенты (Go's http.Transport, AWS SDK, retry
   middleware Envoy) **не повторяют POST** автоматически, потому что POST не
   идемпотентен. Сетевой обрыв → клиент не знает, дошло или нет → выбирает
   между «не повторять и пропустить» и «повторить и продублировать».
3. **Логи и метрики.** Большинство инструментов по умолчанию помечают POST как
   «write» — в трассировках и алертах. POST на чтение портит SLI «процент
   write-операций».
4. **Условные запросы.** `If-Modified-Since`, `If-None-Match`, `If-Match` —
   работают только на GET/PUT/DELETE.

### Когда чтение реально не помещается в URL

Три правильных подхода:

#### Подход A. Подходит GET с query

В 90% случаев фильтры можно выразить query-параметрами или сжатым/закодированным
токеном:

```text
GET /v1/bundles?searchCriteriaId={id}
GET /v1/bundles?searchCriteriaId={id}&refresh=true
GET /v1/bundles/{bundleId}/acc?accId={accId}
```

Особенно если у тебя уже есть концепция `searchCriteriaId` — она и есть
сохранённый фильтр.

#### Подход B. `POST /resource:search` с body

Допустимый компромисс, когда payload реально большой:

```text
POST /v1/bundles:search       # тело — SearchCriteria
POST /v1/bundles:refreshRates # тело — список bundleIds
```

Так делают Google (Drive API `/files:search`), AWS Athena, Elasticsearch
(`/_search`). Ключевое: явная нотация `:verb` говорит читателю, что это
nullipotent action (повтор безопасен, despite POST). Документируй это в
описании метода.

#### Подход C. Через ресурс «search»

Когда поиск дорогой, асинхронный или результаты живут:

```text
POST /v1/searches             # создать запрос, вернуть {searchId}
GET  /v1/searches/{searchId}/bundles?page=...
```

Это нужно, когда поиск долгий и пагинируется. Twitter и Google именно так
делают для долгих запросов.

## POST для создания ресурса

Стандарт REST:

- `POST /collection` — создание.
- Ответ: `201 Created` + `Location: /collection/{id}` + тело ресурса.
- Альтернативно: `200 OK` + тело с id (для grpc-gateway это удобнее).

```text
POST /v1/orders
Body: {...}
Response: 201
Location: /v1/orders/abc-123
Body: { "id": "abc-123", "status": "new", ... }
```

### Идемпотентность POST

POST не идемпотентен — повтор создаёт ещё один ресурс. Для сетевых ретраев
нужна явная идемпотентность.

Stripe-стиль: header `Idempotency-Key`. Клиент генерирует UUID, шлёт его на
POST. Сервер хранит ключ + ответ N часов. При повторе с тем же ключом —
возвращает тот же результат без побочки.

```text
POST /v1/payments
Idempotency-Key: 9e3b... (UUID v4 от клиента)
```

Подробно — в [07-idempotency.md](../protocols/07-idempotency.md).

Особенно критично для платежей:

```text
POST /v1/payments
POST /v1/payments/{id}:confirm
POST /v1/payments/{id}:capture
POST /v1/payment-intents
```

— все обязаны принимать `Idempotency-Key`. Без него сетевой обрыв = двойное
списание или зависший заказ.

## PUT vs PATCH

`PUT` = «замени ресурс целиком». В теле — полное представление.
`PATCH` = «частичное обновление». В теле — только дельта.

Антипример (имя метода противоречит реальному действию):

```text
PATCH /v1/admin/quiz/block/link    # rpc называется ReplaceQuizBlockLinks — это PUT
```

Replace — это PUT. Если меняешь весь ресурс — PUT. Если только часть — PATCH.

### Toggle через PUT

`PUT /collection/{id}` — идемпотентный способ «положить состояние = X»:

```protobuf
rpc SaveBundle(SaveBundleRequest) returns (EmptyResponse) {
  option (google.api.http) = {
    put: "/v1/saved-bundles/{bundleId}"
    body: "*"
  };
}
```

«Сохрани bundleId в список сохранённых». Можно повторять — дубля не будет,
потому что сохранён один и тот же. Это правильный паттерн для idempotent toggle.

Альтернатива через POST:

```text
POST /v1/saved-bundles            # body: { bundleId: "abc" }
```

Тоже валидно, но требует серверной идемпотентности (например, unique constraint
на (userId, bundleId)). С PUT — идемпотентность из коробки.

### PATCH + FieldMask

Для частичных обновлений Google-стиль (AIP-134):

```protobuf
import "google/protobuf/field_mask.proto";

message UpdatePromoCodeRequest {
  PromoCode promoCode = 1;
  google.protobuf.FieldMask updateMask = 2;
}
```

`updateMask = "description,currencyCode"` — обновятся только эти поля. Это
точнее, чем PATCH с произвольным телом, и устраняет необходимость в отдельных
Update*Request типах. Подробно — в [05-payloads-and-types.md](./05-payloads-and-types.md).

### PATCH без FieldMask

Альтернатива — JSON Merge Patch (RFC 7396) или JSON Patch (RFC 6902). Для
proto-API менее естественно, но допустимо в чисто-REST API без protobuf.

## DELETE

`DELETE /collection/{id}`. Возвращает `204 No Content` или `200 OK` с телом
удалённого ресурса.

Антипаттерны:

```text
DELETE /v1/rec/bundles/pr-transfer        # без id — что удаляем?
DELETE /v1/admin/quiz?quizId=X            # id в query
DELETE /v1/orders        body: {...}      # body на DELETE
```

Правильно:

```text
DELETE /v1/bundles/{bundleId}/pr-transfers/{leg}
DELETE /v1/admin/quizzes/{quizId}
DELETE /v1/orders/{orderId}
```

### Тело в DELETE

RFC 9110 формально разрешает, но:

- многие клиенты не отправят тело на DELETE;
- многие прокси/gateway его обрежут;
- при ретрае не все клиенты повторят тело.

Лучше **никогда** не использовать body на DELETE. Всё в path/query.

### Идемпотентность DELETE

DELETE идемпотентен по семантике: «привести состояние к "ресурса нет"».
Повторный DELETE на уже удалённый ресурс — ок, обычно возвращает `404` или
`204`. На клиенте это легко обработать.

Бывают bulk-удаления:

```text
POST /v1/orders:batchDelete    body: { orderIds: [...] }
```

Это не DELETE, это custom action — потому что у тебя несколько ресурсов и тело.

## OPTIONS и HEAD

Редко используются явно, но:

- `OPTIONS` — для CORS preflight. Gateway/middleware отрабатывает автоматически.
- `HEAD` — как GET, но только headers. Полезно для проверки существования
  ресурса без скачивания тела.

Обычно не объявляются в proto. grpc-gateway не генерирует HEAD/OPTIONS, но
HTTP-сервер часто отрабатывает их сам.

## Резюме: правило выбора метода

1. Чтение без сайд-эффектов на сервере → `GET`.
2. Не помещается в URL → `POST /resource:search` (custom action, не путать с
   созданием) и явно документируем как nullipotent.
3. Создание нового ресурса → `POST /collection`, ответ `201 + id`.
4. Полная замена существующего → `PUT /collection/{id}`.
5. Частичное обновление → `PATCH /collection/{id}` + `FieldMask`.
6. Идемпотентное «сохранение» (toggle on) → `PUT /collection/{id}`.
7. Удаление → `DELETE /collection/{id}`, без тела.
8. Бинарная активация/деактивация / любая операция, не выражаемая CRUD →
   `POST /collection/{id}:verb`.
9. Платежи и любые «опасные при повторе» операции → обязательно
   `Idempotency-Key` (header или поле).

Если эти правила применить к существующему API — три четверти роутов
переименовываются автоматически. Оставшиеся — это редкие исключения,
которые честно становятся `:verb` action'ами.

## Связанные документы

- [01-principles.md](./01-principles.md) — аксиомы дизайна.
- [02-url-design.md](./02-url-design.md) — что писать в path.
- [05-payloads-and-types.md](./05-payloads-and-types.md) — `FieldMask` для PATCH.
- [06-cross-cutting-concerns.md](./06-cross-cutting-concerns.md) — `Idempotency-Key`.
- [../protocols/07-idempotency.md](../protocols/07-idempotency.md) — реализация
  идемпотентности на сервере.
