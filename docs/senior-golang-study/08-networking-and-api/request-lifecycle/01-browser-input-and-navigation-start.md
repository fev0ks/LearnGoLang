# Ввод в браузере и старт навигации

## Содержание

- [Omnibox: URL vs поисковый запрос](#omnibox-url-vs-поисковый-запрос)
- [Разбор URL](#разбор-url)
- [Что браузер проверяет до сети](#что-браузер-проверяет-до-сети)
- [HSTS: принудительный HTTPS](#hsts-принудительный-https)
- [Service Worker: перехват запроса](#service-worker-перехват-запроса)
- [HTTP cache: можно ли обойтись без сети](#http-cache-можно-ли-обойтись-без-сети)
- [Resource hints: preconnect и prefetch](#resource-hints-preconnect-и-prefetch)
- [Navigation Timing API: что браузер измеряет](#navigation-timing-api-что-браузер-измеряет)
- [Interview-ready answer](#interview-ready-answer)

Когда пользователь вводит `google.com`, цепочка начинается не с backend и даже не с DNS, а с браузера. Множество решений принимается до первого сетевого пакета.

---

## Omnibox: URL vs поисковый запрос

Современный браузер использует omnibox (адресная строка = поисковая строка):
- Если строка похожа на hostname или URL — браузер пробует навигацию.
- Если нет — отправляет как search query в поисковик по умолчанию.

Эвристики распознавания:
- `google.com` → навигация (hostname без пробелов с TLD).
- `golang scheduler` → поиск.
- `localhost:8080` → навигация (localhost).
- `go` → зависит от настроек (может быть поиском или navigation к `go` домену).

После распознавания как URL браузер нормализует:
```text
google.com  →  https://google.com/
```

Причина: большинство публичных сайтов ожидают HTTPS. Браузер пробует HTTPS первым. Если не работает — fallback на HTTP (если нет HSTS).

---

## Разбор URL

Браузер парсит URL по компонентам:
```text
https://user:pass@example.com:443/path/to/page?key=val#section
  │      │         │           │   │            │       └─ fragment (не идёт на сервер)
  │      │         │           │   │            └─ query string
  │      │         │           │   └─ path
  │      │         │           └─ port (443 = default для https, опускается)
  │      │         └─ host
  │      └─ credentials (редко, небезопасно)
  └─ scheme
```

Fragment (`#section`) — никогда не уходит на сервер. Это исключительно браузерный механизм для якорной навигации. Но Single-Page Applications используют `hash routing` (`#/profile`) на клиенте.

URL encoding: пробелы → `%20`, спецсимволы → percent-encoded. Браузер делает это автоматически перед отправкой.

---

## Что браузер проверяет до сети

Прежде чем сделать сетевой запрос, браузер проходит несколько проверок:

```text
URL введён
    │
    ▼
HSTS cache → если домен в HSTS — сразу HTTPS, без попытки HTTP
    │
    ▼
HTTP cache → есть свежий ответ? → отдать немедленно
    │
    ▼
Service Worker → зарегистрирован? → передать запрос SW
    │  SW может ответить из cache без сети
    ▼
Открытое соединение → есть HTTP/2 stream к хосту? → reuse
    │
    ▼
DNS lookup (следующий этап)
```

---

## HSTS: принудительный HTTPS

HSTS (HTTP Strict Transport Security) — браузер помнит, что домен всегда должен открываться по HTTPS, даже если пользователь ввёл `http://`.

Сервер устанавливает политику через заголовок:
```http
Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
```

- `max-age=31536000` — браузер запомнит на 1 год.
- `includeSubDomains` — политика распространяется на все поддомены.
- `preload` — домен можно добавить в [HSTS Preload List](https://hstspreload.org/) — встроенный список браузера.

**HSTS Preload**: Chrome, Firefox, Safari, Edge имеют встроенный список доменов, которые всегда открываются по HTTPS — **до первого подключения**. Без preload HSTS действует только после первого успешного HTTPS-ответа (уязвимо к MITM на первом запросе).

Удалить домен из preload list — долго (месяцы), поэтому добавлять его туда стоит только при уверенности в постоянной поддержке HTTPS.

---

## Service Worker: перехват запроса

Service Worker (SW) — это JavaScript, работающий в фоне, отдельно от страницы. Может перехватывать все `fetch` запросы и отвечать из cache.

```javascript
// sw.js: перехват navigation запроса
self.addEventListener('fetch', event => {
    if (event.request.mode === 'navigate') {
        event.respondWith(
            caches.match(event.request)
                .then(cached => cached || fetch(event.request))
        );
    }
});
```

Последствия для backend инженера:
- SW может вернуть кэшированный HTML **без сети** — TTFB = 0 мс.
- Новый деплой backend не попадёт к пользователям, пока SW не обновится.
- SW управляет своим cycle (install → activate → fetch). Обновление SW требует закрытия всех вкладок.

Если SW заглючил и закэшировал сломанную страницу — пользователи видят ошибку даже при рабочем backend. Инструмент лечения: "Unregister Service Worker" в DevTools.

---

## HTTP cache: можно ли обойтись без сети

Браузер имеет HTTP cache (disk cache + memory cache). При наличии свежей записи (max-age не истёк) — запрос в сеть не делается вообще.

Три варианта:
1. **Fresh**: `max-age` не истёк → отдать из cache, 200 (from cache). Сеть = 0.
2. **Stale + ETag**: `max-age` истёк, но есть ETag → conditional GET с `If-None-Match` → `304 Not Modified` (тело из cache). Сеть = 1 RTT, без тела.
3. **No cache**: нет записи или `no-store` → полный запрос.

Для HTML-страниц обычно `Cache-Control: no-cache` (всегда ревалидировать). Для статики с hash в URL — `max-age=31536000, immutable` (кэшируется навсегда). Полный разбор директив — [06-response-return-caching-and-browser-rendering.md](./06-response-return-caching-and-browser-rendering.md).

---

## Resource hints: preconnect и prefetch

Браузер поддерживает явные подсказки для ускорения будущих запросов:

```html
<!-- Начать DNS lookup заранее (бесплатно по сети, без TCP) -->
<link rel="dns-prefetch" href="//fonts.googleapis.com">

<!-- Установить TCP+TLS к хосту заранее (≈ 1-2 RTT saved) -->
<link rel="preconnect" href="https://api.example.com" crossorigin>

<!-- Скачать ресурс заранее (высокий приоритет) -->
<link rel="preload" href="/fonts/Inter.woff2" as="font" crossorigin>

<!-- Prefetch: низкий приоритет, для следующей страницы -->
<link rel="prefetch" href="/next-page">

<!-- Prerender: полностью загрузить и отрисовать страницу в фоне -->
<link rel="prerender" href="/likely-next-page">
```

(`rel="prerender"` устарел — Chrome заменил его Speculation Rules API; остальные hints актуальны.)

`preconnect` к API/CDN хостам — самая простая оптимизация. До первого запроса к `api.example.com` браузер уже имеет открытое соединение, TCP+TLS latency = 0.

---

## Navigation Timing API: что браузер измеряет

Браузер собирает performance metrics. Инструментация доступна через `window.performance.timing` (legacy) и `PerformanceNavigationTiming` (modern):

```javascript
const nav = performance.getEntriesByType('navigation')[0];

console.log({
    dns:      nav.domainLookupEnd   - nav.domainLookupStart,   // DNS lookup
    tcp:      nav.connectEnd        - nav.connectStart,         // TCP + TLS
    ttfb:     nav.responseStart     - nav.requestStart,         // Time To First Byte
    download: nav.responseEnd       - nav.responseStart,        // download body
    domParse: nav.domInteractive    - nav.responseEnd,          // HTML parse
    total:    nav.loadEventEnd      - nav.startTime,            // полная загрузка
});
```

**TTFB** (Time To First Byte) — ключевая метрика серверной части, но её граница зависит от инструмента, и это стоит проговаривать явно. В Navigation Timing выше `responseStart - requestStart` — время от отправки запроса до первого байта ответа: сеть в одну сторону плюс работа сервера (очередь в балансировщике, middleware, обработчик, база, сериализация). В `curl -w %{time_starttransfer}` отсчёт идёт от старта команды, поэтому туда попадают ещё DNS, TCP и TLS. Сравнение цифр из разных источников без этой оговорки даёт расхождение в сотни миллисекунд на ровном месте.

Хорошие значения TTFB:
- < 200ms — отлично.
- 200–500ms — приемлемо.
- > 800ms — нужна оптимизация backend или CDN.

---

## Interview-ready answer

**1. Что происходит в браузере до первого сетевого пакета?**

- Разбор строки — браузер решает, это адрес или поисковый запрос, и нормализует адрес к HTTPS.
- HSTS-кэш — если домен в нём, попытки открыть по HTTP не будет вообще.
- HTTP-кэш — свежий ответ отдаётся сразу, сеть не задействуется.
- Service Worker — если зарегистрирован, запрос уходит ему, и он может ответить из своего хранилища.
- Соединения — браузер ищет уже открытое соединение к хосту, чтобы не платить за рукопожатия заново.
- Отдельная деталь для собеседования: фрагмент после `#` на сервер не отправляется никогда.

**2. Что такое HSTS и зачем нужен preload?**

- Механика — заголовок `Strict-Transport-Security` заставляет браузер ходить на домен только по HTTPS в течение `max-age`.
- Дыра без preload — политика запоминается лишь после первого успешного ответа по HTTPS, поэтому самый первый запрос остаётся уязвимым к перехвату.
- Preload — список доменов, вшитый в браузер: защита работает до первого подключения.
- Цена — удаление из списка занимает месяцы, поэтому включают его, только когда HTTPS поддерживается надолго и на всех поддоменах.

**3. Чем Service Worker опасен для деплоя backend?**

- Он перехватывает запросы и может отвечать из своего хранилища вообще без сети.
- Следствие для релиза — новая версия не дойдёт до пользователя, пока не обновится сам Service Worker.
- Худший случай — закэширована сломанная страница: backend здоров, а пользователь видит ошибку.
- Обратная сторона той же силы — работа офлайн и нулевое время до первого байта; лечение инцидента — снятие регистрации Service Worker.

**4. Что такое TTFB и что в него входит?**

- Определение зависит от инструмента, и это первое, что стоит уточнить: в Navigation Timing это `responseStart - requestStart`, то есть сеть в одну сторону плюс работа сервера.
- В `curl -w %{time_starttransfer}` отсчёт идёт от старта команды, поэтому туда попадают ещё DNS, TCP и TLS.
- Со стороны сервера в метрику входят очередь в балансировщике, middleware, обработчик, запросы к базе и сериализация ответа.
- Ориентиры — до 200 мс хорошо, 200–500 мс приемлемо, дольше 800 мс означает работу над backend или вынос контента ближе к пользователю.
