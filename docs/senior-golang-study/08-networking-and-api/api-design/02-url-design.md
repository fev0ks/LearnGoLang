# 02. URL-дизайн

Базовое правило REST: URL — это **имена ресурсов**, действие — это HTTP-метод.
Глаголы в URL допустимы только как кастомные операции (Google AIP-136), и для
них есть особая нотация `:verb`. Этот файл — про конкретные правила
именования.

## Правила в одном списке

1. Коллекции — во **множественном числе**: `/orders`, `/bundles`, `/quizzes`,
   `/promo-codes`, `/users`.
2. Элемент коллекции — `/<collection>/{id}`: `/orders/{orderId}`.
3. **kebab-case** в path-segment: `/promo-codes`, `/search-criteria`,
   `/page-info`, `/contact-info`.
4. **camelCase** в JSON-полях (proto3 default).
5. **ID в path**, фильтры в query.
6. Никаких глаголов в URL: `/orders/list`, `/quizzes/find`, `/bundles/update`,
   `/bundles/refresh` — нельзя.
7. Custom action — нотация `:verb`: `POST /orders/{id}:cancel`.
8. Глубина — максимум 3-4 уровня. Глубже = выделить top-level ресурс.
9. Версия в URL: префикс `/v1/`, breaking — `/v2/`. Не суффиксами в имени rpc.
10. Один путь — одна семантика. Не вешать «совсем разные» операции на один путь
    разными методами без необходимости.

Дальше — обоснование каждого правила и анти-примеры.

## Множественное число для коллекций

Конвенция: коллекция — множественное (orders), элемент — `/{id}` от коллекции.

Антипример (реальный, из ревью legacy API):

```text
/v1/order/list                   # singular + suffix 'list'
/v1/admin/promo_code/list        # singular + 'list'
/v1/admin/quiz/list
/v1/rec/bundle                   # singular коллекция
/v1/rec/bundles/pr-transfer      # plural — в том же файле!
/v1/rec/bundle/saved/{bundleId}  # должно быть /bundles/{bundleId}/saved
/v1/admin/rec/promo-offer        # singular
/v1/admin/rec/promo-offers       # plural — на 6 строк ниже singular'а
```

Правило: имена коллекций всегда во множественном числе. Это AIP-122,
стандарт у GitHub, Stripe, Twitter, Google.

После рефакторинга:

```text
GET    /v1/orders                    # был /v1/order/list
GET    /v1/orders/{orderId}          # был /v1/order/checkout?orderId=
GET    /v1/orders/{orderId}/summary  # был /v1/order/summary?orderId=
GET    /v1/bundles                   # был POST /v1/rec/bundle
GET    /v1/admin/quizzes             # был /v1/admin/quiz/list + /find
GET    /v1/admin/promo-codes         # был /v1/admin/promo_code/list
```

## kebab-case в path

В одном легаси-API одновременно встречается:

```text
/v1/order/details/contactInfo            # camelCase
/v1/order/checkout/promo_code            # snake_case
/v1/admin/promo_code                     # snake_case
/v1/rec/bundle/pr-transfer               # kebab-case
/v1/rec/search-criteria                  # kebab-case
/v1/admin/rec/user/searchCriteria        # camelCase!
/v1/admin/quiz/find/by_block             # snake_case
/v1/quiz/session/page-info               # kebab-case
```

Это худшая из мелких болей — клиент никогда не помнит, какой стиль в каком
сегменте.

Почему именно kebab-case:

- URL — case-sensitive в path-сегменте (в отличие от hostname). Нужно
  фиксировать один кейс.
- kebab-case читается лучше: дефис явно отделяет слова, не путается с
  именами переменных в коде.
- camelCase в URL технически работает, но плохо смотрится в логах/документации
  («это поле или URL?»).
- snake_case отдаёт ощущением имени таблицы БД, не URL.

Все крупные стайлгайды (Google AIP-122, Microsoft REST Guidelines, Zalando
RESTful API Guidelines, GitHub API) сходятся на kebab-case в path.

## ID в path, фильтры в query

Принцип: иерархия ресурсов отражается путём, фильтрация — query. ID
конкретного ресурса — всегда в path.

Антипаттерн:

