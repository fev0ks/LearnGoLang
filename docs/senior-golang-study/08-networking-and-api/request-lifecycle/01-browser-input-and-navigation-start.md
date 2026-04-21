# Browser Input And Navigation Start

Когда пользователь вводит `google.com`, цепочка начинается не с backend и даже не с DNS, а с браузера. Множество решений принимается до первого сетевого пакета.

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

Fragment (`#section`) — **никогда не уходит на сервер**. Это исключительно браузерный механизм для якорной навигации. Но Single-Page Applications используют `hash routing` (`#/profile`) на клиенте.

URL encoding: пробелы → `%20`, спецсимволы → percent-encoded. Браузер делает это автоматически перед отправкой.

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

Удалить домен из preload list — долго (месяцы). Добавляй только если уверен в постоянной поддержке HTTPS.

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

## HTTP cache: можно ли обойтись без сети

Браузер имеет HTTP cache (disk cache + memory cache). При наличии свежей записи (max-age не истёк) — запрос в сеть не делается вообще.

Три варианта:
1. **Fresh**: `max-age` не истёк → отдать из cache, 200 (from cache). Сеть = 0.
2. **Stale + ETag**: `max-age` истёк, но есть ETag → conditional GET с `If-None-Match` → `304 Not Modified` (тело из cache). Сеть = 1 RTT, без тела.
3. **No cache**: нет записи или `no-store` → полный запрос.

Для HTML-страниц обычно `Cache-Control: no-cache` (всегда проверяй). Для статики с hash в URL — `max-age=31536000, immutable` (кэшируй навсегда).

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

`preconnect` к API/CDN хостам — самая простая оптимизация. До первого запроса к `api.example.com` браузер уже имеет открытое соединение, TCP+TLS latency = 0.

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

**TTFB** (Time To First Byte) — ключевая server-side метрика. Включает: DNS + TCP + TLS + queue в LB + middleware + handler + DB + serialization. Всё что делает сервер до первого байта ответа.

Хорошие значения TTFB:
- < 200ms — отлично.
- 200–500ms — приемлемо.
- > 800ms — нужна оптимизация backend или CDN.

## Interview-ready answer

Браузер принимает несколько решений до DNS: распознаёт URL vs поиск, нормализует к HTTPS, проверяет HSTS (если домен в HSTS-кэше — попытки HTTP нет), проверяет HTTP cache (fresh ответ = сеть не нужна), проверяет Service Worker (может ответить из SW cache). Fragment (`#`) не уходит на сервер. HSTS preload list — встроен в браузер, защищает даже от первого non-HTTPS запроса. Service Worker перехватывает запросы и может полностью обойти сеть — это и сила (offline), и риск (закэшированный баг). TTFB — главная server-side метрика latency, включает весь путь до первого байта ответа.
