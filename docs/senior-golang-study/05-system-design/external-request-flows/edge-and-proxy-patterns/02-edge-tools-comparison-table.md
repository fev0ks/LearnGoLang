# Edge Tools Comparison Table

Эта таблица не про маркетинг и не про все feature details, а про роли инструментов в архитектуре.

## Сравнение

| Tool | Типичная роль | Где обычно стоит | Что обычно делает | Когда уместен |
| --- | --- | --- | --- | --- |
| Cloudflare | Edge, CDN, WAF | Самый внешний слой | TLS, WAF, DDoS, caching, proxying | Публичный интернет-трафик, глобальный edge |
| CloudFront | CDN и edge delivery | Перед origin в AWS | CDN caching, TLS, origin routing | Статика и public content в AWS |
| Fastly | Edge CDN и compute edge | Самый внешний слой | Fast CDN, edge caching, request logic | High-performance public edge |
| Akamai | Enterprise edge CDN | Самый внешний слой | CDN, security, edge delivery | Большие enterprise и global delivery |
| Nginx | Reverse proxy, ingress, web server | После edge или прямо перед app | Proxy, routing, static files, TLS, buffering | Origin-side proxy, ingress, simple edge |
| Envoy | Reverse proxy, ingress, service proxy | Edge, ingress, service mesh | L7 routing, observability, retries, policy | Modern L7 proxy, ingress, mesh-style setups |
| HAProxy | Load balancer и proxy | Перед app pool | High-performance LB, TCP and HTTP balancing | Simple fast balancing and proxying |
| Cloud LB | Managed load balancer | Между edge и app | Health checks, routing, balancing | Cloud-native external entrypoint |
| API Gateway | API control layer | Перед services | Auth, quotas, transformation, routing | Public APIs, multi-tenant APIs |

## Как правильно это сравнивать

Самая частая ошибка:
- сравнивать `Cloudflare` и `nginx` как будто это один и тот же класс инструментов.

На практике:
- `Cloudflare` чаще внешний глобальный edge;
- `nginx` чаще reverse proxy ближе к твоему origin;
- `Envoy` чаще advanced L7 proxy или ingress;
- `HAProxy` часто быстрый балансировщик и proxy;
- `API gateway` это больше про policy и external API control.

## Практический shorthand

Если у тебя вопрос "кто первый принимает интернет-трафик":
- чаще это Cloudflare, CloudFront, Fastly, Akamai или cloud LB.

Если вопрос "кто стоит прямо перед приложением":
- часто это Nginx, Envoy, HAProxy или ingress controller.

Если вопрос "кто проверяет API key, tenant quota и auth policy":
- часто это API gateway или gateway-like слой.

## Что важно на интервью

Нужно не перечислить бренды, а показать роль:
- кто edge;
- кто балансирует;
- кто проксирует;
- кто валидирует auth;
- где выполняется caching;
- где реальный origin.
