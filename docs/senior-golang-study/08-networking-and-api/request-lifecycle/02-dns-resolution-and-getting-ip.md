# DNS Resolution And Getting IP

После того как browser понял, что нужно открыть `google.com`, ему нужен IP-адрес, куда отправлять сетевые пакеты.

## 1. Браузерный DNS cache

Сначала проверяется browser DNS cache.

Если свежая запись уже есть:
- новый DNS lookup не нужен;
- браузер сразу идет дальше к установке соединения.

## 2. OS resolver cache

Если browser cache не помог, запрос идет в операционную систему.

OS тоже может держать DNS cache:
- запись `A` для IPv4;
- запись `AAAA` для IPv6;
- иногда negative cache, если недавно домен не резолвился.

## 3. Запрос к recursive DNS resolver

Если локального ответа нет, запрос уходит к DNS resolver:
- это может быть resolver провайдера;
- корпоративный DNS;
- публичный resolver вроде `1.1.1.1` или `8.8.8.8`.

Браузер напрямую обычно не опрашивает root servers:
- это делает recursive resolver.

## 4. Что делает recursive resolver

Если записи нет в его cache, recursive resolver проходит цепочку:

1. спрашивает root DNS servers, кто отвечает за `.com`
2. спрашивает TLD servers, кто отвечает за `google.com`
3. спрашивает authoritative DNS server домена
4. получает конечную запись

На выходе браузеру возвращается IP или набор IP-адресов.

## 5. Что именно может вернуть DNS

Часто участвуют:
- `A` record для IPv4;
- `AAAA` record для IPv6;
- `CNAME`, если один host указывает на другой;
- TTL, который определяет, как долго запись можно кэшировать.

Практически:
- один hostname часто маппится не на один IP;
- это уже часть balancing и geo-routing story.

## 6. Что происходит после получения IP

Browser выбирает, куда подключаться:
- IPv4 или IPv6;
- один из нескольких IP;
- иногда с учетом Happy Eyeballs, чтобы быстрее выбрать рабочий маршрут.

## Где здесь бывает latency и failure

Проблемы бывают на каждом уровне:
- browser cache miss;
- медленный local resolver;
- packet loss до DNS;
- stale record;
- неправильный authoritative DNS;
- слишком маленький TTL и постоянные lookups;
- слишком большой TTL и медленное обновление маршрута.

## Что важно для backend engineer

Даже идеальный application server не спасет, если:
- домен не резолвится;
- resolver отвечает медленно;
- DNS cache протух;
- traffic идет на неправильный IP через stale DNS.

## Что могут спросить на интервью

- чем browser DNS cache отличается от OS DNS cache;
- что делает recursive resolver;
- чем отличаются `A`, `AAAA`, `CNAME`;
- зачем нужен TTL;
- почему DNS может быть источником latency или incident.
