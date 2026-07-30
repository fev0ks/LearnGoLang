# 11. Tooling

## Содержание

- [buf — швейцарский нож для proto](#buf--швейцарский-нож-для-proto)
- [protoc-gen-openapiv2](#protoc-gen-openapiv2)
- [protoc-gen-validate (или buf validate)](#protoc-gen-validate-или-buf-validate)
- [CI-правила](#ci-правила)
- [Локальный workflow](#локальный-workflow)
- [Грамотная организация Go-кода вокруг proto](#грамотная-организация-go-кода-вокруг-proto)
- [Документация для клиентов](#документация-для-клиентов)
- [Чек-лист по tooling](#чек-лист-по-tooling)
- [Связанные документы](#связанные-документы)

Хороший дизайн не выживает без инструментов, которые поддерживают дисциплину
автоматически. Здесь — конкретный стек для proto-monorepo + grpc-gateway +
OpenAPI.

---

## buf — швейцарский нож для proto

[buf](https://buf.build/) от Buf Technologies — стандарт de-facto для proto-репозиториев.
Заменяет `protoc` + `protoc-gen-*` + кучу скриптов.

### buf lint

Статический анализ proto. Проверяет правила naming, structure, evolution.

Конфигурация (`buf.yaml` в корне):

```yaml
version: v2
modules:
  - path: proto
lint:
  use:
    - STANDARD
  except:
    - PACKAGE_VERSION_SUFFIX  # если не хочешь "v1" в package name
  enum_zero_value_suffix: _UNSPECIFIED
  service_suffix: Service
```

Запуск:

```bash
buf lint
```

Что ловит:

- enum без `_UNSPECIFIED = 0`
- message-имена не в PascalCase
- field-имена не в lower_snake_case (для proto)
- service без суффикса `Service`
- circular imports
- неиспользуемые imports

### buf breaking

Проверка backward-compatibility. Сравнивает текущее состояние с baseline (main
branch / tag).

```bash
buf breaking --against '.git#branch=main'
```

Что ловит:

- удаление поля без `reserved`
- изменение типа поля
- изменение номера поля
- удаление enum-значения
- переименование rpc / message
- изменение wire-format

В CI:

```yaml
# .github/workflows/proto.yml
- name: buf breaking
  run: buf breaking --against 'https://github.com/example/proto.git#branch=main'
```

Pipeline падает, если PR ломает существующих клиентов.

### buf format

Автоформатирование proto-файлов:

```bash
buf format -w
```

Аналог `gofmt`. Положить в pre-commit hook.

### buf generate

Генерация Go/TS/Python/прочих stubs:

```yaml
# buf.gen.yaml
version: v2
plugins:
  - remote: buf.build/protocolbuffers/go
    out: gen/go
    opt: paths=source_relative
  - remote: buf.build/grpc/go
    out: gen/go
    opt: paths=source_relative,require_unimplemented_servers=false
  - remote: buf.build/grpc-ecosystem/gateway
    out: gen/go
    opt: paths=source_relative
  - remote: buf.build/grpc-ecosystem/openapiv2
    out: gen/openapi
    opt: allow_merge=true,merge_file_name=api
```

Запуск:

```bash
buf generate
```

Генерирует Go-stub'ы и OpenAPI JSON одной командой, без локальной установки
плагинов.

### Custom buf-lint правила

Можно писать свои правила (custom lint plugin). Пример полезных правил:

- Запрет на поля `userId`, `tenantId`, `traceId` в request-message'ах
  (cross-cutting).
- Обязательная аннотация `(google.api.http)` на каждом rpc в `services/external/`.
- Обязательный `(google.api.field_behavior)` на key-полях.
- Запрет на использование `int64` для timestamp'ов (вместо `Timestamp`).

Это серьёзная инвестиция, но окупается на больших командах.

---

## protoc-gen-openapiv2

Генерация OpenAPI v2 (Swagger) из proto + grpc-gateway аннотаций.

```bash
protoc -I. \
  --openapiv2_out=./gen/openapi \
  --openapiv2_opt=logtostderr=true \
  --openapiv2_opt=use_go_templates=true \
  proto/v1/services/external/*.proto
```

Результат — `api.swagger.json`, который можно:

- Хостить как Swagger UI для разработчиков клиентов.
- Импортировать в Postman / Insomnia для тестирования.
- Генерировать клиентские SDK через `openapi-generator-cli`.
- Валидировать API contract в тестах.

`use_go_templates=true` позволяет в комментариях писать описания.

### Аннотации в proto для OpenAPI

```protobuf
import "protoc-gen-openapiv2/options/annotations.proto";

option (grpc.gateway.protoc_gen_openapiv2.options.openapiv2_swagger) = {
  info: {
    title: "Orders API"
    version: "1.0.0"
    description: "Public API for order management"
  }
  schemes: [HTTPS]
  consumes: ["application/json"]
  produces: ["application/json"]
  security_definitions: {
    security: {
      key: "BearerAuth"
      value: {
        type: TYPE_API_KEY
        in: IN_HEADER
        name: "Authorization"
      }
    }
  }
};

service OrderService {
  rpc GetOrder(GetOrderRequest) returns (Order) {
    option (google.api.http) = { get: "/v1/orders/{orderId}" };
    option (grpc.gateway.protoc_gen_openapiv2.options.openapiv2_operation) = {
      summary: "Get order by ID"
      tags: ["Orders"]
      security: { security_requirement: { key: "BearerAuth" value: {} } }
    };
  }
}
```

---

## protoc-gen-validate (или buf validate)

Валидация полей в proto через аннотации, без написания кода:

```protobuf
import "validate/validate.proto";

message CreateOrderRequest {
  string contactEmail = 1 [(validate.rules).string.email = true];
  string phoneNumber = 2 [(validate.rules).string.pattern = "^\\+[0-9]{10,15}$"];
  int64 amountMinor = 3 [(validate.rules).int64.gt = 0];
  repeated string tags = 4 [(validate.rules).repeated = {min_items: 1, max_items: 10}];
}
```

В сгенерированном коде появляется метод `Validate()`:

```go
if err := req.Validate(); err != nil {
    return nil, status.Errorf(codes.InvalidArgument, "%v", err)
}
```

Или через interceptor — автоматическая валидация на входе:

```go
import "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/validator"

grpcServer := grpc.NewServer(
    grpc.UnaryInterceptor(validator.UnaryServerInterceptor()),
)
```

Альтернатива — `buf validate` (новый Buf-stack):

```protobuf
import "buf/validate/validate.proto";

message CreateOrderRequest {
  string contactEmail = 1 [(buf.validate.field).string.email = true];
}
```

---

## CI-правила

Минимальный pipeline для proto-репозитория:

```yaml
name: Proto CI

on: [push, pull_request]

jobs:
  proto:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0   # для buf breaking нужна история

      - uses: bufbuild/buf-setup-action@v1

      - name: Format check
        run: buf format --diff --exit-code

      - name: Lint
        run: buf lint

      - name: Breaking change check
        run: buf breaking --against 'https://github.com/example/proto.git#branch=main'

      - name: Generate
        run: buf generate

      - name: Verify generated code is up-to-date
        run: |
          if [ -n "$(git status --porcelain)" ]; then
            echo "Generated code is out of date. Run 'buf generate' locally."
            git diff
            exit 1
          fi
```

### Дополнительные проверки

- **OpenAPI valid:** `swagger-cli validate api.swagger.json`.
- **No hardcoded URLs in proto:** grep на `localhost`, hardcoded host'ы.
- **No personal info in examples:** grep на email-pattern, phone-pattern в
  комментариях.

---

## Локальный workflow

Для разработчика — pre-commit hook:

```bash
#!/bin/sh
# .git/hooks/pre-commit

buf format -w
buf lint || exit 1
buf generate

# Если генерированный код изменился — добавить в commit
git add gen/
```

Это ловит большинство проблем до push'а.

---

## Грамотная организация Go-кода вокруг proto

### Layout

```
github.com/example/myservice/
  proto/                          # сабмодуль или git submodule с proto-monorepo
  gen/go/                         # сгенерированный код, в .gitignore не кладём!
  internal/
    handlers/
      booking.go                  # http/grpc handlers, тонкие
    service/
      booking.go                  # бизнес-логика, не зависит от proto
    repo/
      booking.go                  # доступ к БД
  cmd/
    server/main.go
```

Бизнес-логика (`internal/service/`) не зависит от proto. Handlers принимают
proto-запрос, конвертируют в domain-объект, вызывают service, конвертируют
ответ в proto. Это позволяет:

- Менять proto без переписывания бизнес-логики.
- Тестировать service unit-тестами без mock'а gRPC.
- Использовать одну service-функцию из gRPC и из CLI/cron.

### Преобразование proto ↔ domain

Часто writeahead — простая функция:

```go
func orderToProto(o *domain.Order) *commonv1.Order {
    return &commonv1.Order{
        Id:           o.ID,
        Status:       orderStatusToProto(o.Status),
        Total:        moneyToProto(o.Total),
        ContactEmail: o.ContactEmail,
        CreatedAt:    timestamppb.New(o.CreatedAt),
    }
}
```

Если этих функций много, применяют кодогенерацию (см. [10-protobuf-repo-layout.md](./10-protobuf-repo-layout.md)).

---

## Документация для клиентов

Минимум что нужно публиковать:

1. **OpenAPI Swagger UI** — auto-generated, всегда актуальный.
2. **README с описанием** — основные принципы, auth, errors, rate limits.
3. **Changelog** — что меняется в каждой версии, что deprecated, когда EOL.
4. **Примеры** — curl, Go-client, Postman collection.

Платформы для хостинга API docs:

- **buf.build:** хостинг proto-репозитория + auto-generated docs.
- **Redocly / Stoplight:** для OpenAPI.
- **Postman:** auto-generated docs из collection.
- **Свой Swagger UI:** простой `docker run swaggerapi/swagger-ui`.

---

## Чек-лист по tooling

- [ ] `buf.yaml` с lint-правилами в корне.
- [ ] `buf.gen.yaml` для генерации Go/OpenAPI.
- [ ] `buf lint` + `buf breaking` в CI.
- [ ] `buf format` в pre-commit hook.
- [ ] OpenAPI генерится автоматически и публикуется.
- [ ] `protoc-gen-validate` или `buf validate` на required-полях.
- [ ] Custom lint-правило: запрет на `userId`/`tenantId` в request'ах.
- [ ] CI проверяет, что generated code актуален.

---

## Связанные документы

- [10-protobuf-repo-layout.md](./10-protobuf-repo-layout.md) — структура репо.
- [05-payloads-and-types.md](./05-payloads-and-types.md) — `field_behavior` для
  OpenAPI.
- [13-references.md](./13-references.md) — buf docs, openapi-generator.
- [../protocols/06-grpc.md](../protocols/06-grpc.md) — gRPC основы.
- [../protocols/08-openapi-and-swagger.md](../protocols/08-openapi-and-swagger.md) —
  OpenAPI глубже.
