# Backend Application And Data Access

После edge слоя запрос наконец доходит до application backend.

## 1. Запрос попадает в web server

На стороне backend сначала работает HTTP server:
- принимает соединение;
- читает request line и headers;
- парсит тело запроса, если оно есть;
- передает управление router или middleware chain.

В Go это обычно:
- `net/http`;
- router поверх него;
- middleware для auth, logging, tracing, metrics.

## 2. Middleware chain

До бизнес-логики часто выполняются:
- request logging;
- trace context extraction;
- authentication;
- authorization;
- rate limiting;
- body size limit;
- timeout wrapping;
- panic recovery.

То есть handler часто не первая точка обработки.

## 3. Business logic

Потом запрос попадает в application handler или service layer:
- валидируются входные данные;
- строится use case;
- идут обращения к внутренним сервисам;
- вызывается repository layer;
- собирается response model.

Здесь часто рождается основная application latency:
- медленный SQL;
- поход в Redis;
- вызов внешнего API;
- fan-out в несколько сервисов.

## 4. Доступ к данным

В типичном backend запрос может сходить в:
- Postgres;
- Redis;
- Kafka или другой broker;
- object storage;
- другой internal service по HTTP или gRPC.

Каждый hop добавляет:
- сетевую задержку;
- сериализацию;
- шанс timeout или partial failure.

## 5. Формирование ответа

Backend определяет:
- status code;
- headers;
- body;
- cache headers;
- cookies;
- content type.

Для HTML-страницы ответ может быть:
- server-side rendered HTML;
- redirect;
- JSON bootstrap data;
- или минимальный shell, который потом догружает frontend assets.

## Где здесь бывают проблемы

- slow database query;
- N+1 calls;
- block на connection pool;
- timeout до downstream;
- serialization overhead;
- oversized response body;
- ошибка только на одном из внутренних hops.

## Почему это только часть маршрута

Backend часто считают "главным местом", но по факту:
- до него уже были browser, DNS, TCP, TLS и edge;
- после него еще будет обратный путь, кеширование и render в браузере.

## Что могут спросить на интервью

- где обычно ставят middleware;
- как считать end-to-end latency, а не только время handler;
- где правильно делать timeout и retry;
- почему downstream fan-out быстро убивает latency budget;
- как correlation id и tracing помогают разбирать путь запроса.
