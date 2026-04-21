# CDN, Load Balancer, Reverse Proxy

После того как browser отправил запрос, он редко попадает сразу в application process. Обычно сначала участвует edge layer — несколько уровней с разными ролями.

## Содержание

- [Типичная топология edge](#типичная-топология-edge)
- [CDN: когда отвечает edge, а не origin](#cdn-когда-отвечает-edge-а-не-origin)
- [L4 vs L7 Load Balancer: в чём разница](#l4-vs-l7-load-balancer-в-чём-разница)
- [Алгоритмы балансировки](#алгоритмы-балансировки)
- [Health checks: active vs passive](#health-checks-active-vs-passive)
- [Sticky sessions и consistent hashing](#sticky-sessions-и-consistent-hashing)
- [Reverse proxy: что делает Nginx/Envoy](#reverse-proxy-что-делает-nginxenvoy)
- [Заголовки, которые добавляет edge](#заголовки-которые-добавляет-edge)
- [Circuit breaker на уровне proxy](#circuit-breaker-на-уровне-proxy)
- [Где тут бывают проблемы](#где-тут-бывают-проблемы)
- [Interview-ready answer](#interview-ready-answer)

## Типичная топология edge

```text
User
  │
  ▼
CDN / Anycast edge (Cloudflare, CloudFront, Fastly)
  │ cache miss / не-кэшируемый запрос
  ▼
WAF (Web Application Firewall) — опционально
  │
  ▼
L4 LB (AWS NLB, GCP Network LB) — TCP/UDP routing
  │
  ▼
L7 LB / Reverse Proxy (Nginx, Envoy, HAProxy, Ingress)
  │ TLS termination, routing, auth
  ▼
Backend pods / instances
```

В простом окружении может быть только Nginx без CDN. В больших системах каждый слой решает свою задачу и настраивается независимо.

## CDN: когда отвечает edge, а не origin

CDN распределяет копии контента по PoP (Point of Presence) по всему миру. Запрос идёт к ближайшему PoP.

**Cache hit**: статика (JS, CSS, images) с `Cache-Control: public, max-age=31536000` — ответ с edge, origin не видит запрос вообще. Latency: 5–20 ms вместо 100–200 ms до origin.

**Cache miss или некэшируемый**: CDN передаёт запрос на origin. Origin → CDN → user. CDN кэширует ответ на будущее.

Что кэшируется на CDN, а что нет:
- `Cache-Control: public, s-maxage=300` → CDN кэшируeт на 300s, browser — стандартно.
- `Cache-Control: private` → только browser cache, CDN не кэширует.
- `Cache-Control: no-store` → не кэшируется нигде.
- `Set-Cookie` в ответе → CDN часто не кэширует (персонализированный контент).
- `Vary: Cookie` → отдельный кэш-вариант для каждого значения Cookie — практически означает "не кэшировать на CDN".

**Invalidation**: CDN-кэш не следит за изменениями на origin. Явная инвалидация через API (Cloudflare cache purge, CloudFront invalidation). Быстрый способ: версионирование URL (`main.a3b4c5.js`) — новый URL = новый контент, старый URL истечёт по TTL.

## L4 vs L7 Load Balancer: в чём разница

**L4 (Transport Layer)**:
- Работает с TCP/UDP потоками, не видит HTTP.
- Маршрутизация по IP:port и протоколу.
- Очень быстрый, минимальный overhead.
- Не может принять решение на основе URL path, заголовков или cookie.
- Не выполняет TLS termination (можно, но обычно не нужно).
- Примеры: AWS NLB, GCP Network LB, Linux IPVS.

**L7 (Application Layer)**:
- Видит HTTP-заголовки, URL, метод, cookie, body.
- Может маршрутизировать `/api/*` на один кластер, `/static/*` — на другой.
- Выполняет TLS termination: расшифровывает запрос, разбирает HTTP, шифрует снова для upstream (или отправляет plaintext по внутренней сети).
- Может добавлять/удалять заголовки, проводить auth, rate limiting.
- Больше overhead из-за парсинга HTTP.
- Примеры: Nginx, Envoy, HAProxy, AWS ALB, Kubernetes Ingress.

В типичной production-архитектуре L4 снаружи (принимает TCP), L7 внутри (разбирает HTTP).

## Алгоритмы балансировки

| Алгоритм | Когда использовать |
|---|---|
| **Round Robin** | Все instances одинаковые, запросы однородные |
| **Least Connections** | Разная длительность запросов (upload, streaming) |
| **Weighted Round Robin** | Инстансы с разной capacity (разные типы машин) |
| **IP Hash** | Нужна "мягкая" привязка клиента к серверу |
| **Random** | Простая балансировка без state на LB |
| **Least Response Time** | Envoy, adaptive; route к самому быстрому |

Для stateless микросервисов в k8s: round robin или least connections — оба хороши. `kube-proxy` использует iptables round robin по умолчанию.

## Health checks: active vs passive

**Active health checks**: LB периодически делает probe к каждому instance.
- HTTP: GET `/healthz` → 200 OK.
- TCP: открыть соединение → закрыть.
- gRPC: health check protocol.

```text
LB → GET /healthz → instance
     200 OK → mark healthy
     timeout или 5xx → mark unhealthy → stop routing
```

**Passive health checks** (outlier detection в Envoy): LB наблюдает за реальным трафиком. Если instance возвращает 5xx подряд N раз — временно исключается из ротации. Плюс: реакция быстрее. Минус: реальные запросы пользователей страдают до исключения.

Правило: active health checks обязательны. Passive — дополнительная защита.

**Connection draining**: при деплое (instance уходит) LB перестаёт посылать новые запросы, но ждёт завершения in-flight запросов. Время draining должно быть больше максимального времени обработки запроса.

## Sticky sessions и consistent hashing

**Sticky sessions** — привязка клиента к конкретному instance для сохранения состояния (если state хранится на instance, а не в Redis/DB).

Реализации:
- **Cookie-based**: LB ставит cookie с ID instance (`AWSALB=xxx`). Клиент возвращает cookie в следующих запросах.
- **IP hash**: hash(client_ip) % N. Ломается при NAT (тысячи клиентов под одним IP).

Лучшее решение: вынести state в Redis/DB и сделать instances по-настоящему stateless. Sticky sessions — костыль.

**Consistent hashing** используется в CDN и distributed cache:
- Добавление/удаление node реструктурирует минимальное количество ключей (не всё сразу, как в modulo).
- Применение: Nginx `upstream` с `hash $request_uri consistent;` для cache affinity.
- Применение в Redis Cluster: 16384 hash slots равномерно распределены по master-нодам.

## Reverse proxy: что делает Nginx/Envoy

Reverse proxy получает запрос от клиента и проксирует к upstream. Основные функции:

1. **TLS termination**: расшифровывает запрос от клиента. К upstream может идти HTTP (по внутренней сети) или mTLS.
2. **Request routing**: по `Host`, `path`, `method`, заголовкам.
3. **Header manipulation**: добавить `X-Forwarded-For`, удалить `Authorization` перед логированием.
4. **Compression**: `gzip`/`br` ответов.
5. **Buffering**: nginx буферизует медленных клиентов — upstream отдал ответ быстро, nginx буферизует и отправляет клиенту в его темпе. Upstream горутина освобождается.
6. **Rate limiting**: `limit_req_zone` в nginx.
7. **Retries**: при 502 повторить запрос к другому upstream instance (только для идемпотентных запросов).

Nginx конфиг для Go backend:
```nginx
upstream go_backend {
    server 10.0.0.1:8080;
    server 10.0.0.2:8080;
    keepalive 32;  # пул keep-alive соединений к upstream
}

server {
    listen 443 ssl;
    ssl_protocols TLSv1.2 TLSv1.3;

    location /api/ {
        proxy_pass http://go_backend;
        proxy_http_version 1.1;
        proxy_set_header Connection "";  # для keep-alive к upstream
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        proxy_connect_timeout 5s;
        proxy_read_timeout 30s;  # должен быть >= WriteTimeout Go сервера
        proxy_send_timeout 10s;
    }
}
```

## Заголовки, которые добавляет edge

Приложение должно знать об этих заголовках и доверять им только от доверенных proxy:

| Заголовок | Что содержит |
|---|---|
| `X-Forwarded-For` | IP клиента (может быть список при нескольких proxy) |
| `X-Real-IP` | IP клиента (nginx, один адрес) |
| `X-Forwarded-Proto` | Исходный протокол (`http` или `https`) |
| `X-Request-Id` | Идентификатор запроса для трассировки |
| `Traceparent` | W3C Trace Context для distributed tracing |
| `CF-Connecting-IP` | Реальный IP клиента от Cloudflare |

**IP spoofing**: клиент может сам поставить `X-Forwarded-For: 8.8.8.8`. Доверяй только последнему proxy в цепочке.

В Go:
```go
func clientIP(r *http.Request) string {
    // Если за доверенным proxy (nginx ставит X-Real-IP)
    if ip := r.Header.Get("X-Real-IP"); ip != "" {
        return ip
    }
    // Иначе — первый не-proxy IP из XFF (с конца)
    ip, _, _ := net.SplitHostPort(r.RemoteAddr)
    return ip
}
```

## Circuit breaker на уровне proxy

Envoy реализует outlier detection — автоматическое исключение нездоровых upstream:

```yaml
outlier_detection:
  consecutive_5xx: 5           # 5 подряд 5xx → eject
  interval: 10s                # период анализа
  base_ejection_time: 30s      # минимальное время исключения
  max_ejection_percent: 50     # не исключать больше 50% nodes
```

Паттерн circuit breaker в приложении (кроме proxy-уровня) полезен для downstream сервисов, которые не проходят через Envoy:

```go
// sony/gobreaker или самописный с atomic state
cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
    MaxRequests: 1,
    Interval:    10 * time.Second,
    Timeout:     60 * time.Second,
    ReadyToTrip: func(counts gobreaker.Counts) bool {
        return counts.ConsecutiveFailures > 5
    },
})

result, err := cb.Execute(func() (interface{}, error) {
    return client.Get(ctx, url)
})
```

## Где тут бывают проблемы

- **CDN stale content**: инвалидация не выполнена после деплоя → пользователи видят старый JS.
- **Timeout mismatch**: proxy timeout меньше application timeout → proxy возвращает 504, backend ещё работает и тратит ресурсы. Решение: `proxy_read_timeout nginx > WriteTimeout Go server`.
- **LB шлёт на нездоровые instances**: health check endpoint слишком легкий (200 OK всегда) → не отражает реальное состояние (нет DB connection pool).
- **X-Forwarded-For pollution**: несколько proxy добавляют свои IP → получить реальный client IP сложно.
- **TLS termination gap**: трафик CDN → origin идёт по HTTP на внутренней сети → нарушение compliance.
- **Large headers blocked**: nginx по умолчанию `client_header_buffer_size 1k` — JWT токены часто больше.

## Interview-ready answer

L4 балансировщик работает на уровне TCP, не видит HTTP — быстрый, но не может маршрутизировать по URL. L7 видит HTTP, выполняет TLS termination, routing, auth. CDN кэширует на edge, снимает нагрузку с origin; `s-maxage` управляет CDN-кешем отдельно от `max-age`. Health checks бывают active (probe к `/healthz`) и passive (outlier detection по реальному трафику); оба нужны. Sticky sessions — костыль, stateless лучше. Consistent hashing используется для cache affinity — добавление ноды не инвалидирует весь кэш. Timeout proxy должен быть больше timeout приложения, иначе proxy разрывает соединение раньше, backend тратит ресурсы впустую.
