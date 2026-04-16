# Ports And Expose

`ports` и `expose` отвечают за сетевые порты сервиса, но решают разные задачи.

## `expose`

`expose` открывает порт только для других контейнеров в сети и не публикует его на хост.

Пример:

```yaml
expose:
  - "8080"
  - "8081-8085/tcp"
```

Формат:
- `<port>/[<proto>]`
- `<start-end>/[<proto>]`

Если protocol не указан:
- используется `tcp`

Практически:
- в локальном Go-стеке `expose` нужен реже;
- если сервисом пользуются только другие контейнеры, его можно оставить без `ports`.

## `ports`

`ports` публикует порт контейнера на хост.

Это нужно, когда ты хочешь:
- открыть API в браузере или `curl`;
- зайти в Grafana или Prometheus;
- подключиться к Postgres или Redis с хоста.

## Short syntax

Формат:

```yaml
[HOST:]CONTAINER[/PROTOCOL]
```

Где:
- `HOST` это `[IP:](port | range)` и он optional;
- `CONTAINER` это `port | range`;
- `PROTOCOL` это `tcp` или `udp`.

Примеры:

```yaml
ports:
  - "8080:8080"
  - "127.0.0.1:8080:8080"
  - "6060:6060/udp"
  - "9090-9091:8080-8081"
  - "3000"
```

Что означают варианты:

`"8080:8080"`:
- хост `8080`
- контейнер `8080`

`"127.0.0.1:8080:8080"`:
- доступ только с localhost хоста;
- полезно, когда не хочешь светить сервис во внешнюю сеть.

`"3000"`:
- контейнерный порт `3000`;
- host port будет выбран автоматически.

Если host IP не указан:
- Docker обычно bindится на `0.0.0.0`.

Практический security caveat:
- `0.0.0.0` может случайно сделать сервис доступным извне, если машина имеет внешний IP.

## Long syntax

Long syntax нужен, когда хочется явно управлять полями.

```yaml
ports:
  - name: http
    target: 8080
    published: "8080"
    host_ip: 127.0.0.1
    protocol: tcp
    app_protocol: http
    mode: host
```

### Поля

`target`:
- контейнерный порт.

`published`:
- host port или range строкой.

`host_ip`:
- IP на хосте, куда bindится порт.

`protocol`:
- `tcp`
- `udp`

`app_protocol`:
- подсказка про application protocol;
- типичные значения: `http`, `https`.

`mode`:
- `host`
- `ingress`

`ingress` особенно относится к swarm-style publishing. Для обычного local compose чаще достаточно short syntax или `mode: host`.

`name`:
- человекочитаемое имя порта.

## Ограничения и важные нюансы

`ports` нельзя использовать с `network_mode: host`:
- в таком режиме контейнер уже сидит в host network namespace.

Short syntax лучше всегда писать как строку в кавычках:
- это защищает от YAML weirdness с числовыми значениями.

## Когда использовать `ports`, а когда нет

Нужен `ports`:
- API, к которому ты ходишь с хоста;
- UI сервисы вроде Grafana;
- debug endpoints, которые ты открываешь браузером или `curl`.

Часто не нужен `ports`:
- `postgres`, если к нему ходит только `api`;
- `redis`, если он нужен только контейнерам;
- internal worker-only сервисы.

## Быстрые примеры

API только для localhost:

```yaml
ports:
  - "127.0.0.1:8080:8080"
```

Prometheus UI:

```yaml
ports:
  - "9090:9090"
```

Только internal listener:

```yaml
expose:
  - "8080"
```
