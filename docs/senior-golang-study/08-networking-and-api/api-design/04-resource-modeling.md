# 04. Моделирование ресурсов

URL — это не только синтаксис, это **модель ресурсов**. Если модель неверная,
никакой кейс и плюрализация не спасут. Здесь — про то, как решать «это
sub-resource или top-level», «насколько глубоко вкладывать», «когда выделять
отдельный ресурс».

## Sub-resource vs top-level

Главный критерий — **глобальная уникальность id**.

| Признак | Тип ресурса |
|---|---|
| `id` глобально уникален (например, UUID) | top-level |
| `id` уникален только в контексте родителя (порядковый номер, slug, "name within parent") | sub-resource |
| Ресурс не существует без родителя | sub-resource |
| Ресурс может существовать независимо | top-level |
| Lifecycle ресурса привязан к родителю (cascade delete) | sub-resource |
| Lifecycle независимый | top-level |

Stripe-style — почти всё top-level с глобально уникальными id, плюс
фильтрация по parent:

```text
GET /v1/payments?orderId=ord_123
GET /v1/refunds?paymentId=pay_456
GET /v1/charges/{chargeId}
```

`payment` — top-level, потому что `pay_xxx` уникален во всей системе, и payment
может быть запрошен без знания order'а.

Google-style (AIP-122) — ресурс «принадлежит» родителю через resource name:

```text
GET /v1/projects/{project}/databases/{database}/documents/{document}
```

Здесь `document` имеет смысл только в контексте конкретной database. Lifecycle —
часть лестницы.

## Глубина вложенности

Эмпирическое правило — максимум 3-4 уровня (включая коллекции):

```text
/v1/orders/{orderId}                                              # 2
/v1/orders/{orderId}/payments                                     # 3
/v1/orders/{orderId}/payments/{paymentId}                         # 4 — потолок
/v1/orders/{orderId}/payments/{paymentId}/refunds                 # 5 — пересмотри
/v1/orders/{orderId}/payments/{paymentId}/refunds/{refundId}      # 6 — точно нет
```

Если глубже — это сигнал:

- Выделить sub-resource в top-level. `refund_xxx` — это самостоятельный ресурс
  со своим id, лучше `/v1/refunds/{refundId}` + фильтр.
- Или промежуточный уровень избыточен, его можно убрать.

Глубокая вложенность даёт три проблемы:

1. **URL становится длинным**: `/v1/orders/abc-123-def-456/payments/pay-789/refunds/ref-012`.
2. **Авторизация дороже**: middleware должен проверить доступ к каждому
   уровню (доступ к order → доступ к payment → доступ к refund).
3. **Менять иерархию сложно**: если завтра payments можно делать не только в
   контексте order — путь рассыпется.

## Когда `/parent/{id}/children` vs `/children?parentId=X`

Оба паттерна валидны. Выбор:

### `/parent/{id}/children` — когда

- Запрос всегда в контексте родителя (нет смысла читать «все children сразу»).
- Авторизация проверяется на уровне родителя.
- Lifecycle children'ов привязан к родителю.

```text
GET /v1/orders/{orderId}/payments     # все платежи этого ордера
GET /v1/quizzes/{quizId}/questions    # все вопросы этого квиза
```

### `/children?parentId=X` — когда

- Children — самостоятельный top-level ресурс.
- Можно агрегировать children по разным родителям (`?status=pending` без
  parentId).
- Children может существовать без родителя или менять родителя.

```text
GET /v1/payments?orderId={orderId}    # платежи ордера
GET /v1/payments?status=failed&from=2026-01-01  # все failed платежи за период
```

### Иногда оба

Не запрещено:

```text
GET /v1/payments/{paymentId}                       # прямой доступ
GET /v1/payments?orderId={orderId}                 # фильтр
GET /v1/orders/{orderId}/payments                  # удобный alias
```

Это `additional_bindings` в proto. Главное — одна семантика, и не дублировать
бизнес-логику в двух хендлерах.

## Bulk-операции

Антипаттерн — POST на коллекцию с массивом для смеси create/update:

```text
POST /v1/orders   body: { orders: [...] }
```

Непонятно: это создать N штук? Или upsert? Или batch операция?

Правильно — явный custom action:

```text
POST /v1/orders:batchCreate     body: { orders: [...] }
POST /v1/orders:batchUpdate     body: { orders: [...], updateMask: "..." }
POST /v1/orders:batchDelete     body: { orderIds: [...] }
POST /v1/orders:batchGet        body: { orderIds: [...] }
```

Это AIP-231/232/233/234/235. Bulk endpoints — отдельные от single, с явной
семантикой.

### Когда bulk нужен

- Производительность: 100 отдельных PATCH'ей vs один `batchUpdate`.
- Атомарность: либо все 100 операций пройдут, либо ни одна (`:batchUpdate` с
  транзакцией).
- Сокращение round-trip'ов с мобильного клиента.

### Когда не нужен

- Если можно в reasonable времени сделать N отдельных запросов.
- Если атомарность не нужна (можно частично).
- Если bulk усложняет ошибки (что вернуть, если одна из 100 операций упала?).

