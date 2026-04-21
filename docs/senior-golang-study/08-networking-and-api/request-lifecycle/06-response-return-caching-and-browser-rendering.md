# Response: Caching и Browser Rendering

После того как backend сформировал ответ, работа не заканчивается. Ответ проходит обратный путь, кэшируется на нескольких уровнях, и браузер превращает его в пиксели.

## Содержание

- [Обратный путь через edge](#обратный-путь-через-edge)
- [Cache-Control: директивы и их значение](#cache-control-директивы-и-их-значение)
- [ETag и условные запросы](#etag-и-условные-запросы)
- [Vary: опасный заголовок](#vary-опасный-заголовок)
- [Инвалидация CDN-кэша](#инвалидация-cdn-кэша)
- [Browser parsing: от HTML до пикселей](#browser-parsing-от-html-до-пикселей)
- [Core Web Vitals: что измеряет Google](#core-web-vitals-что-измеряет-google)
- [Render-blocking ресурсы](#render-blocking-ресурсы)
- [Где тут бывают проблемы](#где-тут-бывают-проблемы)
- [Interview-ready answer](#interview-ready-answer)

## Обратный путь через edge

Ответ идёт обратно через те же слои в обратном порядке:

```text
Backend → Reverse Proxy → LB → CDN → Browser
```

На каждом уровне возможны:
- **Header manipulation**: proxy может добавить/удалить заголовки.
- **Compression**: если backend не сжал, nginx/CDN могут применить gzip/br.
- **Caching decision**: CDN смотрит на `Cache-Control` и решает, сохранить ли ответ.
- **Chunked transfer**: если backend стримит ответ, proxy должен поддерживать chunked encoding.

## Cache-Control: директивы и их значение

`Cache-Control` — самый важный заголовок кэширования. Директивы можно комбинировать.

| Директива | Смысл |
|---|---|
| `max-age=N` | Кэшировать на N секунд в browser и CDN |
| `s-maxage=N` | Только для shared caches (CDN). Переопределяет `max-age` для CDN |
| `public` | Разрешено кэшировать shared caches (CDN) |
| `private` | Только browser cache, CDN не кэширует |
| `no-cache` | Кэшировать можно, но перед использованием — revalidate с сервером |
| `no-store` | Не кэшировать вообще нигде (пароли, платёжные данные) |
| `stale-while-revalidate=N` | Отдать stale, параллельно обновить кэш в фоне |
| `stale-if-error=N` | При ошибке origin — отдавать stale до N секунд |
| `immutable` | Содержимое никогда не изменится (для versioned assets) |

Примеры для разных типов контента:

```http
# HTML страница: всегда revalidate
Cache-Control: no-cache

# Статика с version hash (main.a3b4c5.js): кэшировать вечно
Cache-Control: public, max-age=31536000, immutable

# API ответ: CDN кэширует 60s, browser revalidate
Cache-Control: public, s-maxage=60, max-age=0, must-revalidate

# Профиль пользователя: только browser
Cache-Control: private, max-age=300

# Стриминг / realtime данные
Cache-Control: no-store
```

`stale-while-revalidate` — мощный паттерн для снижения latency:
```http
Cache-Control: public, max-age=60, stale-while-revalidate=600
```
Ответ всегда быстрый (из кэша). Если кэш устарел больше 60s, но меньше 660s — отдаётся stale, и асинхронно обновляется. Пользователь не ждёт.

## ETag и условные запросы

ETag — fingerprint версии ресурса. Сервер генерирует (hash content, version number и т.п.).

**Поток с ETag:**

```text
1. Первый запрос:
   GET /api/config → 200 OK
   ETag: "abc123"
   Cache-Control: max-age=60

2. После 60s (max-age истёк):
   GET /api/config
   If-None-Match: "abc123"
   → сервер сравнивает ETag
   → контент не изменился: 304 Not Modified (тело пустое)
   → контент изменился: 200 OK с новым ETag

3. Результат при 304: нет передачи body → экономия трафика
```

**Сильный vs слабый ETag**:
- Сильный `ETag: "abc123"` — побайтовое совпадение. Используется для range requests.
- Слабый `ETag: W/"abc123"` — семантически эквивалентный контент. Подходит для dynamic pages.

**Last-Modified** — аналог ETag на основе времени. Менее надёжен (granularity секунда, clock skew). Лучше использовать ETag.

В Go:
```go
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
    cfg := h.config.Load()
    etag := fmt.Sprintf(`"%x"`, cfg.Hash())

    if r.Header.Get("If-None-Match") == etag {
        w.WriteHeader(http.StatusNotModified)
        return
    }

    w.Header().Set("ETag", etag)
    w.Header().Set("Cache-Control", "public, max-age=60")
    json.NewEncoder(w).Encode(cfg)
}
```

## Vary: опасный заголовок

`Vary` говорит кэшу: "создай отдельную запись кэша для каждого значения этих заголовков".

```http
Vary: Accept-Encoding
```
Разумно: отдельный кэш для gzip и для br. Два варианта.

```http
Vary: Cookie
```
Опасно: каждое уникальное значение Cookie → отдельная запись в CDN. При миллионах пользователей — фактически означает "не кэшировать на CDN". Origin получает всю нагрузку.

```http
Vary: Accept-Language
```
Умеренно: если ты отдаёшь разный контент по языку — нормально. Но fingerprint пользователей через `Accept-Language` + `Cookie` = огромная фрагментация.

Правило: `Vary` только на то, что действительно влияет на контент. CDN-провайдеры (Cloudflare, CloudFront) часто игнорируют `Vary: Cookie` для управляемых кэшей.

## Инвалидация CDN-кэша

Проблема: CDN закэшировал баг. Нужно сбросить кэш до истечения TTL.

Подходы:

**1. URL versioning** (лучший для статики):
```html
<script src="/static/main.a3b4c5.js"></script>
```
Новый деплой → новый hash → новый URL. CDN никогда не отдаст старый файл по новому URL. Старый URL истечёт по TTL.

**2. Явная инвалидация через API**:
```bash
# Cloudflare
curl -X POST "https://api.cloudflare.com/client/v4/zones/{zone_id}/purge_cache" \
  -H "Authorization: Bearer {token}" \
  -d '{"files":["https://example.com/api/config"]}'

# CloudFront invalidation (дороже, по количеству путей)
aws cloudfront create-invalidation --distribution-id E1234 \
  --paths "/api/config" "/api/settings"
```

**3. Surrogate keys / Cache tags** (Cloudflare, Fastly):
```http
Surrogate-Key: user-42 product-catalog
```
Одним API-вызовом инвалидировать все ответы с тегом `product-catalog` — независимо от URL.

## Browser parsing: от HTML до пикселей

Когда HTML-документ начал приходить, браузер не ждёт полной загрузки:

```text
1. HTML parser → DOM tree
2. CSS parser   → CSSOM tree (параллельно с HTML при наличии)
3. DOM + CSSOM  → Render Tree (только visible nodes)
4. Layout (Reflow): вычислить размеры и позиции
5. Paint: пиксели
6. Composite: layers → final image
```

JavaScript изменяет DOM и CSSOM, что может вызвать reflow и repaint.

При нахождении `<script src="...">` без атрибутов браузер:
1. Прекращает парсинг HTML.
2. Скачивает JS.
3. Выполняет JS.
4. Продолжает парсинг.

Это и есть render-blocking.

## Core Web Vitals: что измеряет Google

Google использует Core Web Vitals как сигнал ранжирования и UX-метрику:

| Метрика | Что измеряет | Хорошо | Плохо |
|---|---|---|---|
| **LCP** (Largest Contentful Paint) | Когда главный контент виден | < 2.5s | > 4s |
| **INP** (Interaction to Next Paint) | Отзывчивость на клики/нажатия | < 200ms | > 500ms |
| **CLS** (Cumulative Layout Shift) | Прыжки контента при загрузке | < 0.1 | > 0.25 |

Ранее использовался FID (First Input Delay), заменён на INP в 2024.

**LCP** чаще всего ограничен:
- TTFB (Time To First Byte) — медленный origin.
- Размером hero image.
- Render-blocking JS/CSS.

**CLS** чаще всего вызван:
- Изображениями без `width`/`height`.
- Динамически вставляемыми элементами (ads, banners).
- Шрифтами, которые меняют layout при загрузке (FOUT — Flash Of Unstyled Text).

## Render-blocking ресурсы

CSS в `<head>` блокирует рендеринг (CSSOM должен быть готов для Render Tree):
```html
<link rel="stylesheet" href="styles.css">  <!-- блокирует render -->
```

JavaScript без атрибутов блокирует парсинг HTML:
```html
<script src="app.js"></script>               <!-- блокирует parser -->
<script src="app.js" defer></script>         <!-- не блокирует, выполнится после parse -->
<script src="app.js" async></script>         <!-- скачивает параллельно, выполнит как скачает -->
<script type="module" src="app.js"></script> <!-- как defer -->
```

Подсказки браузеру (resource hints):
```html
<!-- Начать DNS-lookup заранее -->
<link rel="dns-prefetch" href="//api.example.com">

<!-- TCP + TLS к хосту заранее -->
<link rel="preconnect" href="https://fonts.googleapis.com">

<!-- Скачать ресурс заранее с высоким приоритетом -->
<link rel="preload" href="/fonts/Inter.woff2" as="font" crossorigin>

<!-- Предзагрузить следующую страницу -->
<link rel="prefetch" href="/next-page">
```

`preconnect` к CDN/API-хостам — простая оптимизация, которая экономит TCP+TLS RTT (до 200ms) до первого использования.

## Где тут бывают проблемы

- **CDN отдаёт stale** после деплоя: не настроена инвалидация или TTL слишком большой.
- **`Vary: Cookie`** фактически отключает CDN-кэш — каждый пользователь бьёт в origin.
- **Нет `Cache-Control`**: разные браузеры и CDN по-разному интерпретируют отсутствие заголовка (эвристическое кэширование).
- **`no-cache` перепутан с `no-store`**: `no-cache` не запрещает кэшировать, он требует revalidate. Для конфиденциальных данных нужен `no-store`.
- **Большой JS bundle**: LCP растёт, INP страдает из-за long tasks.
- **Изображения без размеров**: CLS при загрузке.
- **TTFB > 600ms**: нет смысла оптимизировать frontend, если origin медленный.

## Interview-ready answer

`Cache-Control` управляет кэшированием: `s-maxage` — только для CDN (shared cache), `max-age` — browser и CDN, `private` — только browser. `no-cache` ≠ `no-store`: первый требует revalidation, второй запрещает кэш полностью. ETag + `If-None-Match` позволяют отвечать 304 Not Modified без тела — экономия bandwidth. `stale-while-revalidate` отдаёт stale ответ мгновенно, обновляет кэш в фоне — хорош для API с мягкими требованиями к свежести. `Vary: Cookie` убивает CDN-кэш. Для инвалидации статики: URL versioning (hash в имени файла) надёжнее явной инвалидации. Core Web Vitals (LCP, INP, CLS) — метрики пользовательского опыта; backend влияет прежде всего через TTFB → LCP.
