# CDN Load Balancer Reverse Proxy

После того как browser отправил request, он редко попадает сразу в application process. Обычно сначала участвует edge layer.

## Содержание

- [Типичный путь на edge](#типичный-путь-на-edge)
- [1. CDN](#1-cdn)
- [2. Load balancer](#2-load-balancer)
- [3. Reverse proxy](#3-reverse-proxy)
- [4. Что добавляется к запросу](#4-что-добавляется-к-запросу)
- [5. Где тут бывают проблемы](#5-где-тут-бывают-проблемы)
- [Почему это важно для Go backend](#почему-это-важно-для-go-backend)
- [Что могут спросить на интервью](#что-могут-спросить-на-интервью)

## Типичный путь на edge

Запрос может пройти через:
- CDN;
- Anycast edge;
- WAF;
- L4 или L7 load balancer;
- reverse proxy;
- ingress gateway.

В простом локальном проекте этого слоя может не быть, но в production он почти всегда есть.

## 1. CDN

CDN нужен, чтобы:
- отдавать статику ближе к пользователю;
- уменьшать latency;
- снимать нагрузку с origin;
- кэшировать контент на edge.

Если запрос кэшируемый:
- CDN может отдать ответ без похода на origin.

Если запрос некэшируемый или cache miss:
- запрос идет дальше к origin или к следующему proxy layer.

## 2. Load balancer

Load balancer выбирает, куда отправить запрос:
- на какой data center;
- на какой cluster;
- на какой instance или pod.

Алгоритмы могут быть разные:
- round robin;
- least connections;
- weighted balancing;
- geo-routing;
- latency-based routing.

## 3. Reverse proxy

Reverse proxy часто делает:
- TLS termination;
- request routing;
- header normalization;
- compression;
- rate limiting;
- access logging;
- retries к upstream в некоторых сценариях.

Примеры:
- Nginx;
- Envoy;
- HAProxy;
- cloud ingress и managed LB.

## 4. Что добавляется к запросу

По дороге request часто получает служебные headers:
- `X-Forwarded-For`
- `X-Forwarded-Proto`
- `X-Request-Id`
- `Traceparent`
- `X-Real-IP`

Backend должен понимать:
- где доверенный proxy;
- как правильно читать client IP;
- где заканчивается transport layer и начинается application identity.

## 5. Где тут бывают проблемы

- CDN отдает stale content;
- LB шлет traffic в больные instances;
- proxy режет большие headers;
- ingress имеет меньший timeout, чем backend;
- TLS завершается на edge, а дальше идет неожиданный plaintext hop;
- потеря оригинального client IP.

## Почему это важно для Go backend

Даже если handler написан правильно:
- запрос может умереть раньше из-за WAF, proxy timeout или misrouting;
- response code может сгенерировать не приложение, а proxy;
- latency может накапливаться до приложения.

## Что могут спросить на интервью

- чем CDN отличается от reverse proxy;
- где обычно завершается TLS;
- как узнать реальный client IP за балансировщиком;
- почему timeout на proxy должен быть согласован с timeout приложения;
- как cache на CDN влияет на нагрузку на origin.
