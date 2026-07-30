# 01. Принципы

## Содержание

- [Аксиома 1. Payload — это только доменные данные](#аксиома-1-payload--это-только-доменные-данные)
- [Аксиома 2. URL — это имена ресурсов, HTTP-метод — действие](#аксиома-2-url--это-имена-ресурсов-http-метод--действие)
- [Аксиома 3. Один источник правды на доменный тип](#аксиома-3-один-источник-правды-на-доменный-тип)
- [Как эти три аксиомы связаны](#как-эти-три-аксиомы-связаны)
- [Чек-лист для каждого нового endpoint](#чек-лист-для-каждого-нового-endpoint)

Три аксиомы, на которых стоит всё остальное в этом разделе. Если их не
выполнить, никакой кейс URL и плюрализация уже не спасут.

---

## Аксиома 1. Payload — это только доменные данные

В request/response message'ах живут поля бизнес-объекта: id ресурса,
содержательные поля (`amount`, `status`, `dateRange`, `members`), фильтры
запроса.

Не живут — cross-cutting concerns:

| Не в payload, а в metadata/headers | Где правильно |
|---|---|
| `userId`, `tenantId`, `actorId` | `Authorization` header → JWT → ctx |
| `locale`, `language` | `Accept-Language` |
| `currency` (когда это пользовательский preference, не данные ресурса) | `X-Currency` или из профиля по JWT |
| `traceId`, `requestId`, `spanId` | `traceparent` (W3C Trace Context), `X-Request-Id` |
| `idempotencyKey` | `Idempotency-Key` |
| `clientVersion`, `userAgent` | `User-Agent`, `X-Client-Version` |
| `deadline`/`timeout` | gRPC deadline / `Request-Timeout` header |

Почему это правило одно решает кучу проблем:

- **Контракт не врёт клиенту.** Когда `userId` лежит в payload, OpenAPI его
  показывает как input. Фронт начинает заполнять, партнёр через год пришлёт
  чужой и удивится «работает». Если поля нет — нечем ошибиться.
- **Не нужно дублировать external/internal message'и.** Когда `userId` —
  поле message'а, внешний контракт «нельзя» иметь его как input от клиента, а
  внутреннему сервису он нужен. Появляются два набора одинаковых типов с одной
  разницей и мапперы между ними. Если же `userId` — это metadata, message один и
  тот же.
- **Безопасность не висит на gateway.** Если gateway забудет перезаписать
  `userId` из токена в одном из endpoint'ов — privilege escalation. Если поля в
  payload нет вообще — нечего забыть.
- **PII не уезжает в request-log клиента.** `userId` в теле запроса — это PII,
  отправленный клиентом. `userId` в JWT — это server-side enrichment, и в логе
  он появляется только осознанно.

Формальный признак, что поле — cross-cutting: оно одинаково для всех ручек, и
если бы можно было передавать его автоматически на каждый запрос — никто бы не
заметил. `userId` именно такой: клиент его не «выбирает», он принадлежит сессии.

---

## Аксиома 2. URL — это имена ресурсов, HTTP-метод — действие

Простая проверка: посмотреть на путь без HTTP-метода. Если из пути понятно, что
эндпойнт *делает* — путь спроектирован неправильно.

```
POST /v1/orders/{id}/cancel      # глагол в пути — анти-паттерн
POST /v1/orders/{id}:cancel      # путь = ресурс, действие через :verb
```

Глаголы, которые не должны появляться в URL:
`update`, `refresh`, `list`, `find`, `replace`, `delete`, `get`, `create`,
`save`, `apply`, `start`, `stop`, `complete`. За них отвечает HTTP-метод или
nullipotent custom action.

Когда custom action действительно нужен — нотация AIP-136:

```
POST /v1/orders/{orderId}:cancel
POST /v1/quizzes/{quizId}:duplicate
POST /v1/bundles/{bundleId}:prebook
POST /v1/bundles:search           # custom action на коллекцию
```

Это позволяет выразить операции, которые не ложатся в CRUD (отмена с побочным
эффектом, дубликат с генерацией id, prebook с резервированием), не превращая
URL в RPC-вызов.

Полное обоснование выбора методов — в [03-http-methods.md](./03-http-methods.md).

### Почему путь — это не имя функции

В мире gRPC есть `service Foo { rpc UpdateBundleHotelItem(...) }`, и при
накатывании HTTP-аннотации соблазн перевести имя rpc буквально:
`/v1/bundle/update-hotel-item` или `/v1/rec/bundle/acc/update`. Это работает, но
теряется главная ценность REST: клиент по виду URL может предположить операцию.

Один и тот же rpc может в HTTP стать стандартным CRUD:

```protobuf
rpc UpdateBundleHotelItem(UpdateBundleHotelItemRequest) returns (OfferBundle) {
  option (google.api.http) = {
    patch: "/v1/bundles/{bundleId}/acc"
    body: "*"
  };
}
```

`PATCH` на под-ресурс `acc` бандла — без `update` в URL, без `bundle/acc/update`.

---

## Аксиома 3. Один источник правды на доменный тип

`Order`, `Bundle`, `User`, `Payment` — каждый определён ровно в одном месте
(обычно `common/<domain>.proto`). Везде, где этот тип используется (external
service, internal service, событие в Kafka, ответ админ-панели), это тот
же message.

Что эта аксиома убирает:

- **Дублирование Update*Request.** Вместо `UpdatePromoCodeRequest` с
  повторением 15 полей `PromoCode` — `FieldMask` (AIP-134):

  ```protobuf
  message UpdatePromoCodeRequest {
    PromoCode promoCode = 1;
    google.protobuf.FieldMask updateMask = 2;
  }
  ```

  Один `PromoCode` живёт в Create-ответе, Update-запросе, Read-ответе.
- **Дублирование external/internal payloads.** Один `Order` используется и в
  публичном `BookingService`, и во внутреннем `BookingInternalService`. Они
  различаются только `service`-определением и HTTP-аннотациями.
- **Дублирование response shape'ов.** `OrderSummary`, `OrderPreview`,
  `OrderDetails` — если они отличаются только наличием/отсутствием полей,
  лучше один `Order` + `FieldMask` в ответе или поле `view=SUMMARY|FULL`.

Подробнее — в [05-payloads-and-types.md](./05-payloads-and-types.md) §FieldMask
и в [10-protobuf-repo-layout.md](./10-protobuf-repo-layout.md).

### Когда дублирование оправдано

Не всегда «один тип — один файл». Иногда нужны разные типы:

- **Жирная разница в данных.** Internal `Payment` знает
  `stripe_account_id`, `internal_risk_score`, `processor_fees_breakdown` — это
  нельзя отдавать наружу даже случайно. Тогда `PublicPayment` и `InternalPayment`
  — разные типы, и mapper между ними — это явная anti-corruption boundary, а не
  лишняя работа.
- **Версионная развязка.** External `/v1/` застрял (клиенты не обновляются),
  internal эволюционирует. Mapper становится трансляцией между поколениями
  схем.
- **PII / GDPR.** Response для external скрывает поля для пользователей без
  специальных прав. Лучше иметь отдельный sanitized тип, чем фильтровать поля
  ad-hoc в каждом хендлере.

В этих случаях mapper:

1. Кладётся в один edge-слой (BFF/gateway), не в каждом сервисе.
2. Генерируется (codegen), а не пишется руками.

---

## Как эти три аксиомы связаны

Все три об одном: минимизировать места, где можно ошибиться.

- Аксиома 1 убирает поля, в которых ошибка = security issue.
- Аксиома 2 делает URL предсказуемым и инструментально-валидируемым (любая
  CDN/прокси/линтер «понимает» REST, но не понимает RPC-в-URL).
- Аксиома 3 убирает места, где можно рассинхронизировать схемы.

Если хоть одна нарушена — будут протекать ошибки. Если выполнены все три —
80% контракта проектируется сам собой, остаются только содержательные решения
про модель данных и бизнес-операции.

---

## Чек-лист для каждого нового endpoint

Перед добавлением нового HTTP endpoint пройти три вопроса:

1. **Метод и путь:** «Если я скажу путь и метод вслух, поймёт ли коллега, что
   эндпойнт делает?» Если нужно объяснять — путь неправильный.
2. **Payload:** «Есть ли в request message хоть одно поле, которое одинаково
   для всех ручек этого сервиса (`userId`, `lang`, `traceId`)?» Если есть —
   убрать в metadata/header.
3. **Тип ресурса:** «Использую ли я уже существующий доменный тип, или создаю
   новый message ради этого endpoint?» Если новый — точно ли он нужен, или
   достаточно `FieldMask`/`view`/`oneof`?

Эти три вопроса в ревью PR ловят 80% будущих болей.