```text
GET /v1/orders?orderId=abc                 # id в query
GET /v1/order/checkout?orderId=abc         # id в query + 'checkout' в пути
GET /v1/admin/quiz?quizId=abc              # id в query
DELETE /v1/admin/quiz?quizId=abc           # id в query на DELETE!
```

Правильно:

```text
GET    /v1/orders/{orderId}
GET    /v1/orders/{orderId}/checkout
GET    /v1/admin/quizzes/{quizId}
DELETE /v1/admin/quizzes/{quizId}
```

Почему так:

- **Логи** становятся осмысленными: в access-log путь несёт идентификатор,
  легко строить метрики «какие именно ресурсы тяжело отдаются».
- **Авторизация** на gateway/middleware (правило «пользователь имеет доступ к
  /orders/{orderId}» — простое сопоставление по path) перестаёт требовать
  парсинга query.
- **Кеширование** на CDN/прокси работает естественным образом: у пути одна
  семантика.
- **OpenAPI/Swagger** автоматически отражает иерархию ресурсов и валидирует
  обязательность id.

Query — для фильтров, сортировки, пагинации:

```text
GET /v1/orders?status=paid&from=2026-01-01&pageSize=50
GET /v1/admin/quizzes?isActive=true&category=ski
```

## Никаких глаголов в URL

Самая частая ошибка. Список глаголов, которые не должны быть отдельными
сегментами:

`list`, `find`, `search`, `update`, `replace`, `refresh`, `delete`, `get`,
`create`, `save`, `apply`, `start`, `stop`, `complete`, `cancel`, `activate`,
`deactivate`, `details`, `summary`.

Они либо избыточны (за них отвечает HTTP-метод), либо должны быть выражены
как custom action `:verb`.

Антипаттерны и их замены:

| Антипаттерн | Замена |
|---|---|
| `GET /orders/list` | `GET /orders` |
| `GET /quizzes/find?...` | `GET /quizzes?...` |
| `GET /quizzes/find/by-block?blockId=X` | `GET /quizzes?blockId=X` |
| `POST /bundles/update` | `PATCH /bundles/{id}` |
| `POST /bundles/{id}/refresh` | `POST /bundles/{id}:refresh` |
| `POST /orders/{id}/cancel` | `POST /orders/{id}:cancel` |
| `GET /orders/{id}/details` | `GET /orders/{id}` (детали — это и есть GET resource) |
| `GET /orders/{id}/summary` | `GET /orders/{id}` с полем `view=SUMMARY` или `GET /orders/{id}/summary` если это под-ресурс |
| `POST /quizzes/replace` | `PUT /quizzes/{id}` (replace = full PUT) |
| `POST /promo-codes/active` | `POST /promo-codes/{id}:activate` / `:deactivate` |
| `POST /payments/confirm` | `POST /payments/{id}:confirm` |
| `POST /quiz-sessions/start` | `POST /quiz-sessions` (создание) |
| `POST /quiz-sessions/{id}/complete` | `POST /quiz-sessions/{id}:complete` |

## Custom actions через `:verb` (AIP-136)

Когда операция не ложится в CRUD — есть конкретная нотация:

```text
POST /v1/orders/{orderId}:cancel
POST /v1/bundles/{bundleId}:prebook
POST /v1/quizzes/{quizId}:duplicate
POST /v1/payments/{paymentId}:confirm
POST /v1/templates/{templateId}:deactivate
POST /v1/bundles:search             # custom action на коллекцию
POST /v1/bundles:refreshRates       # bulk action
POST /v1/orders:batchCancel         # bulk
```

Двоеточие `:` — это валидный символ в path-сегменте (RFC 3986). grpc-gateway
его поддерживает.

Когда уместен custom action:

- Операция имеет побочный эффект, не ложащийся в CRUD (`:cancel`, `:confirm`,
  `:prebook`).
- Поиск с большим телом (`:search` с body, см. [03-http-methods.md](./03-http-methods.md)).
- Bulk-операции (`:batchGet`, `:batchUpdate`, `:batchDelete`).
- Бинарные toggle (`:activate`/`:deactivate`).

Когда **не** уместен:

- Стандартный CRUD (Create/Read/Update/Delete) — для них есть HTTP-методы.
- «Просто потому что не определился, какой метод выбрать».

