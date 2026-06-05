# Command Entrypoint And Restart

Эта группа ключей управляет тем, что именно контейнер запускает и как он завершается или перезапускается.

## Содержание

- [`command`](#command)
- [`entrypoint`](#entrypoint)
- [Когда менять `command`, а когда `entrypoint`](#когда-менять-command-а-когда-entrypoint)
- [`restart`](#restart)
- [`init`](#init)
- [`stop_signal`](#stop_signal)
- [`stop_grace_period`](#stop_grace_period)
- [`user`](#user)
- [`working_dir`](#working_dir)
- [`stdin_open` и `tty`](#stdin_open-и-tty)
- [Practical patterns](#practical-patterns)
- [Practical rules](#practical-rules)

## `command`

`command` переопределяет `CMD` из image.

### Строковая форма

```yaml
command: ./app --config /etc/app/config.yaml
```

### List form

```yaml
command: ["./app", "--config", "/etc/app/config.yaml"]
```

Практически list form обычно безопаснее:
- меньше quoting surprises;
- проще передавать аргументы.

Важные значения:

`null`:
- использовать default command из image.

`[]` или `''`:
- очистить default command image.

Нюанс:
- `command` не запускается автоматически через shell image;
- если тебе нужна shell semantics, пиши ее явно.

Пример:

```yaml
command: /bin/sh -c 'echo "hello $$HOSTNAME"'
```

## `entrypoint`

`entrypoint` переопределяет `ENTRYPOINT` из image.

Строковая форма:

```yaml
entrypoint: /docker-entrypoint.sh
```

List form:

```yaml
entrypoint: ["/docker-entrypoint.sh"]
```

Важные значения:

`null`:
- использовать default entrypoint из image.

`[]` или `''`:
- очистить entrypoint image.

Практический эффект:
- если `entrypoint` задан не `null`, Compose игнорирует default `CMD` image, пока ты явно не задашь `command`.

## Когда менять `command`, а когда `entrypoint`

Менять `command`:
- когда образ тот же, но аргументы или подкоманда другие;
- `api` и `worker` запускаются из одного образа;
- мигратор и приложение разделены только startup command.

Менять `entrypoint`:
- когда нужен другой startup script;
- надо перехватить init logic образа;
- нужен wrapper process.

## `restart`

`restart` определяет policy перезапуска контейнера.

Актуальные значения:

`no`:
- не перезапускать;
- default.

`always`:
- всегда перезапускать, пока контейнер не удален.

`on-failure`:
- перезапускать только если процесс завершился с ошибкой.

`on-failure:<max-retries>`:
- как `on-failure`, но с лимитом попыток.

`unless-stopped`:
- перезапускать, пока сервис явно не остановлен или не удален.

Примеры:

```yaml
restart: "no"
restart: always
restart: on-failure
restart: on-failure:3
restart: unless-stopped
```

Практический выбор:
- `migrator`: обычно `no`
- `api`, `worker`, `postgres`: часто `unless-stopped`
- сервис, который должен падать явно и заметно: иногда `on-failure`

## `init`

`init` включает небольшой init process как PID 1 внутри контейнера.

```yaml
init: true
```

Допустимые значения:
- `true`
- `false`

Зачем это нужно:
- корректная доставка сигналов;
- reaping дочерних процессов.

Полезно:
- если приложение или dev tooling спавнит subprocesses;
- если не хочешь странностей от "голого" PID 1 в контейнере.

## `stop_signal`

Какой signal Compose отправляет контейнеру при остановке.

Пример:

```yaml
stop_signal: SIGUSR1
```

Если не задан:
- обычно используется `SIGTERM`.

## `stop_grace_period`

Сколько ждать graceful shutdown до `SIGKILL`.

Примеры:

```yaml
stop_grace_period: 1s
stop_grace_period: 30s
stop_grace_period: 1m30s
```

Если не задан:
- default обычно 10 секунд.

Для Go-сервиса это важно, если:
- нужно успеть закрыть HTTP server;
- надо дожать in-flight requests;
- надо flush metrics, traces, producer buffers.

## `user`

Переопределяет пользователя, от которого запускается процесс.

```yaml
user: "10001:10001"
user: root
```

Полезно:
- когда mounted files имеют чувствительные права;
- когда образ должен работать без `root`.

## `working_dir`

Переопределяет рабочую директорию процесса.

```yaml
working_dir: /app
```

## `stdin_open` и `tty`

`stdin_open`:
- аналог `docker run -i`
- значения: `true` или `false`

`tty`:
- аналог `docker run -t`
- значения: `true` или `false`

Практически:
- чаще нужны для интерактивных debug и dev сценариев;
- production-like сервисам обычно не нужны.

## Practical patterns

API:

```yaml
command: ["./app"]
restart: unless-stopped
stop_grace_period: 30s
```

Worker:

```yaml
command: ["./app", "worker"]
restart: unless-stopped
```

Migrator:

```yaml
command: ["./app", "migrate", "up"]
restart: "no"
```

## Practical rules

- сначала старайся зафиксировать sane `ENTRYPOINT` и `CMD` в Dockerfile;
- в compose переопределяй только то, что реально меняется между ролями;
- one-shot jobs не стоит крутить с `always`;
- для Go-сервисов не забывай про `stop_grace_period`, если graceful shutdown тебе важен.
