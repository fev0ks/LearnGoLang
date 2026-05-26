# 05. Payload и типы данных

URL — это только половина API. Вторая половина — какие message'и (request/response)
видят клиенты как JSON. Здесь много мелких, но важных решений, которые
«навсегда» застревают в контракте.

## Время: Timestamp vs Date vs string

Самая частая беда — в одном API одновременно живут:

```protobuf
message Order {
  int64 createdAt = 1;     // unix секунды? миллисекунды? непонятно
  string dueAt = 2;        // ISO8601? просто "2026-05-26"?
  string dateOfBirth = 3;  // "YYYY-MM-DD"?
}

message DateRange {
  string from = 1;         // строка без формата
  string to = 2;
}
```

Это всё разные семантики. Нужно различать:

### Инстант (момент времени)

`createdAt`, `expiresAt`, `paidAt`, `bookedAt` — это **глобальная** точка во
времени. Тип:

```protobuf
import "google/protobuf/timestamp.proto";

message Order {
  google.protobuf.Timestamp createdAt = 1;
  google.protobuf.Timestamp expiresAt = 2;
}
```

В JSON через grpc-gateway сериализуется как RFC 3339:

```json
{"createdAt": "2026-05-26T10:30:00Z"}
```

Клиент читает как стандартную ISO-строку, парсится в любом языке.

### Локальная дата

`dateOfBirth`, `checkInDate`, `tripStartDate` — это дата без времени и без зоны.
«Чек-ин 4 января в Куршевеле» — это та же дата, в каком бы time zone клиент ни
сидел. Unix-секунды здесь категорически неправильны.

```protobuf
import "google/type/date.proto";

message Order {
  google.type.Date checkInDate = 1;
}
```

В JSON:

```json
{"checkInDate": {"year": 2026, "month": 1, "day": 4}}
```

Альтернатива — `string` в формате `YYYY-MM-DD` с явной документацией:

```protobuf
string checkInDate = 1;  // ISO 8601 date, "YYYY-MM-DD"
```

Менее типобезопасно, но проще на клиенте.

### Длительность

`tripPeriod`, `timeout`, `retryDelay`:

```protobuf
import "google/protobuf/duration.proto";

message Trip {
  google.protobuf.Duration duration = 1;
}
```

В JSON: `"7200s"` или `"PT2H"` (зависит от сериализатора).

### Никогда

- `int64 createdAt` без указания «секунды vs миллисекунды». Если очень надо
  числом — называй `createdAtMs` или `createdAtUnix` и фиксируй в комментарии.
- `string from`, `string to` для дат без указания формата. Минимум —
  комментарий «ISO 8601 date".

## Деньги

Правильно — `int64` минорные единицы + currency code:

```protobuf
message Money {
  string currencyCode = 1;    // ISO 4217: "USD", "EUR"
  int64 amountMinor = 2;      // в центах: 1099 = $10.99
}

message Order {
  Money total = 1;
  Money discount = 2;
}
```

Никаких `double` для денег — плавающая точка теряет копейки. Никаких `string` —
непонятна точность.

Имя поля несёт единицу: `amountMinor`, `priceCents`, `feeNanos`. Это
страхует от ошибки «принял за major единицы».

Antipattern (реальный из ревью):

```protobuf
message PaymentInfo {
  string amount = 1;       // "10.99" — float-as-string
  string currencyCode = 2;
}
```

Парсинг строки в локали с запятой («10,99») сломается. Минорные int — нет.

## Enum: canonical стиль

Антипаттерн (proto2-style с префиксом через `_`):

```protobuf
enum OrderStatus {
  OrderStatus_Unknown = 0;
  OrderStatus_New = 1;
  OrderStatus_Paid = 2;
}
```

Канонический стиль proto3 (Google Style Guide, AIP-126):

```protobuf
enum OrderStatus {
  ORDER_STATUS_UNSPECIFIED = 0;
  ORDER_STATUS_NEW = 1;
  ORDER_STATUS_PAID = 2;
}
```

Почему так:

- В proto enum-значения живут в **глобальном** scope внутри package. Два enum'а
  с одинаковым `Unknown` в одном package — ошибка компиляции. Префикс из имени
  enum'а решает.
- `_UNSPECIFIED = 0` — общепринятое имя нулевого значения. «Не задано», а не
  «неизвестно».
- В JSON proto3 enum сериализуется как имя:
  `"status": "ORDER_STATUS_PAID"` (канонично) vs `"status": "OrderStatus_Paid"`
  (некрасиво).

### Эволюция enum'ов

При добавлении нового значения старые клиенты не сломаются (proto3 это
позволяет — неизвестное значение остаётся числом). Но JSON-имя — публичный
контракт. Менять имя = breaking change.