## Глубина вложенности

Максимум 3-4 уровня:

```text
/v1/orders/{orderId}                                              # 2 уровня
/v1/orders/{orderId}/payments                                     # 3 уровня
/v1/orders/{orderId}/payments/{paymentId}                         # 4 уровня
/v1/orders/{orderId}/payment-plan/installments/{itemId}           # 5 — уже потолок
/v1/orders/{orderId}/payment-plan/installments/{itemId}/intents   # 6 — слишком
```

Если глубже — это сигнал, что бизнес-объект надо выделить в самостоятельный
ресурс верхнего уровня:

```text
# Вместо:
POST /v1/orders/{orderId}/payment-plan/installments/{itemId}/intents

# Лучше:
POST /v1/payment-intents
# В body: { orderId, paymentPlanItemId, ... }
```

`payment-intent` — самостоятельная сущность со своим жизненным циклом, не
просто sub-resource installment'а. Stripe именно так и делает.

## Один путь — одна семантика

Антипример:

```text
POST /v1/rec/bundle   # «получить рекомендации по фильтру» (на самом деле — чтение)
GET  /v1/rec/bundle   # «получить бандлы по searchCriteriaId» (тоже чтение)
```

Технически HTTP это позволяет (метод + путь = идентификатор операции), но
семантически два радикально разных действия под одним именем сбивают и
клиента, и систему мониторинга.

Решение: либо разные пути, либо оба метода с близкой семантикой.

```text
GET  /v1/bundles?searchCriteriaId=...   # чтение текущих по фильтру
POST /v1/bundles:search                 # поиск по большому фильтру (тело)
POST /v1/search-criteria                # создание сохранённого фильтра
```

## Подресурсы и плоские ресурсы

Иногда лучше плоский ресурс верхнего уровня, чем глубокая вложенность:

```text
# Глубокая вложенность:
GET    /v1/users/{userId}/orders/{orderId}/payments/{paymentId}

# Плоский top-level:
GET    /v1/payments/{paymentId}     # paymentId сам уникален
GET    /v1/payments?orderId=X
GET    /v1/orders/{orderId}/payments    # если нужна вложенная коллекция
```

Правило: если id ресурса глобально уникален, и его можно использовать без
родительского контекста — он top-level. Stripe-style.

Если id уникален только в контексте родителя (например, `quiz/{id}/block/{n}`,
где `n` — порядковый номер блока) — sub-resource.

## Версия — всегда в URL

`/v1/` префикс. Breaking change — `/v2/`. Не суффиксы в именах rpc или типов.

Антипример:

```protobuf
rpc SearchResorts(SearchResortsRequest) returns(SearchResortsResponse) {
  option (google.api.http) = { get: "/v1/provider/resorts" };
}
rpc SearchResortsV2(SearchResortsRequestV2) returns(SearchResortsResponseV2) {
  option (google.api.http) = { get: "/v1/provider/resorts/search" };
}
```

Версия `V2` живёт в имени RPC, в URL её нет. Клиент HTTP не видит, что один из
эндпойнтов «v2». Если завтра появится `V3` — путь будет ещё длиннее.

Правильные подходы:

1. **URL version bump:** `/v2/provider/resorts`. Старая `/v1/...` остаётся.
2. **Backward-compatible эволюция:** добавить опциональные поля в существующий
   `SearchResortsRequest`, без нового rpc.
3. **Если действительно нужно новое API:** новое имя в URL (`/v1/provider/resort-search`)
   с понятной семантикой, без суффикса `V2`.

Подробнее — [09-versioning-and-evolution.md](./09-versioning-and-evolution.md).

## Итоговая шпаргалка

Перед добавлением нового URL пройти 5 вопросов:

1. Имя первого segment после `/v1/` — во множественном числе?
2. Последний segment — это ресурс, а не глагол?
3. Если есть `{id}` — он в path, а не в query?
4. HTTP-метод соответствует тому, что делает endpoint? (см.
   [03-http-methods.md](./03-http-methods.md))
5. Если это custom action — он оформлен как `:verb`, а не отдельным
   path-сегментом?

Если на любой ответ «нет» — путь нужно переделать до публикации, потому что
после ввода в продакшен это становится breaking change для клиентов.
