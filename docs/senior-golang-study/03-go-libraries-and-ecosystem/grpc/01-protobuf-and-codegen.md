# Protobuf и кодогенерация

Protocol Buffers (protobuf) — язык описания схем и бинарный формат сериализации. gRPC использует protobuf как IDL (Interface Definition Language) и формат передачи данных.

## Содержание

- [Зачем protobuf](#зачем-protobuf)
- [.proto синтаксис](#proto-синтаксис)
- [gRPC сервисы в .proto](#grpc-сервисы-в-proto)
- [Кодогенерация: protoc vs buf](#кодогенерация-protoc-vs-buf)
- [buf: установка и конфигурация](#buf-установка-и-конфигурация)
- [Структура сгенерированного кода](#структура-сгенерированного-кода)
- [buf lint и breaking change detection](#buf-lint-и-breaking-change-detection)

---

## Зачем protobuf

| | JSON | protobuf |
|---|---|---|
| Формат | текст | бинарный |
| Размер | baseline | 3-10x меньше |
| Парсинг | медленнее | быстрее |
| Схема | опциональная (JSON Schema) | обязательная (.proto) |
| Читаемость | человекочитаем | нечитаем без схемы |
| Кодогенерация | нет | да (типобезопасные клиент/сервер) |
| Эволюция схемы | вручную | правила совместимости в spec |

Главное преимущество protobuf в gRPC-контексте — **кодогенерация**: из `.proto` файла генерируются типобезопасные server stub и client stub на Go, Java, Python, TypeScript и других языках. Контракт API живёт в одном месте.

---

## .proto синтаксис

```protobuf
syntax = "proto3";

package user.v1;

option go_package = "github.com/myorg/myrepo/gen/user/v1;userv1";

import "google/protobuf/timestamp.proto";

// Сообщения
message User {
  string id    = 1;
  string name  = 2;
  string email = 3;
  google.protobuf.Timestamp created_at = 4;

  // Enum
  Role role = 5;
}

enum Role {
  ROLE_UNSPECIFIED = 0;  // zero value всегда UNSPECIFIED
  ROLE_ADMIN       = 1;
  ROLE_USER        = 2;
  ROLE_VIEWER      = 3;
}

message CreateUserRequest {
  string name  = 1;
  string email = 2;
  Role   role  = 3;
}

message CreateUserResponse {
  User user = 1;
}

message GetUserRequest {
  string id = 1;
}

message GetUserResponse {
  User user = 1;
}

message ListUsersRequest {
  int32  page_size   = 1;
  string page_token  = 2;
}

message ListUsersResponse {
  repeated User users         = 1;
  string        next_page_token = 2;
}
```

**Правила нумерации полей:**
- Числа 1-15 кодируются в 1 байт (используй для частых полей)
- Числа 16-2047 — 2 байта
- Никогда не переиспользуй удалённые номера — используй `reserved`

```protobuf
message User {
  reserved 6, 7;              // зарезервировать номера удалённых полей
  reserved "old_field_name";  // зарезервировать имена
  string id   = 1;
  string name = 2;
}
```

**Типы данных:**

| proto3 | Go |
|---|---|
| `string` | `string` |
| `int32`, `int64` | `int32`, `int64` |
| `uint32`, `uint64` | `uint32`, `uint64` |
| `float`, `double` | `float32`, `float64` |
| `bool` | `bool` |
| `bytes` | `[]byte` |
| `repeated T` | `[]T` |
| `map<K, V>` | `map[K]V` |
| `google.protobuf.Timestamp` | `*timestamppb.Timestamp` |
| `google.protobuf.Duration` | `*durationpb.Duration` |

---

## gRPC сервисы в .proto

```protobuf
service UserService {
  // Unary — один запрос, один ответ
  rpc GetUser(GetUserRequest) returns (GetUserResponse);
  rpc CreateUser(CreateUserRequest) returns (CreateUserResponse);

  // Server streaming — один запрос, поток ответов
  rpc WatchUser(WatchUserRequest) returns (stream UserEvent);

  // Client streaming — поток запросов, один ответ
  rpc BatchCreateUsers(stream CreateUserRequest) returns (BatchCreateUsersResponse);

  // Bidirectional streaming — поток запросов, поток ответов
  rpc SyncUsers(stream SyncRequest) returns (stream SyncResponse);
}
```

---

## Кодогенерация: protoc vs buf

### protoc (старый способ)

`protoc` — оригинальный компилятор от Google. Требует:
1. Установить `protoc` бинарник
2. Установить плагины: `protoc-gen-go`, `protoc-gen-go-grpc`
3. Управлять путями к импортам вручную
4. Запускать с длинными флагами

```bash
protoc \
  --proto_path=. \
  --proto_path=third_party \
  --go_out=. \
  --go_opt=paths=source_relative \
  --go-grpc_out=. \
  --go-grpc_opt=paths=source_relative \
  proto/user/v1/user.proto
```

Неудобно: много флагов, сложно воспроизвести, нет linting и breaking change detection.

### buf (современный способ)

`buf` — полноценный инструмент для работы с protobuf:

```bash
brew install bufbuild/buf/buf
# или
go install github.com/bufbuild/buf/cmd/buf@latest
```

---

## buf: установка и конфигурация

Структура проекта:
```
proto/
  buf.yaml          # конфиг модуля
  buf.gen.yaml      # конфиг кодогенерации
  user/
    v1/
      user.proto
      user_service.proto
gen/                # сгенерированный код (в .gitignore или нет — по договорённости)
  user/
    v1/
      user.pb.go
      user_grpc.pb.go
```

`buf.yaml`:
```yaml
version: v2
name: buf.build/myorg/myrepo   # если публикуешь в Buf Schema Registry

deps:
  - buf.build/googleapis/googleapis   # для google.protobuf.Timestamp и др.

lint:
  use:
    - DEFAULT

breaking:
  use:
    - FILE
```

`buf.gen.yaml`:
```yaml
version: v2
managed:
  enabled: true
  override:
    - file_option: go_package_prefix
      value: github.com/myorg/myrepo/gen

plugins:
  - remote: buf.build/protocolbuffers/go      # protoc-gen-go
    out: gen
    opt:
      - paths=source_relative

  - remote: buf.build/grpc/go                 # protoc-gen-go-grpc
    out: gen
    opt:
      - paths=source_relative
```

Генерация:
```bash
cd proto
buf generate          # генерирует в gen/
buf generate --watch  # следит за изменениями
```

Обновление зависимостей:
```bash
buf dep update
```

---

## Структура сгенерированного кода

Из одного `.proto` файла `buf` генерирует два `.go` файла:

**`user.pb.go`** — типы сообщений:
```go
// Сгенерировано protoc-gen-go
type User struct {
    Id        string                 `protobuf:"bytes,1,opt,name=id"`
    Name      string                 `protobuf:"bytes,2,opt,name=name"`
    Email     string                 `protobuf:"bytes,3,opt,name=email"`
    CreatedAt *timestamppb.Timestamp `protobuf:"bytes,4,opt,name=created_at"`
    Role      Role                   `protobuf:"varint,5,opt,name=role,enum=user.v1.Role"`
    // скрытые поля для protobuf internals
}

// Методы: Reset, String, ProtoMessage, ProtoReflect, GetId, GetName, ...
```

**`user_grpc.pb.go`** — server и client интерфейсы:
```go
// Сгенерировано protoc-gen-go-grpc

// Client
type UserServiceClient interface {
    GetUser(ctx context.Context, in *GetUserRequest, opts ...grpc.CallOption) (*GetUserResponse, error)
    CreateUser(ctx context.Context, in *CreateUserRequest, opts ...grpc.CallOption) (*CreateUserResponse, error)
    WatchUser(ctx context.Context, in *WatchUserRequest, opts ...grpc.CallOption) (UserService_WatchUserClient, error)
}

// Server (нужно реализовать)
type UserServiceServer interface {
    GetUser(context.Context, *GetUserRequest) (*GetUserResponse, error)
    CreateUser(context.Context, *CreateUserRequest) (*CreateUserResponse, error)
    WatchUser(*WatchUserRequest, UserService_WatchUserServer) error
    mustEmbedUnimplementedUserServiceServer()
}

// Embed для forward-compatibility
type UnimplementedUserServiceServer struct{}
func (UnimplementedUserServiceServer) GetUser(context.Context, *GetUserRequest) (*GetUserResponse, error) {
    return nil, status.Errorf(codes.Unimplemented, "method GetUser not implemented")
}
```

---

## buf lint и breaking change detection

```bash
# Проверить .proto файлы на соответствие стайлгайду
buf lint

# Примеры ошибок lint:
# user.proto:5:1:Package name "user" should be suffixed with a correctly formed version, such as "user.v1".
# user.proto:15:3:Field name "UserName" should be lower_snake_case, such as "user_name".
# user.proto:20:1:Enum value name "ADMIN" should be prefixed with "ROLE_".
```

```bash
# Проверить что изменения не ломают обратную совместимость
buf breaking --against .git#branch=main

# Примеры breaking changes:
# Field "1" on message "User" changed type from "string" to "int32".
# RPC "GetUser" on service "UserService" had request type changed.
# Enum value "ROLE_ADMIN" on enum "Role" changed number from "1" to "5".
```

Breaking change detection — критично при публичных API: если клиент скомпилирован со старой схемой, удаление поля или изменение типа сломает десериализацию.

**Безопасные изменения** (не breaking):
- Добавить новое поле с новым номером
- Добавить новый RPC метод
- Добавить новое значение enum

**Breaking изменения**:
- Удалить или переименовать поле
- Изменить тип поля
- Изменить номер поля
- Изменить имя RPC метода
