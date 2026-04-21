# DNS Resolution And Getting IP

После того как browser понял, что нужно открыть `google.com`, ему нужен IP-адрес. DNS — не "просто lookup", а иерархическая кэширующая система с реальными задержками.

## Содержание

- [Иерархия DNS cache с TTL](#иерархия-dns-cache-с-ttl)
- [Recursive resolution](#recursive-resolution)
- [Типы DNS-записей](#типы-dns-записей)
- [CNAME chains и скрытая latency](#cname-chains-и-скрытая-latency)
- [Negative caching](#negative-caching)
- [Happy Eyeballs](#happy-eyeballs)
- [DNS over HTTPS и DNS over TLS](#dns-over-https-и-dns-over-tls)
- [DNS в Kubernetes: CoreDNS](#dns-в-kubernetes-coredns)
- [Где здесь бывает latency и failure](#где-здесь-бывает-latency-и-failure)
- [Debugging: dig и практика](#debugging-dig-и-практика)
- [Interview-ready answer](#interview-ready-answer)

## Иерархия DNS cache с TTL

DNS-lookup проходит несколько уровней кэша, каждый с разным TTL:

```text
1. Browser DNS cache        — TTL из записи, но мин. ~1 мин в Chrome
2. OS resolver cache        — nscd / systemd-resolved / mDNSResponder
3. Router DNS cache         — часто 5–60 мин
4. ISP / corporate resolver — держит popular records часами
5. Recursive resolver       — 8.8.8.8, 1.1.1.1 и т.д.
6. Authoritative DNS        — source of truth, устанавливает TTL
```

Если запись найдена на любом уровне и TTL не истёк — дальше не идём. Типичная latency:
- Browser cache hit: **0 ms**
- OS cache hit: **< 1 ms**
- Recursive resolver cache hit: **1–10 ms** (зависит от расстояния)
- Full recursive lookup (до authoritative): **20–120 ms**

TTL в DNS-записи определяет, сколько времени запись можно кэшировать. Изменения DNS вступают в силу только после истечения TTL всех кэшей — это важно при миграции.

**Практика**: перед DNS-миграцией понижай TTL до 300s (5 мин) за 48 часов. После успешной смены — поднимай обратно до 3600+.

## Recursive resolution

Если записи нет ни в одном кэше, recursive resolver проходит цепочку:

```text
Browser → Recursive resolver (8.8.8.8)
                   │
                   ├─► Root servers (13 кластеров)
                   │     "Кто отвечает за .com?"
                   │
                   ├─► TLD nameservers (.com)
                   │     "Кто отвечает за google.com?"
                   │
                   └─► Authoritative nameserver (ns1.google.com)
                             "A-запись для google.com = 142.250.x.x"
```

Recursive resolver кэширует результат на TTL записи. Следующий клиент (другой пользователь того же resolver) получит ответ мгновенно.

## Типы DNS-записей

| Запись | Назначение | Пример |
|---|---|---|
| `A` | IPv4-адрес | `google.com → 142.250.80.46` |
| `AAAA` | IPv6-адрес | `google.com → 2a00:1450:...` |
| `CNAME` | Псевдоним → другой hostname | `www.example.com → example.com` |
| `MX` | Mail server | приоритет + hostname |
| `TXT` | Произвольный текст | SPF, DKIM, domain verification |
| `SRV` | Сервис + порт + приоритет | используется в k8s, gRPC discovery |
| `NS` | Authoritative nameserver | делегирование зоны |
| `PTR` | Reverse DNS (IP → hostname) | для логов, SPF |

Один A-record для `google.com` обычно возвращает **несколько IP**. Браузер выберет один — это уже часть geo-routing и anycast балансировки.

## CNAME chains и скрытая latency

CNAME — это перенаправление одного hostname на другой. Браузер должен разрезолвить итоговый hostname.

```text
cdn.example.com   → CNAME → example.cloudfront.net
example.cloudfront.net → CNAME → d1234.cloudfront.net
d1234.cloudfront.net → A → 13.32.x.x
```

Каждый CNAME — потенциально дополнительный lookup, если нет в кэше. Длинные CNAME-цепочки (3+) заметно увеличивают DNS latency.

Правило: CDN и managed сервисы часто требуют CNAME. Выбирай провайдеров с коротким TTL у итогового A-record и локальными anycast PoP.

## Negative caching

NXDOMAIN (домен не существует) тоже кэшируется. Время кэширования берётся из `NCACHE` записи в SOA.

```bash
# запись не существует — кэшируется на SOA MINIMUM TTL
dig nonexistent.example.com
# → NXDOMAIN, cached for 300s
```

Практическое значение: если сервис временно недоступен из-за DNS-ошибки — даже после её устранения, клиенты будут получать NXDOMAIN ещё несколько минут.

## Happy Eyeballs

RFC 6555 — алгоритм для быстрого выбора между IPv4 и IPv6.

Если у хоста есть оба — браузер делает TCP SYN к IPv6 и IPv4 почти одновременно (с задержкой ~300ms для IPv4). Использует то соединение, которое первым успешно установится.

```text
t=0ms:    SYN к 2a00::1234 (IPv6)
t=300ms:  SYN к 142.250.x.x (IPv4)  ← если IPv6 не ответил
t=310ms:  SYN-ACK от 142.250.x.x    ← побеждает IPv4
           → continue with IPv4
```

Для backend инженера: если IPv6 плохо работает в корпоративной сети — включи логирование DNS для диагностики. `Happy Eyeballs` скроет проблему от пользователя, но создаст лишние DNS queries.

## DNS over HTTPS и DNS over TLS

Стандартный DNS работает на UDP 53 — plaintext, видно всем (ISP, корпоративный firewall, MITM).

**DoT (DNS over TLS, порт 853)**: DNS поверх TLS. Шифрует запросы, но порт 853 часто блокируется.

**DoH (DNS over HTTPS, порт 443)**: DNS-запросы выглядят как HTTPS-трафик. Chrome, Firefox используют DoH по умолчанию к настроенному resolver (Cloudflare 1.1.1.1, Google 8.8.8.8).

Последствие для backend: корпоративный DNS-мониторинг может не видеть запросы браузеров с DoH.

## DNS в Kubernetes: CoreDNS

В k8s кластере DNS обслуживает CoreDNS (обычно `kube-dns` service). Каждый Pod имеет `/etc/resolv.conf`, указывающий на CoreDNS.

Service discovery через DNS:
```text
my-service.my-namespace.svc.cluster.local  → ClusterIP
my-pod.my-namespace.pod.cluster.local       → Pod IP
```

Короткое имя `my-service` работает в пределах того же namespace благодаря search domains в `/etc/resolv.conf`:
```text
search my-namespace.svc.cluster.local svc.cluster.local cluster.local
nameserver 10.96.0.10  # CoreDNS ClusterIP
```

Проблема: каждый DNS-запрос на короткое имя пробует несколько search domains. Для `my-service` делается 4 запроса: `my-service.my-namespace.svc.cluster.local` → `my-service.svc.cluster.local` → `my-service.cluster.local` → `my-service.`. Реши добавлением точки: `my-service.my-namespace.svc.cluster.local.` — это FQDN, без перебора.

CoreDNS — bottleneck при высокой нагрузке. Настраивай кэширование через `cache` плагин и `ndots` в Pod spec.

## Где здесь бывает latency и failure

| Проблема | Симптом | Решение |
|---|---|---|
| DNS cache miss (full lookup) | первый запрос к хосту 50–120 ms | prefetch DNS, keep-alive |
| Слишком маленький TTL | постоянные lookups | TTL ≥ 300s для стабильных записей |
| Слишком большой TTL | медленное обновление при failover | TTL 60s для A-record при geo-LB |
| NXDOMAIN после ошибки | клиенты получают ошибку минутами после фикса | низкий negative TTL |
| CoreDNS перегружен | intermittent 5xx в k8s | горизонтальное масштабирование CoreDNS |

## Debugging: dig и практика

```bash
# полный recursive lookup с timing
dig +stats google.com

# посмотреть TTL и authoritative server
dig +nocmd +noall +answer +ttlid google.com

# trace: показать весь путь от root
dig +trace google.com

# reverse DNS
dig -x 142.250.80.46

# конкретный resolver
dig @8.8.8.8 google.com

# AAAA-записи
dig AAAA google.com
```

Пример `dig +stats` output:
```text
;; Query time: 23 msec
;; SERVER: 8.8.8.8#53(8.8.8.8)
;; WHEN: Mon Apr 20 10:00:00 2026
;; MSG SIZE  rcvd: 55
```

23 ms = время до resolver, который уже имел запись в кэше. Для cold cache (full recursive) — 50–120 ms.

## Interview-ready answer

DNS — кэширующая иерархия: browser → OS → router → recursive resolver → authoritative. Каждый уровень имеет свой TTL. Full recursive lookup занимает 20–120 ms; cache hit — <10 ms. CNAME — псевдоним на другой hostname, требует дополнительного lookup. Negative caching: NXDOMAIN кэшируется — даже после исправления DNS-ошибки клиенты некоторое время получают ошибку. Happy Eyeballs — параллельный race между IPv4 и IPv6. В Kubernetes CoreDNS обслуживает service discovery, короткие имена генерируют несколько DNS-запросов из-за search domains — FQDN (с точкой на конце) избегает этого. Перед DNS-миграцией: снижай TTL до 300s за 48 часов.