Поэтому **с первого дня** используйте canonical стиль. Перейти потом — это
ломать клиентов.

## FieldMask для частичных обновлений

Антипаттерн:

```protobuf
message UpdatePromoCodeRequest {
  string id = 1;
  string description = 2;
  string currencyCode = 3;
  PromoCodeDiscountType discountType = 4;
  int64 discountValue = 5;
  int64 minOrderAmount = 6;
  // ... 15 полей, повторение PromoCode
}
```

Это и есть «mapper-боль»: 15 полей в Update + 15 в Create + 15 в Read. Любое
изменение `PromoCode` требует синхронизации трёх типов.

AIP-134 — стандарт Google:

```protobuf
import "google/protobuf/field_mask.proto";

message UpdatePromoCodeRequest {
  PromoCode promoCode = 1;
  google.protobuf.FieldMask updateMask = 2;
}
```

Запрос:

```json
{
  "promoCode": { "id": "abc", "description": "New text", "discountValue": 500 },
  "updateMask": "description,discountValue"
}
```

Сервер обновляет только поля из `updateMask`. Остальные поля в `promoCode` —
игнорируются.

Преимущества:

- Один тип `PromoCode` живёт и в Create-ответе, и в Update-запросе, и в Read.
- Не нужно перечислять поля в каждом Update*Request.
- Семантика «не передал поле» однозначна: его нет в mask = не трогать.
- Можно обнулять поля: `updateMask: "description"` + `promoCode.description: ""`
  → описание очищается. Без mask так не сделать (пустая строка vs «не указано»
  не различаются в proto3).

В HTTP-аннотации `updateMask` обычно передаётся в query:

```protobuf
rpc UpdatePromoCode(UpdatePromoCodeRequest) returns (PromoCode) {
  option (google.api.http) = {
    patch: "/v1/promo-codes/{promoCode.id}"
    body: "promoCode"
  };
}
```

Запрос: `PATCH /v1/promo-codes/abc?updateMask=description,discountValue`.

## field_behavior аннотации

Аннотации из `google/api/field_behavior.proto` управляют семантикой полей и
видимостью в OpenAPI:

```protobuf
import "google/api/field_behavior.proto";

message Order {
  string id = 1 [(google.api.field_behavior) = OUTPUT_ONLY];        // сервер ставит
  string shortId = 2 [(google.api.field_behavior) = OUTPUT_ONLY];
  string contactEmail = 3 [(google.api.field_behavior) = REQUIRED];
  google.protobuf.Timestamp createdAt = 4 [(google.api.field_behavior) = OUTPUT_ONLY];
  OrderStatus status = 5 [(google.api.field_behavior) = OUTPUT_ONLY];
  repeated OrderMember members = 6;  // mutable, OPTIONAL по умолчанию
  string userId = 7 [(google.api.field_behavior) = OUTPUT_ONLY];    // сервер заполняет из auth
}
```

Значения:

| Аннотация | Смысл |
|---|---|
| `REQUIRED` | Клиент обязан заполнить при create/update. |
| `OPTIONAL` | Можно не заполнять (default). |
| `OUTPUT_ONLY` | Только сервер заполняет (id, createdAt, computed fields). Скрывается из request-schema в OpenAPI. |
| `INPUT_ONLY` | Только клиент шлёт (password). Скрывается из response-schema. |
| `IMMUTABLE` | Можно при create, нельзя при update. |
| `IDENTIFIER` | Это resource name (как `name = "projects/foo/orders/bar"`). |
| `UNORDERED_LIST` | Список без значимого порядка. |
| `NON_EMPTY_DEFAULT` | Поле должно иметь дефолтное непустое значение. |

`protoc-gen-openapiv2` уважает эти аннотации:

- `OUTPUT_ONLY` поля не появляются в request schema → клиент не видит их в Swagger.
- `REQUIRED` — становятся обязательными в OpenAPI.
- `IMMUTABLE` — добавляется в описание поля.

Это решает кучу проблем без отдельных External/Internal сообщений. Например,
поле `userId` с `OUTPUT_ONLY` — в схеме клиента его нет, но в proto-сообщении
оно есть для использования внутри сервиса.

## oneof для расширяемых типов

`oneof` — это «одно из» полей. Полезно для:

- Полиморфных типов (один из вариантов: text/number/checkbox).
- Расширяемых дискриминаторов.
- Альтернатив, где набор будет расти.

```protobuf
message Question {
  string id = 1;
  string text = 2;

  reserved 3 to 9;  // запас на будущие основные поля

  oneof options {
    TextOption textOption = 10;
    NumberOption numberOption = 11;
    CheckboxOption checkboxOption = 12;
    RadioOption radioOption = 13;
  }
}
```

