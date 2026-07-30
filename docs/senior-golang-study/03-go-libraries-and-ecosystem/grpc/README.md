# gRPC in Go

gRPC — бинарный RPC-протокол поверх HTTP/2 с кодогенерацией из `.proto` схем.

Граница с соседним разделом: сам протокол и его семантика (HTTP/2, трейлеры, дедлайны, коды ошибок) разобраны в [08-networking-and-api/protocols](../../08-networking-and-api/protocols/README.md), прежде всего в [06-grpc.md](../../08-networking-and-api/protocols/06-grpc.md). Здесь — чем это писать в Go: библиотеки, кодогенерация, инструменты.

## Материалы

- [01 Protobuf и кодогенерация](./01-protobuf-and-codegen.md) — .proto синтаксис, формат на проводе (varint, зигзаг, теги), присутствие поля и совместимость типов, buf, protoc-gen-go, структура сгенерированного кода
- [02 grpc-go](./02-grpc-go.md) — сервер и клиент, interceptors, streaming, metadata, error handling
- [03 connect-go](./03-connect-go.md) — Connect/gRPC/gRPC-Web на одном порту, browser-friendly RPC
- [04 Сравнение и выбор](./04-comparison.md) — grpc-go vs connect-go, gRPC vs REST, когда что
- [05 grpc-gateway](./05-grpc-gateway.md) — REST proxy поверх gRPC, google.api.http аннотации, OpenAPI генерация

## Вопросы

- чем gRPC лучше REST и когда REST лучше gRPC
- как работает HTTP/2 multiplexing и почему это важно для gRPC streaming
- что такое interceptor и чем он отличается от HTTP middleware
- почему connect-go проще деплоить чем grpc-go без proxy
- как обрабатывать ошибки в gRPC — Status vs стандартные Go errors
- что такое buf и почему protoc один стал неудобен
- когда grpc-gateway, а когда connect-go — в чём принципиальная разница подходов