Для частично успешных bulk — `google.rpc.Status` per item в ответе:

```protobuf
message BatchUpdateOrdersResponse {
  repeated BatchUpdateOrderResult results = 1;
}

message BatchUpdateOrderResult {
  Order order = 1;          // успех
  google.rpc.Status error = 2;  // ошибка для этого item
}
```

## Подресурсы с одним значением (singleton)

Иногда у parent есть ровно один child заданного типа — настройки, профиль,
конфиг:

```text
GET    /v1/users/{userId}/profile         # один профиль на пользователя
PATCH  /v1/users/{userId}/profile
GET    /v1/orders/{orderId}/payment-plan  # один план оплаты на ордер
PUT    /v1/orders/{orderId}/payment-plan
```

Это singleton sub-resource. Не требует id потомка (его всегда один). PUT/PATCH
работают как обычно.

Когда singleton превращается в коллекцию (например, у пользователя стало
несколько профилей под разные роли) — это breaking change, требует `/v2/`.

## Альтернативные представления одного ресурса

Часто на один ресурс хочется несколько «view»:

- preview / summary / full
- public / admin

Антипаттерн — отдельные ресурсы:

```text
GET /v1/orders/{id}/summary       # OrderSummary тип
GET /v1/orders/{id}/details       # OrderDetails тип
GET /v1/orders/{id}/full          # OrderFull тип
```

Три похожих типа, три endpoint'а, три mapper'а.

Правильно — параметр `view`:

```text
GET /v1/orders/{id}                          # default view
GET /v1/orders/{id}?view=FULL                # full view
GET /v1/orders/{id}?view=SUMMARY             # минимум
```

Или `FieldMask` в запросе (Google-style):

```text
GET /v1/orders/{id}?readMask=id,status,total
```

Один ресурс, один тип, разные подмножества полей. Тип в ответе один — `Order`,
а клиент получает только запрошенные поля.

## Альтернативные «коллекции» одного ресурса

Аналогично — у одного ресурса часто есть подмножества:

```text
/v1/bundles                       # рекомендованные / активные
/v1/bundle-history                # история
/v1/saved-bundles                 # сохранённые пользователем
```

Это три разных коллекции **одного и того же** типа `Bundle`. Решения:

### Вариант A. Три top-level коллекции

```text
GET /v1/bundles
GET /v1/bundle-history
GET /v1/saved-bundles
```

Подходит, если каждая коллекция имеет свой lifecycle, поля метаданных
(saved-bundles имеют `savedAt`), и логика разная.

### Вариант B. Одна коллекция + фильтр

```text
GET /v1/bundles?type=current
GET /v1/bundles?type=history
GET /v1/bundles?type=saved
```

Подходит, если различие — это фильтр, а структура ресурса одинакова.

### Вариант C. Sub-resource через user

```text
GET /v1/users/me/bundles
GET /v1/users/me/bundle-history
GET /v1/users/me/saved-bundles
```

Подходит, если все три коллекции принадлежат пользователю.

Выбор зависит от того, насколько разные коллекции по структуре и lifecycle.

## Пример: моделирование одного домена

Возьмём «bundle» — рекомендованный набор путёвок. Что у него есть:

- Список рекомендаций (генерируется по фильтру/квизу).
- История ранее показанных.
- Сохранённые пользователем.
- Внутри bundle есть подресурсы: accommodation, private transfer.
- Custom actions: `prebook`, `refreshRates`.

Хорошее моделирование:

```text
# Основная коллекция
GET    /v1/bundles                                # листинг с фильтрами
GET    /v1/bundles/{bundleId}                     # один (полная инфа)
POST   /v1/bundles:search                         # поиск с большим телом
POST   /v1/bundles:refreshRates                   # bulk custom action

# Custom actions на одиночный bundle
POST   /v1/bundles/{bundleId}:prebook

# Подресурсы (sub-resource — потому что без bundleId не имеют смысла)
GET    /v1/bundles/{bundleId}/acc
PATCH  /v1/bundles/{bundleId}/acc                 # частичное обновление
GET    /v1/bundles/{bundleId}/pr-transfers
PATCH  /v1/bundles/{bundleId}/pr-transfers/{transferId}
DELETE /v1/bundles/{bundleId}/pr-transfers/{leg}

# Отдельные top-level коллекции (свой lifecycle, свои поля)
GET    /v1/bundle-history
GET    /v1/bundle-history/{bundleId}              # архивная запись
GET    /v1/saved-bundles
PUT    /v1/saved-bundles/{bundleId}               # save (idempotent toggle)
DELETE /v1/saved-bundles/{bundleId}
```

11 endpoint'ов, каждый с очевидной семантикой. Никаких `update`/`list`/`refresh`
в путях, никаких дублей, sub-resources явно выражают принадлежность.

## Связанные документы

- [02-url-design.md](./02-url-design.md) — правила написания URL.
- [03-http-methods.md](./03-http-methods.md) — какой метод когда.
- [05-payloads-and-types.md](./05-payloads-and-types.md) — `FieldMask` для
  `view`/`readMask`.
