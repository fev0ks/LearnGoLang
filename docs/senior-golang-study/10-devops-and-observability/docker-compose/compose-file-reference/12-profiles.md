# Profiles

`profiles` позволяют делать часть сервисов опциональными.

## Базовый пример

```yaml
services:
  grafana:
    image: grafana/grafana
    profiles: ["observability"]
```

## Как это работает

Если сервису заданы `profiles`:
- он не поднимется по умолчанию без активации нужного профиля;
- его можно включить через CLI или явный запуск сервиса.

Допустимый формат имени профиля:
- начинается с буквы или цифры;
- дальше допускает буквы, цифры, `_`, `.` и `-`.

Запуск:

```bash
docker compose up
docker compose --profile observability up
COMPOSE_PROFILES=observability docker compose up
```

Можно включить несколько профилей:

```bash
docker compose --profile observability --profile debug up
COMPOSE_PROFILES=observability,debug docker compose up
```

Можно включить все профили:

```bash
docker compose --profile "*" up
```

## Что это дает

- один compose-файл может описывать несколько сценариев запуска;
- тяжелые сервисы можно не поднимать каждый раз;
- локальный стек становится гибче.

## Типичные use cases

- observability stack;
- debug tools;
- optional admin services;
- performance tooling;
- smoke и integration-only dependencies.

## Важные нюансы

Если ты явно таргетишь сервис с profile через CLI:
- профиль не нужно включать вручную;
- Compose поднимет сам этот сервис и его declared dependencies, но не все остальные сервисы с тем же профилем.

По текущей Docker Compose docs:
- ссылки на сервисы через `depends_on`, `links`, `extends` и похожие механизмы не включают profile автоматически, если этот сервис иначе был бы выключен;
- в некоторых комбинациях это может приводить к invalid model.

Top-level элементы от profiles не выключаются:
- `networks`
- `volumes`
- `configs`
- `secrets`

Практически это значит:
- profiles надо проектировать осознанно;
- optional сервис не должен внезапно становиться hard dependency для always-on сервиса без общего profile story.

## Practical rule

Хороший default:
- основные app services без profiles;
- optional observability и debug tooling через profiles.

Это обычно самый понятный local dev UX.