В JSON только одно поле из oneof установлено за раз.

### Подводные камни oneof

- **Старый клиент не понимает новый вариант.** Если добавишь `DateOption` —
  клиенты со старой схемой не смогут его прочитать. Это нормально для эволюции,
  но **новые варианты должны быть опциональны** для пользовательского сценария.
- **JSON omitempty.** В Go-generated коде oneof — это интерфейс, который
  сериализуется как `null` при пустом. Внимательно с json.Marshal.

## reserved для эволюции

При удалении поля **никогда не переиспользовать** его номер. Это создаст
несовместимость со старыми клиентами, у которых сериализованные данные с
прежней семантикой.

```protobuf
message Order {
  string id = 1;
  // string oldField = 2;  // удалено в v1.2
  reserved 2;
  reserved "oldField";

  string newField = 3;
}
```

Хорошая привычка — резервировать диапазоны заранее под расширения:

```protobuf
message Question {
  string id = 1;
  string text = 2;

  reserved 3 to 9;   // зарезервировано под будущие основные поля
  reserved 100 to 199;  // зарезервировано под experimental

  oneof options {
    TextOption textOption = 10;
    NumberOption numberOption = 11;
  }
}
```

## Naming полей

Правила:

1. **camelCase в proto.** `firstName`, `dateRange`, `orderId`. proto3 default —
   при JSON-сериализации становится тем же camelCase.
2. **Полное слово, не аббревиатура.** `userIdentifier` лучше `usrId`,
   `timestamp` лучше `ts`. Исключение — общепринятые аббревиатуры (`url`, `id`,
   `uuid`, `iso`, `iata`).
3. **Имя несёт единицу.** `amountMinor`, `priceCents`, `durationMs`, `sizeBytes`,
   `temperatureCelsius`. Без подсказки — клиент гадает.
4. **boolean — без отрицания.** `isActive` лучше, чем `isInactive`. Двойное
   отрицание (`if (!isInactive)`) портит читаемость.
5. **Единый префикс для bool.** `is*` / `has*` / `can*`. Не вперемешку.
6. **Один тип под один концепт.** `days` всегда `int32` (не `int64`), `userId`
   всегда `string`. Не «здесь int64, там int32».

## Дубль типов из google.protobuf

Не изобретайте свои `BoolValue` / `StringValue`:

```protobuf
// плохо
message BoolValue { bool value = 1; }

// хорошо
import "google/protobuf/wrappers.proto";
// и используй google.protobuf.BoolValue
```

Стандартные wrappers решают проблему «значение явно false vs не задано» в proto3
(где scalar fields всегда имеют zero-value).

Альтернатива — `optional` в proto3 (доступно с 3.15):

```protobuf
syntax = "proto3";

message Order {
  optional bool isActive = 1;  // можно различить unset и false
}
```

В сгенерированном Go-коде это будет указатель `*bool` (а не bool), и проверка
`o.IsActive != nil`.

## Что не класть в payload

Cross-cutting concerns (`userId`, `tenantId`, `locale`, `traceId`,
`idempotencyKey`) — это в metadata/headers, не в payload. Подробно — в
[06-cross-cutting-concerns.md](./06-cross-cutting-concerns.md).

Если очень надо оставить в proto-сообщении для удобства работы на сервере —
помечать `OUTPUT_ONLY`, чтобы поле не появилось в публичной схеме.

## Шпаргалка по типам

| Концепт | Тип | Пример |
|---|---|---|
| Глобальный момент | `google.protobuf.Timestamp` | `createdAt` |
| Локальная дата | `google.type.Date` или `string` "YYYY-MM-DD" | `checkInDate` |
| Длительность | `google.protobuf.Duration` | `timeout` |
| Деньги | `Money { currencyCode, amountMinor }` | `total` |
| Опциональный bool | `optional bool` или `google.protobuf.BoolValue` | `isActive` |
| Enum со скрытым default | `_UNSPECIFIED = 0` | `OrderStatus_Unspecified` |
| Полиморфный тип | `oneof` | `payment_method` |
| Изменяемый частично | `FieldMask` | `UpdateRequest` |
| Server-set field | `[(field_behavior) = OUTPUT_ONLY]` | `id`, `createdAt` |

## Связанные документы

- [01-principles.md](./01-principles.md) — аксиомы.
- [03-http-methods.md](./03-http-methods.md) — PATCH + FieldMask.
- [06-cross-cutting-concerns.md](./06-cross-cutting-concerns.md) — что выносить
  в metadata.
- [09-versioning-and-evolution.md](./09-versioning-and-evolution.md) — `reserved`
  и backward-compat.
