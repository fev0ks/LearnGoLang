# CORS: основы и где настраивать

CORS (Cross-Origin Resource Sharing) — политика браузера, которая контролирует может ли JS-код на одном origin читать ответ от другого origin.

## Содержание

- [Что такое origin и зачем CORS](#что-такое-origin-и-зачем-cors)
- [Что CORS делает и чего не делает](#что-cors-делает-и-чего-не-делает)
- [Simple request vs preflight](#simple-request-vs-preflight)
- [Основные заголовки](#основные-заголовки)
- [Где настраивать](#где-настраивать)
- [Частые ошибки](#частые-ошибки)

---

## Что такое origin и зачем CORS

`Origin = scheme + host + port`. Всё что отличается — разные origins:

```
https://app.example.com       ← frontend
https://api.example.com       ← API (другой origin!)
http://localhost:3000         ← local dev
```

Когда JS на `app.example.com` делает fetch к `api.example.com`, браузер проверяет CORS-политику сервера. Если сервер не разрешил — браузер **блокирует чтение ответа** (запрос физически уходит, но JS не получает данные).

---

## Что CORS делает и чего не делает

CORS — это **browser-only** механизм. Он не защищает API от:
- `curl` и любых non-browser клиентов
- server-to-server запросов
- атак при которых код выполняется не в браузере

CORS **не заменяет:**
- аутентификацию и авторизацию
- CSRF-защиту (это другая атака)
- rate limiting
- WAF

---

## Simple request vs preflight

**Simple request** — идёт сразу, без предварительной проверки:
- методы: GET, POST, HEAD
- заголовки: только стандартные (Content-Type: text/plain, application/x-www-form-urlencoded, multipart/form-data)

**Preflight** — браузер сначала отправляет `OPTIONS` чтобы спросить разрешение, затем настоящий запрос:

```
OPTIONS /api/users
Origin: https://app.example.com
Access-Control-Request-Method: DELETE
Access-Control-Request-Headers: Authorization, Content-Type
```

Preflight срабатывает при: `PUT/PATCH/DELETE`, кастомных заголовках (`Authorization`, `X-Request-ID`), `Content-Type: application/json`.

Сервер **обязан ответить** на `OPTIONS`, иначе браузер заблокирует основной запрос.

---

## Основные заголовки

**Ответ сервера:**

| Заголовок | Назначение |
|---|---|
| `Access-Control-Allow-Origin` | Какой origin может читать ответ. Конкретный или `*` |
| `Access-Control-Allow-Methods` | Разрешённые методы |
| `Access-Control-Allow-Headers` | Разрешённые заголовки запроса |
| `Access-Control-Allow-Credentials` | Разрешены ли cookies/session |
| `Access-Control-Expose-Headers` | Какие response-заголовки видны JS (`X-Total-Count` и т.д.) |
| `Access-Control-Max-Age` | Как долго браузер кэширует preflight (секунды) |
| `Vary: Origin` | Подсказка кэшам что ответ зависит от Origin |

**Критичная комбинация:** `Allow-Credentials: true` требует **конкретного** origin, не `*`. Браузер откажет:
```
Access-Control-Allow-Origin: *
Access-Control-Allow-Credentials: true  ← браузер проигнорирует оба заголовка
```

---

## Где настраивать

**На gateway/ingress/proxy** — когда policy единая для всего API:
- меньше дублирования
- проще поддерживать

**В приложении** — когда разные routes требуют разных правил:
- публичные и приватные endpoints
- разные allowed origins для разных частей API

**Избегать:** дублировать одинаковую CORS-логику в каждом сервисе — это источник расхождений.

---

## Частые ошибки

**1. Отражать любой Origin без проверки:**
```go
// Плохо — любой сайт получает доступ
w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
w.Header().Set("Access-Control-Allow-Credentials", "true")
```

**2. Не обрабатывать `OPTIONS`:**
```go
// Если router не знает про OPTIONS — preflight вернёт 404/405
// и браузер заблокирует основной запрос
mux.HandleFunc("OPTIONS /api/", handlePreflight)
```

**3. Использовать `Set` вместо `Add` для `Vary`:**
```go
// Set перезапишет Vary поставленный другим middleware
w.Header().Set("Vary", "Origin")         // плохо — затирает
w.Header().Add("Vary", "Origin")         // хорошо — добавляет
```

**4. Слишком большой `Max-Age`:** при `86400` (24ч) изменение CORS-политики не применится в браузерах до истечения кэша. Разумный default — `600` (10 мин).
