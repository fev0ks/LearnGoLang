# Top-Level Structure

Compose file обычно состоит из нескольких top-level секций. Не все из них нужны в каждом проекте, но именно на этом уровне задается модель всего локального стека.

## Минимальный каркас

```yaml
name: shortener

services:
  api:
    build: .

networks:
  default:
    driver: bridge

volumes:
  pg-data:
```

## Основные top-level keys

### `services`

Главная секция compose-файла.

Здесь описываются runtime-роли:
- `api`
- `worker`
- `postgres`
- `redis`
- `migrator`

Именно `services` отвечает за:
- какой image использовать или как его собрать;
- какую команду запустить;
- какие env vars, порты, volumes и networks выдать контейнеру;
- от каких dependency сервис зависит.

Подробно:
- [Service Definition](./02-service-definition.md)

### `networks`

Top-level `networks` описывает именованные сети, к которым потом подключаются сервисы.

Это нужно, чтобы:
- контейнеры видели друг друга по DNS-именам;
- один compose project был изолирован от другого;
- при необходимости можно было сделать public/internal split.

Подробно:
- [Networks](./08-networks.md)

### `volumes`

Top-level `volumes` описывает именованные volumes, которыми Docker управляет сам.

Типичные use cases:
- `postgres` data;
- `redis` data;
- persist state между перезапусками контейнеров.

Подробно:
- [Volumes](./09-volumes.md)

### `configs`

Top-level `configs` описывает конфигурационные файлы, которые монтируются в контейнер как files.

Это useful, когда:
- конфиг хочется доставлять как файл, а не как env vars;
- один и тот же config нужен нескольким сервисам;
- нужен более декларативный способ конфигурации, чем bind mount.

В local dev встречается реже, чем `env_file` и обычный bind mount, но знать секцию полезно.

Подробно:
- [Configs And Secrets](./10-configs-and-secrets.md)

### `secrets`

Top-level `secrets` описывает чувствительные данные, к которым сервис должен получить явный доступ.

Практически:
- для simple local dev многие команды все еще используют `.env.local`;
- но Compose `secrets` полезны, когда пароль или токен хочется доставлять через file mount, а не через env.

Подробно:
- [Configs And Secrets](./10-configs-and-secrets.md)

### `name`

`name` задает имя Compose project.

Это влияет на:
- namespacing ресурсов;
- имена сетей;
- имена volume;
- имена контейнеров по умолчанию.

Если `name` не задан:
- Compose обычно использует имя директории;
- его можно переопределить через `docker compose -p ...` или `COMPOSE_PROJECT_NAME`.

### `version`

Top-level `version` исторически встречается в старых примерах, но для Compose v2 считается устаревшим.

Практическое правило:
- новые compose-файлы лучше писать без `version`;
- Compose все равно валидирует файл по актуальной Compose Specification и показывает warning, если `version` указан.

## Что важно помнить

Профили влияют только на services:
- сервисы с `profiles` могут быть выключены по умолчанию;
- top-level `networks`, `volumes`, `configs`, `secrets` сами по себе от `profiles` не отключаются.

## Practical map

Если упростить:
- `services` описывает роли и контейнеры;
- `networks` описывает связность;
- `volumes` описывает сохранение данных;
- `configs` и `secrets` описывают delivery config/secret data;
- `name` задает project scope.
