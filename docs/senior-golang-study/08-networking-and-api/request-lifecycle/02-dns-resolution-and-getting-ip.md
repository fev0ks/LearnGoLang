# Разрешение имени и получение адреса

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

После того как browser понял, что нужно открыть `google.com`, ему нужен IP-адрес. DNS — не "просто lookup", а иерархическая кэширующая система с реальными задержками.

---

## Иерархия DNS cache с TTL

DNS-lookup проходит несколько уровней кэша, каждый с разным TTL:

```text
1. Browser DNS cache        — Chrome держит ~60 с независимо от TTL записи
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

**Практика**: перед DNS-миграцией TTL понижается до 300s (5 мин) за 48 часов, после успешной смены — поднимается обратно до 3600+.

---

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

---

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

Один A-record для `google.com` обычно возвращает несколько IP. Браузер выберет один — это уже часть geo-routing и anycast балансировки.

---

## CNAME chains и скрытая latency

CNAME — это перенаправление одного hostname на другой. Браузер должен разрезолвить итоговый hostname.

```text
cdn.example.com   → CNAME → example.cloudfront.net
example.cloudfront.net → CNAME → d1234.cloudfront.net
d1234.cloudfront.net → A → 13.32.x.x
```

Каждый CNAME — потенциально дополнительный lookup, если нет в кэше. Длинные CNAME-цепочки (3+) заметно увеличивают DNS latency.

Правило: CDN и managed-сервисы часто требуют CNAME; предпочтительны провайдеры с коротким TTL у итогового A-record и локальными anycast PoP.

---

## Negative caching

NXDOMAIN (домен не существует) тоже кэшируется. Время кэширования — negative TTL из поля `MINIMUM` SOA-записи зоны (RFC 2308).

```bash
# запись не существует — кэшируется на SOA MINIMUM TTL
dig nonexistent.example.com
# → NXDOMAIN, cached for 300s
```

Практическое значение: если сервис временно недоступен из-за DNS-ошибки — даже после её устранения, клиенты будут получать NXDOMAIN ещё несколько минут.

---

## Happy Eyeballs

Алгоритм быстрого выбора между IPv4 и IPv6: RFC 6555 описал первую версию, действующая — RFC 8305.

Если у хоста есть обе записи, клиент начинает с IPv6 и, не дожидаясь результата, через небольшую паузу пробует IPv4; побеждает то соединение, которое установится первым. Пауза в первой версии алгоритма составляла порядка 300 мс, во второй её сократили до десятков миллисекунд, а разрешение имён A и AAAA выполняется параллельно.

```text
t=0ms:    SYN к 2a00::1234 (IPv6)
t=300ms:  SYN к 142.250.x.x (IPv4)  ← если IPv6 не ответил
t=310ms:  SYN-ACK от 142.250.x.x    ← побеждает IPv4
           → continue with IPv4
```

Для backend-инженера: если IPv6 плохо работает в корпоративной сети, стоит включить логирование DNS для диагностики — Happy Eyeballs скроет проблему от пользователя, но создаст лишние DNS-запросы.

---

## DNS over HTTPS и DNS over TLS

Стандартный DNS работает на UDP 53 — plaintext, видно всем (ISP, корпоративный firewall, MITM).

**DoT (DNS over TLS, порт 853)**: DNS поверх TLS. Шифрует запросы, но порт 853 часто блокируется.

**DoH (DNS over HTTPS, порт 443)**: DNS-запросы выглядят как HTTPS-трафик. Chrome, Firefox используют DoH по умолчанию к настроенному resolver (Cloudflare 1.1.1.1, Google 8.8.8.8).

Последствие для backend: корпоративный DNS-мониторинг может не видеть запросы браузеров с DoH.

---

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

Проблема в том, что короткое имя проходит перебор по списку search-доменов. Имя `my-service` не содержит точек, а `ndots` по умолчанию равен 5, поэтому резолвер сначала пробует его как относительное: `my-service.my-namespace.svc.cluster.local`, затем `my-service.svc.cluster.local`, затем `my-service.cluster.local`, и только потом как абсолютное. Это четыре попытки, а поскольку клиент обычно спрашивает A и AAAA, получается до восьми запросов на одно разрешение имени. Лечится двумя способами: писать полное имя с точкой на конце (`my-service.my-namespace.svc.cluster.local.`) либо понижать `ndots` в `dnsConfig` пода.

CoreDNS — bottleneck при высокой нагрузке; лечится `cache`-плагином и настройкой `ndots` в Pod spec.

---

## Где здесь бывает latency и failure

| Проблема | Симптом | Решение |
|---|---|---|
| DNS cache miss (full lookup) | первый запрос к хосту 50–120 ms | prefetch DNS, keep-alive |
| Слишком маленький TTL | постоянные lookups | TTL ≥ 300s для стабильных записей |
| Слишком большой TTL | медленное обновление при failover | TTL 60s для A-record при geo-LB |
| NXDOMAIN после ошибки | клиенты получают ошибку минутами после фикса | низкий negative TTL |
| CoreDNS перегружен | intermittent 5xx в k8s | горизонтальное масштабирование CoreDNS |

---

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

---

## Interview-ready answer

**1. Как устроено разрешение имени?**

- Иерархия кэшей — браузер, резолвер операционной системы, роутер, резолвер провайдера, авторитативный сервер; у каждого уровня свой срок жизни записи.
- Стоимость — попадание в кэш до 10 мс, полный обход до авторитативного сервера 20–120 мс.
- Полный обход — резолвер идёт по цепочке корневые серверы → серверы зоны `.com` → авторитативный сервер домена.
- CNAME — псевдоним на другое имя, и каждое звено цепочки это потенциальный дополнительный запрос, поэтому длинные цепочки заметно дороже.

**2. Что такое отрицательное кэширование и чем оно опасно?**

- Ответ NXDOMAIN тоже кэшируется, срок берётся из SOA-записи зоны.
- Практическое следствие — после исправления записи клиенты продолжают получать ошибку ещё несколько минут.
- Типичная жалоба «поправили DNS, а не работает» чаще всего именно про это, а не про ошибку в конфигурации.
- Профилактика — держать небольшой отрицательный TTL у зон, где записи меняются.

**3. Почему в Kubernetes короткие имена обходятся дорого?**

- Короткое имя дополняется списком search-доменов из `/etc/resolv.conf`, потому что `ndots` по умолчанию равен 5.
- Итог — до четырёх попыток на одно имя, и поскольку клиент спрашивает A и AAAA, до восьми запросов.
- Лечение — полное имя с точкой на конце либо понижение `ndots` в конфигурации пода.
- Смежная проблема — перегруженный CoreDNS: сбои резолва выглядят как спорадические 5xx без видимой причины на стороне приложения.

**4. Как переезжать на новый адрес без потери трафика?**

- За двое суток понизить TTL примерно до 300 секунд, чтобы кэши по всему пути обновлялись быстро.
- Дождаться истечения старого TTL и только затем менять запись.
- Проверить результат с нескольких резолверов, а не только со своей машины.
- Поднять TTL обратно после стабилизации и помнить, что часть резолверов держит записи дольше объявленного срока.
