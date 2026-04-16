# Networks

`networks` в Compose отвечают за связность контейнеров.

Главная practical идея:
- если сервисы в одной сети, они могут обращаться друг к другу по именам сервисов;
- `api` может ходить в `postgres:5432`, если оба сервиса подключены к одной сети.

## Что происходит по умолчанию

Если явные сети не описаны:
- Compose создает implicit `default` network;
- сервисы проекта попадают туда автоматически;
- сервисы видят друг друга по имени.

Это значит:
- `api` может обратиться к `postgres:5432`, если оба сервиса в одной сети;
- `localhost` внутри контейнера означает сам контейнер, а не соседний сервис.

## Базовый пример

```yaml
services:
  api:
    networks:
      - app-net
  postgres:
    networks:
      - app-net

networks:
  app-net:
    driver: bridge
```

## Часто используемые top-level network attributes

### `driver`

Определяет network driver.

Пример:

```yaml
driver: bridge
```

Частые варианты:
- `bridge` для локального Docker на одной машине;
- `overlay` для swarm и multi-host scenarios.

Для local dev почти всегда:
- `bridge`

### `driver_opts`

Опции для конкретного network driver.

Пример:

```yaml
driver_opts:
  com.docker.network.bridge.host_binding_ipv4: "127.0.0.1"
```

### `attachable`

```yaml
attachable: true
```

Что значит:
- standalone containers тоже могут подключаться к этой сети.

Чаще встречается:
- в более сложных сетевых сценариях;
- реже нужен для обычного local compose.

### `enable_ipv4`

Позволяет включить или отключить IPv4 assignment.

Допустимые значения:
- `true`
- `false`

### `enable_ipv6`

Включает IPv6 address assignment.

Допустимые значения:
- `true`
- `false`

### `external`

```yaml
external: true
```

Что значит:
- Compose не создает сеть сам;
- он подключает сервисы к уже существующей сети.

Важно:
- с `external: true` остальные атрибуты почти не используются;
- обычно имеет смысл только вместе с `name`.

### `ipam`

Позволяет задавать custom IPAM config.

Параметры:
- `driver`
- `config`
- `options`

Внутри `config` могут быть:
- `subnet`
- `ip_range`
- `gateway`
- `aux_addresses`

Это нужно редко, но полезно знать для сложных сетевых сценариев.

### `internal`

```yaml
internal: true
```

Что значит:
- сеть становится внешне изолированной;
- полезно для internal-only traffic.

### `labels`

Metadata на network resource.

Можно задавать:
- map-style;
- list-style.

### `name`

```yaml
name: my-app-net
```

Задает реальное имя сети, а не только logical key внутри compose-файла.

Полезно:
- для integration с external network;
- для более явного naming.

## Service-level `networks`

На уровне сервиса обычно указывают список сетей:

```yaml
networks:
  - app-net
```

В более сложных случаях сервис может быть в нескольких сетях:

```yaml
networks:
  - frontend
  - backend
```

Это позволяет:
- gateway или edge-сервису видеть и внешний, и внутренний трафик;
- изолировать DB от публичных компонентов.

### Расширенный service-level syntax

Вместо списка можно использовать mapping:

```yaml
services:
  api:
    networks:
      backend:
        aliases:
          - database-client
```

Здесь на уровне подключения сервиса к конкретной сети можно задавать дополнительные параметры.

### `aliases`

Дополнительные hostnames для сервиса внутри конкретной сети.

Важно:
- alias network-scoped;
- один и тот же сервис может иметь разные alias в разных сетях;
- alias не обязан быть уникальным, поэтому злоупотреблять ими не стоит.

### `interface_name`

Позволяет задать имя network interface внутри контейнера.

Нужно редко, обычно только в специальных networking-сценариях.

### `ipv4_address`, `ipv6_address`

Статический IP для подключения сервиса к сети.

Пример:

```yaml
ipv4_address: 172.16.238.10
ipv6_address: 2001:3984:3989::10
```

Важно:
- для этого в top-level `networks` должен быть настроен `ipam` с нужным subnet.

### `link_local_ips`

Список link-local IP адресов.

Полезно редко, чаще в очень специальных infra и network сценариях.

### `mac_address`

MAC-адрес на уровне подключения к конкретной сети.

### `driver_opts`

Driver-specific options именно для подключения сервиса к сети.

### `gw_priority`

Числовой приоритет выбора default gateway.

Практически:
- чем выше число, тем выше шанс, что именно эта сеть станет default gateway.

### `priority`

Числовой приоритет подключения сервиса к сетям.

Важно:
- это не то же самое, что default gateway;
- для gateway есть `gw_priority`.

## `network_mode`

Отдельный service key, который меняет сетевой режим контейнера целиком.

Частые значения:
- `none`
- `host`
- `service:{name}`
- `container:{name}`

Примеры:

```yaml
network_mode: "none"
network_mode: "host"
network_mode: "service:api"
network_mode: "container:some-container"
```

Что важно:
- `network_mode` нельзя сочетать с `networks`;
- при `network_mode: host` нельзя использовать `ports`, потому что контейнер уже работает в host network namespace.

## Practical rules

- для локального Go-стека обычно достаточно одной `bridge`-сети;
- внутри контейнера не используй `localhost` для другого контейнера;
- имя сервиса это DNS-имя внутри сети;
- несколько сетей нужны только если реально есть разные trust и connectivity zones.
