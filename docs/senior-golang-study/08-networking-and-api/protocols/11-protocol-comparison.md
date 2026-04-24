# API Protocols: Сравнение

Итоговая таблица для быстрого принятия решения. Читать после изучения каждого протокола.

---

## Большая таблица

| | REST | gRPC | GraphQL | WebSocket | Webhooks | WebRTC | SOAP |
|---|---|---|---|---|---|---|---|
| **Transport** | HTTP/1.1–2 | HTTP/2 | HTTP/1.1–2 | TCP (WS) | HTTP/1.1–2 | UDP/DTLS | HTTP/SMTP |
| **Формат** | JSON/XML | Protobuf | JSON | любой | JSON | binary | XML |
| **Streaming** | SSE/polling | Встроено (4 типа) | Subscriptions | ✅ full-duplex | ❌ | ✅ P2P | ❌ |
| **Browser** | ✅ нативно | ❌ (grpc-web) | ✅ | ✅ | ✅ (server-side) | ✅ | ✅ |
| **Типобезопасность** | OpenAPI opt. | ✅ Protobuf | ✅ Schema | ❌ | ❌ | ❌ | ✅ WSDL |
| **Caching** | ✅ HTTP cache | ❌ | ❌ (POST) | ❌ | ❌ | ❌ | ❌ |
| **Human-readable** | ✅ JSON | ❌ binary | ✅ JSON | зависит | ✅ JSON | ❌ | ⚠️ verbose XML |
| **Инфраструктура** | Минимум | Минимум | Минимум | Минимум | Минимум | STUN/TURN | Минимум |
| **Сложность** | Низкая | Средняя | Высокая | Средняя | Низкая | Высокая | Высокая |
| **Performance** | Средний | Высокий | Средний | Высокий | Средний | P2P высокий | Низкий |
| **Legacy** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |

---

## Decision tree: какой протокол выбрать

```
Публичный API для сторонних разработчиков?
  → REST (знаком всем, curl-friendly, HTTP caching)

Сервис-сервис внутри системы?
  → gRPC (типобезопасно, быстро, streaming из коробки)

Несколько клиентов с разными нуждами (mobile/web/TV)?
  → GraphQL (клиент выбирает поля)

Real-time двусторонняя связь (чат, collaboration)?
  → WebSocket

Уведомление твоего сервиса о событиях в чужом сервисе?
  → Webhooks (GitHub, Stripe, Twilio)

Peer-to-peer видео/аудио (conferencing, live stream)?
  → WebRTC

Интеграция с legacy enterprise (банки, SAP, старый SOAP API)?
  → SOAP (вынужденно)
```

---

## Детализированные trade-offs

### REST

**Сильные стороны:**
- Универсально понятен — любой знает HTTP методы и статус-коды
- HTTP caching (CDN, браузер) — GET запросы кешируются
- curl-friendly, Postman, Bruno — простая отладка
- Stateless — горизонтально масштабируется легко

**Слабые стороны:**
- Over-fetching (вернул 50 полей, нужно 3)
- Under-fetching (нужно 3 запроса вместо одного)
- Нет строгого контракта без OpenAPI (и тот — опционально)
- Streaming — через SSE или WebSocket (отдельные протоколы)

### gRPC

**Сильные стороны:**
- Protobuf — строгий контракт, binary (3–10× меньше JSON), codegen
- HTTP/2 — multiplexing, header compression
- 4 типа RPC включая bidirectional streaming
- Interceptors — middleware для auth/logging/retry

**Слабые стороны:**
- Browser без grpc-web proxy
- Сложнее отлаживать (binary payload)
- Требует Proto и toolchain
- Reflection нужен для динамических clients

### GraphQL

**Сильные стороны:**
- Клиент запрашивает только нужные поля
- Один endpoint для всего
- Schema = автодокументация через introspection
- Эффективен для сложных данных-графов

**Слабые стороны:**
- N+1 без DataLoader
- HTTP POST = нет HTTP caching
- Introspection = утечка schema (отключать в prod)
- File upload сложный
- Complexity scoring и depth limiting нужны для защиты от DoS

### WebSocket

**Сильные стороны:**
- Full-duplex, постоянное соединение
- Низкая latency (нет HTTP overhead)
- Browser native

**Слабые стороны:**
- Stateful соединение — сложнее масштабировать (нужен backplane)
- Прокси/firewall могут рвать долгие соединения
- Нет встроенного retry / reconnect (вручную)

### Webhooks

**Сильные стороны:**
- Простота — просто HTTP POST
- Push семантика — нет polling
- At-least-once через retry

**Слабые стороны:**
- Твой endpoint должен быть публично доступен
- Нет гарантированного ordering
- Debug сложнее (не ты инициатор)

---

## Смешанные архитектуры

В реальных системах протоколы комбинируются:

### API Gateway паттерн

```
Mobile App ──REST──► API Gateway ──gRPC──► User Service
                                  ──gRPC──► Order Service
                                  ──gRPC──► Payment Service
```

- Клиенты используют REST (знакомо, HTTP caching)
- Внутри — gRPC (высокая производительность, типобезопасность)
- API Gateway транслирует

### BFF (Backend For Frontend)

```
Web App ──GraphQL──► BFF ──gRPC──► Services
Mobile  ──REST────► BFF ──gRPC──► Services
```

### Event-driven + Synchronous

```
User request ──REST──► Service A
                            │
                    writes to Kafka
                            │
              Service B reads ──WebSocket──► Client (real-time update)
```

---

## Выбор по типу взаимодействия

```
Тип взаимодействия:

Клиент → Сервер (request/response):
  → REST (public API) или gRPC (internal)

Сервер → Клиент (push):
  → WebSocket (real-time) или SSE (server-only stream) или Webhooks (event-driven)

Сервис → Сервис:
  → gRPC (sync) или Message Broker (async)

P2P медиа:
  → WebRTC

Legacy:
  → SOAP (если вынужден)
```

---

## Interview-ready answer

**Q: REST vs gRPC — когда что?**

REST — для публичных API, когда важен browser support, HTTP caching, простота отладки (curl). gRPC — для service-to-service внутри системы: строгий контракт (Protobuf), binary serialization (быстрее JSON), HTTP/2 multiplexing, встроенный streaming. Часто используют оба: REST-gateway для клиентов, gRPC внутри.

**Q: Зачем нужен GraphQL если есть REST?**

GraphQL решает конкретную проблему: несколько типов клиентов с разными потребностями в данных. Мобильный нужен 3 поля, десктоп — 20, TV — другие 5. С REST — либо separate endpoints, либо over-fetching. GraphQL позволяет одному endpoint обслуживать всех клиентов, которые сами определяют форму ответа. Цена: N+1, нет HTTP caching, сложность.

**Q: WebSocket vs REST с polling?**

Polling создаёт unnecessary requests (большинство — пустые), высокую latency (ждёшь следующий interval), нагрузку на server. WebSocket — постоянное соединение, сервер пушит когда есть данные, ~zero latency. Минус WebSocket — stateful (сложнее масштабировать, нужен backplane для multi-instance).
