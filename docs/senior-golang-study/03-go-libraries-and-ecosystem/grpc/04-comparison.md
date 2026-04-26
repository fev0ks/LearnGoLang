# Сравнение и выбор: gRPC vs REST, grpc-go vs connect-go

## Три варианта дать REST-доступ к gRPC

| | grpc-gateway | connect-go | grpc-go + Envoy |
|---|---|---|---|
| Подход | сгенерированный reverse-proxy | один сервер, 3 протокола | внешний proxy |
| URL маппинг | произвольный (аннотации в .proto) | фиксированный | произвольный (Envoy config) |
| OpenAPI | да (protoc-gen-openapiv2) | нет | нет |
| Browser TypeScript client | curl/fetch, кастомные URL | connect-es, codegen | grpc-web клиент |
| Инфраструктура | +1 процесс или порт | один процесс | +Envoy sidecar |
| Сложность | средняя | низкая | высокая |

Подробнее: [05-grpc-gateway.md](./05-grpc-gateway.md)

---

## grpc-go vs connect-go

| | grpc-go | connect-go |
|---|---|---|
| HTTP engine | собственный HTTP/2 | стандартный `net/http` |
| Браузер без proxy | нет | да (Connect protocol) |
| net/http middleware | нет напрямую | да |
| TLS | через `grpc.Creds` | стандартный `tls.Config` |
| Протоколы | gRPC | Connect + gRPC + gRPC-Web |
| TypeScript клиент | grpc-web + proxy | connect-es напрямую |
| Рефлексия | `grpc/reflection` | `connectrpc/grpcreflect` |
| Зрелость/adoption | высокая | средняя (2022+) |
| Совместимость с grpc-go клиентами | да | да |
| Размер зависимостей | больше | меньше |

### Когда grpc-go

- Команда уже знает grpc-go
- Только server-to-server (браузер не нужен)
- Нужна максимальная совместимость с gRPC экосистемой (Istio, Envoy, gRPC-LB)
- Уже есть Envoy proxy в инфраструктуре

### Когда connect-go

- Нужен браузерный клиент без Envoy
- Хочешь использовать стандартные net/http middleware (OpenTelemetry, OAuth2, etc.)
- Хочешь один сервис отвечать и на gRPC, и на Connect/JSON
- Новый проект без legacy grpc-go кода

---

## gRPC vs REST

| | REST/JSON | gRPC |
|---|---|---|
| Протокол | HTTP/1.1 или HTTP/2 | HTTP/2 |
| Формат | JSON (текст) | protobuf (бинарный) |
| Схема | опциональная | обязательная (.proto) |
| Кодогенерация | нет (OpenAPI → но отдельный инструмент) | из коробки |
| Браузерная поддержка | да (нативно) | только с proxy или connect-go |
| Streaming | SSE или WebSocket | нативный (4 вида) |
| Типизация | слабая (JSON) | строгая |
| Debugging | curl, Postman | grpcurl, grpcui |
| Читаемость трафика | да | нет без схемы |
| Версионирование | URI (/v1/, /v2/) | breaking change detection в buf |

### Когда gRPC

- **Internal service-to-service**: мобильное приложение, браузер не вызывают сервис напрямую
- **Строгий контракт важен**: разные команды или языки, нужна кодогенерация
- **Streaming**: real-time события, bidirectional потоки
- **Performance-критичные сервисы**: меньший размер payload, быстрее парсинг
- **Polyglot environment**: Go сервис общается с Python ML-сервисом и Java billing

### Когда REST

- **Public API**: клиенты неизвестны заранее (браузеры, мобильные приложения, curl-щики)
- **Команда не знает protobuf** и нет ресурса на обучение
- **Простой CRUD**: нет streaming, нет строгих perf требований
- **Debugging важен**: curl в руках разработчика, логи читаемы

### Гибридный подход

Многие production системы используют оба:

```
браузер/mobile → REST API Gateway → gRPC internal services
```

- Public-facing слой: REST (удобство клиентов, OpenAPI документация)
- Internal: gRPC (строгий контракт, performance, streaming)

---

## Вопросы на интервью

**"Когда ты выбрал бы gRPC вместо REST?"**

Хороший ответ:
1. Internal service-to-service, где нет прямых браузерных клиентов
2. Нужен строгий типизированный контракт — кодогенерация гарантирует совместимость
3. Streaming: gRPC нативно поддерживает server/client/bidirectional streaming
4. Polyglot — один .proto, клиенты на любом языке

**"Что такое connect-go и зачем он нужен?"**

Ключевые моменты:
- Поддерживает три протокола (Connect/gRPC/gRPC-Web) на одном порту
- Запускается как обычный `net/http` handler — можно использовать любые middleware
- Браузер может вызывать сервис напрямую через Connect protocol без Envoy proxy
- Совместим с grpc-go клиентами

**"Что такое protobuf breaking change?"**

- Изменение типа или номера существующего поля ломает десериализацию у клиентов со старой схемой
- `buf breaking` автоматически находит такие изменения в CI
- Добавление нового поля безопасно — старые клиенты просто игнорируют его

**"Как работает gRPC streaming?"**

- HTTP/2 позволяет несколько потоков данных в одном TCP соединении
- Server streaming: сервер шлёт несколько сообщений до закрытия stream — подходит для real-time событий
- Bidirectional: оба конца шлют поток независимо — подходит для chat, sync протоколов

---

## Рекомендация по умолчанию

```
Public API, браузерные клиенты     → REST
Internal service-to-server, Go    → gRPC
  ├── Нужен браузерный клиент      → connect-go
  ├── Команда знает grpc-go        → grpc-go
  └── Новый проект, net/http stack → connect-go
Streaming real-time events         → gRPC (любая реализация)
Polyglot (Go + Python + Java)      → gRPC + protobuf
```
