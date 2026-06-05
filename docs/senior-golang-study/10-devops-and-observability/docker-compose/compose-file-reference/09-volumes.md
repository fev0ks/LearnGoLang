# Volumes

`volumes` отвечают за данные, которые не должны жить только в эфемерном слое контейнера.

## Содержание

- [Два главных варианта](#два-главных-варианта)
- [Top-level volume attributes](#top-level-volume-attributes)
- [Service-level `volumes`](#service-level-volumes)
- [`volumes_from`](#volumes_from)
- [Practical rules](#practical-rules)

## Два главных варианта

### Bind mount

```yaml
services:
  api:
    volumes:
      - .:/app
```

Что это значит:
- папка с хоста монтируется внутрь контейнера.

Подходит для:
- local development;
- hot reload;
- mounted config files.

### Named volume

```yaml
services:
  postgres:
    volumes:
      - pg-data:/var/lib/postgresql/data

volumes:
  pg-data:
```

Что это значит:
- Docker сам управляет хранилищем;
- данные живут между перезапусками и пересозданием контейнеров.

Подходит для:
- Postgres data;
- Redis data;
- persistent local state.

## Top-level volume attributes

### `driver`

```yaml
driver: local
```

Определяет volume driver.

### `driver_opts`

Опции для volume driver.

Пример:

```yaml
driver_opts:
  type: "nfs"
  o: "addr=10.40.0.199,nolock,soft,rw"
  device: ":/docker/example"
```

### `external`

```yaml
external: true
```

Что значит:
- volume уже существует вне жизненного цикла Compose application;
- Compose не создает его сам.

Важно:
- при `external: true` остальные атрибуты, кроме `name`, обычно не имеют смысла.

### `labels`

Metadata для volume resource.

### `name`

```yaml
name: my-app-data
```

Позволяет явно задать реальное имя volume.

Полезно:
- для интеграции с уже существующими volume;
- для parameterized naming.

## Service-level `volumes`

На уровне сервиса `volumes` описывает mount points.

```yaml
services:
  api:
    volumes:
      - .:/app
      - pg-data:/var/lib/postgresql/data
```

### Short syntax

Формат:

```yaml
VOLUME:CONTAINER_PATH
VOLUME:CONTAINER_PATH:ACCESS_MODE
```

Где:
- `VOLUME` это либо host path, либо volume name;
- `CONTAINER_PATH` это путь внутри контейнера;
- `ACCESS_MODE` это список опций через запятую.

Основные `ACCESS_MODE` значения:
- `rw`
- `ro`
- `z`
- `Z`

Что они значат:
- `rw` read-write, default;
- `ro` read-only;
- `z` и `Z` это SELinux re-labeling опции для bind mounts.

Практические нюансы:
- относительные host paths лучше начинать с `.` или `..`;
- short syntax для bind mount по legacy-поведению может создать host directory автоматически, если его нет.

### Long syntax

Long syntax нужен, когда короткой формы уже не хватает.

Пример:

```yaml
services:
  api:
    volumes:
      - type: bind
        source: .
        target: /app
        read_only: false
        bind:
          create_host_path: false
```

#### Основные поля

`type`:
- `volume`
- `bind`
- `tmpfs`
- `image`
- `npipe`
- `cluster`

`source`:
- host path для bind mount;
- volume name для named volume;
- image reference для image mount.

`target`:
- путь внутри контейнера.

`read_only`:
- `true`
- `false`

#### Дополнительные поля по типам

`bind`:
- `propagation`
- `create_host_path`
- `selinux`

`create_host_path`:
- `true`
- `false`

`selinux`:
- `z`
- `Z`

`volume`:
- `nocopy`
- `subpath`

`nocopy`:
- `true`
- `false`

`tmpfs`:
- `size`
- `mode`

`image`:
- `subpath`

`consistency`:
- platform-specific значение для consistency policy.

## `volumes_from`

Есть еще `volumes_from`, который монтирует все volumes другого service или контейнера.

Пример:

```yaml
volumes_from:
  - service_name
  - service_name:ro
  - container:container_name
  - container:container_name:rw
```

Практически:
- в современных compose-файлах это нужно редко;
- чаще лучше явно описать нужные mounts.

## Practical rules

- bind mounts чаще для кода и dev-конфигов;
- named volumes чаще для данных сервисов;
- для stateless Go API volume часто вообще не нужен;
- для Postgres и Redis без volume данные будут теряться при пересоздании контейнера.
