# CORS: основы и где настраивать

CORS (Cross-Origin Resource Sharing) — политика браузера, которая контролирует: может ли JS-код на одном origin читать ответ от другого origin.

## Содержание

- [Что такое origin](#что-такое-origin)
- [Что CORS делает и чего не делает](#что-cors-делает-и-чего-не-делает)
- [Simple request vs preflight](#simple-request-vs-preflight)
- [Заголовки ответа сервера](#заголовки-ответа-сервера)
- [Заголовки запроса браузера](#заголовки-запроса-браузера)
- [Где настраивать](#где-настраивать)
- [Частые ошибки](#частые-ошибки)

---

## Что такое origin

`Origin = scheme + host + port`. Всё что отличается — уже разные origins:

```
https://app.example.com       ← frontend
https://api.example.com       ← API       → другой host, другой origin
http://app.example.com        ← то же имя → другой scheme, другой origin
https://app.example.com:8080  ← то же имя → другой порт, другой origin
http://localhost:3000         ← local dev  → другой origin
http://localhost:8080         ← тот же хост, другой порт → другой origin
```

Когда JS на `app.example.com` делает `fetch` к `api.example.com`, браузер проверяет CORS-политику сервера. Если сервер не разрешил — **браузер блокирует чтение ответа**. Запрос физически уходит на сервер, но JS не получает данные.

---

## Что CORS делает и чего не делает

CORS — это механизм **только браузера**. Он не защищает API от:
- `curl` и любых non-browser клиентов
- server-to-server запросов
- атакующего, который пишет код вне браузера

CORS **не заменяет:**
- аутентификацию и авторизацию
- CSRF-защиту (это другая атака — про автоматические cookie)
- rate limiting
- WAF

---

## Simple request vs preflight

**Simple request** — браузер отправляет сразу, без предварительного вопроса. Условия:
- методы: `GET`, `POST`, `HEAD`
- `Content-Type` только: `text/plain`, `application/x-www-form-urlencoded`, `multipart/form-data`
- никаких кастомных заголовков

**Preflight** — браузер **сначала** отправляет `OPTIONS`, спрашивает разрешение, и только после успешного ответа — настоящий запрос:

```
# 1. Браузер спрашивает
OPTIONS /api/users HTTP/1.1
Origin: https://app.example.com
Access-Control-Request-Method: DELETE
Access-Control-Request-Headers: Authorization, Content-Type

# 2. Сервер отвечает
HTTP/1.1 204 No Content
Access-Control-Allow-Origin: https://app.example.com
Access-Control-Allow-Methods: GET, POST, DELETE
Access-Control-Allow-Headers: Authorization, Content-Type
Access-Control-Max-Age: 600

# 3. Браузер отправляет настоящий запрос
DELETE /api/users/123 HTTP/1.1
Origin: https://app.example.com
Authorization: Bearer ...
```

Preflight срабатывает при: `PUT`/`PATCH`/`DELETE`, кастомных заголовках (`Authorization`, `X-Request-ID`), `Content-Type: application/json`.

Если сервер не обрабатывает `OPTIONS` — браузер получает 404/405 и блокирует основной запрос.

---

## Заголовки ответа сервера

### `Access-Control-Allow-Origin`

Самый главный. Говорит браузеру: кому разрешено читать этот ответ.

```
Access-Control-Allow-Origin: https://app.example.com
```
Только этот конкретный origin может прочитать ответ. Остальные — нет.

```
Access-Control-Allow-Origin: *
```
Любой origin может читать ответ. **Нельзя совмещать с `Allow-Credentials: true`** — браузер откажет обоим заголовкам.

```
# Несколько origins — нужно динамически выбирать нужный:
# Сервер проверяет Origin запроса, и если он в allowlist — отвечает конкретным значением
Access-Control-Allow-Origin: https://app.example.com   ← если Origin был этим
Access-Control-Allow-Origin: https://admin.example.com ← если Origin был этим
```
Нельзя написать несколько значений через запятую — браузер не поддерживает. Нужно динамически выбрать один.

Если ответ разный для разных origins — добавить `Vary: Origin` (подсказка кэшам).

---

### `Access-Control-Allow-Credentials`

Разрешает ли браузер передавать и читать credentials в cross-origin запросе.

```
Access-Control-Allow-Credentials: true
```

Под credentials здесь: **cookie**, **HTTP аутентификация** (Basic/Digest), **клиентские TLS-сертификаты**.

Без этого заголовка (или `false`) браузер не отправит cookie к другому origin, даже если `fetch` вызван с `credentials: 'include'`.

**Требование:** если `true`, то `Allow-Origin` обязан быть конкретным, не `*`. Иначе браузер проигнорирует оба.

Типичный случай: SPA на `app.example.com` ходит к API `api.example.com` с session cookie.

---

### `Access-Control-Allow-Methods`

Какие HTTP-методы разрешены в cross-origin запросах.

```
Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS
```

Браузер проверяет это при preflight: если метод из `Access-Control-Request-Method` отсутствует в списке — блокирует запрос.

`GET`, `POST`, `HEAD` не требуют preflight для simple requests, но всё равно стоит явно перечислить все нужные методы.

---

### `Access-Control-Allow-Headers`

Какие заголовки запроса разрешены в cross-origin запросах.

```
Access-Control-Allow-Headers: Content-Type, Authorization, X-Request-Id
```

Браузер проверяет: если клиент хочет отправить `Authorization`, а сервер не включил его в список — preflight провалится.

Стандартные заголовки (`Accept`, `Content-Type` для simple types, `Accept-Language`) разрешены всегда. Всё кастомное (`Authorization`, `X-*`) нужно явно перечислить.

**Антипаттерн:** зеркалить `Access-Control-Request-Headers` обратно — тогда браузер разрешит любой заголовок, policy теряет смысл.

---

### `Access-Control-Expose-Headers`

Какие заголовки ответа браузер **позволит JS-коду прочитать**.

По умолчанию JS видит только: `Cache-Control`, `Content-Language`, `Content-Type`, `Expires`, `Last-Modified`, `Pragma`.

```
Access-Control-Expose-Headers: X-Request-Id, X-Total-Count, X-RateLimit-Remaining
```

Если API возвращает пагинацию через `X-Total-Count`, но этот заголовок не в `Expose-Headers` — frontend JS не сможет его прочитать, даже если он физически пришёл.

---

### `Access-Control-Max-Age`

Сколько секунд браузер кэширует результат preflight и не посылает `OPTIONS` повторно для того же endpoint+method.

```
Access-Control-Max-Age: 600    ← 10 минут — разумный default
Access-Control-Max-Age: 86400  ← 24 часа — долго, изменения policy не применятся быстро
Access-Control-Max-Age: 0      ← кэш отключён, preflight на каждый запрос
```

Браузеры ещё и сами ограничивают максимум: Chrome — 7200 сек, Firefox — 86400 сек.

---

### `Vary`

Не CORS-специфичный, но важный. Говорит кэшам: мой ответ зависит от этих заголовков запроса.

```
Vary: Origin
```

Без него CDN или браузерный кэш может сохранить ответ для `Origin: https://app.example.com` и отдать его запросу с `Origin: https://evil.com`. С `Vary: Origin` кэш понимает что для разных Origins нужны разные записи.

```go
// Добавлять через Add, не Set — чтобы не затереть Vary от других middleware
w.Header().Add("Vary", "Origin")
```

---

## Заголовки запроса браузера

Браузер сам добавляет эти заголовки — разработчик их не устанавливает.

### `Origin`

```
Origin: https://app.example.com
```

Браузер сообщает откуда пришёл запрос. Сервер смотрит на него и решает — разрешить или нет.

Отсутствует в same-origin запросах и в некоторых навигационных запросах.

### `Access-Control-Request-Method` (только в preflight)

```
Access-Control-Request-Method: DELETE
```

Браузер спрашивает: "можно ли мне использовать этот метод?" Сервер должен включить его в `Allow-Methods`.

### `Access-Control-Request-Headers` (только в preflight)

```
Access-Control-Request-Headers: authorization, content-type, x-request-id
```

Браузер перечисляет кастомные заголовки которые хочет отправить. Сервер должен включить их в `Allow-Headers`.

---

## Где настраивать

**На gateway/ingress/proxy** — когда policy единая для всего API:
- один nginx/Traefik/Envoy блок вместо дублирования в каждом сервисе
- проще поддерживать единый allowlist origins

**В приложении** — когда разные routes требуют разных правил:
- публичные endpoints с `Allow-Origin: *` и авторизованные с конкретным origin
- разные `Expose-Headers` для разных частей API

**Не делать оба одновременно** без точного понимания: gateway и middleware могут конфликтовать, выигрывает обычно первый кто ответил.

---

## Частые ошибки

**1. Отражать любой Origin без allowlist + Credentials:**
```go
// Любой сайт получает доступ к cookies пользователя
w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))  // плохо
w.Header().Set("Access-Control-Allow-Credentials", "true")
```

**2. `*` с Credentials:**
```
Access-Control-Allow-Origin: *
Access-Control-Allow-Credentials: true   ← браузер откажет
```

**3. Не обрабатывать `OPTIONS`:**
Если router возвращает 405 на `OPTIONS` — preflight провалится и основной запрос никогда не уйдёт.

**4. `Set` вместо `Add` для `Vary`:**
```go
w.Header().Set("Vary", "Origin")   // затирает Vary от других middleware
w.Header().Add("Vary", "Origin")   // правильно
```

**5. Слишком большой `Max-Age`:** при `86400` изменение allowlist не применится в закэшированных браузерах до следующего дня.
