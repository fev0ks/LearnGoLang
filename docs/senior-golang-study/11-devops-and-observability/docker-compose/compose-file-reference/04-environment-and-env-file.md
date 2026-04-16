# Environment And Env File

`environment` и `env_file` определяют переменные окружения контейнера.

## `environment`

Можно задавать в двух формах.

### Map syntax

```yaml
environment:
  APP_ENV: local
  HTTP_ADDR: :8080
  LOG_JSON: "true"
```

### List syntax

```yaml
environment:
  - APP_ENV=local
  - HTTP_ADDR=:8080
  - LOG_JSON=true
```

Практически map syntax обычно удобнее:
- проще читать diff;
- меньше шансов ошибиться с quoting.

## Важные нюансы `environment`

Boolean-like значения лучше заключать в кавычки:

```yaml
environment:
  FEATURE_X: "true"
  DRY_RUN: "false"
```

Иначе YAML может интерпретировать их не как строки.

Переменная может быть указана без значения:

```yaml
environment:
  USER_INPUT:
```

или:

```yaml
environment:
  - USER_INPUT
```

Что это значит:
- Compose пытается взять значение из environment, в котором запускается сам `docker compose`;
- если значение не найдено, переменная будет unset в контейнере.

## `env_file`

`env_file` загружает env vars из одного или нескольких файлов.

### Простой вариант

```yaml
env_file:
  - .env.local
```

### Несколько файлов

```yaml
env_file:
  - ./base.env
  - ./override.env
```

Правило precedence:
- файлы обрабатываются сверху вниз;
- если одна и та же переменная есть в нескольких файлах, выигрывает последний.

## Расширенный синтаксис `env_file`

Элемент списка может быть объектом.

```yaml
env_file:
  - path: ./default.env
    required: true
  - path: ./override.env
    required: false
    format: raw
```

### Поля

`path`:
- путь к env-файлу.

`required`:
- `true`
- `false`

Если `required: false`:
- отсутствие файла не валит запуск.

`format`:
- по умолчанию обычный Compose env-file parser;
- `raw` отключает interpolation и передает значения как есть.

`raw` полезен, когда:
- значение содержит `$`;
- нужно сохранить quotes буквально;
- не хочется, чтобы Compose пытался интерпретировать содержимое.

## Формат env-файла

Каждая строка:

```text
VAR=VAL
VAR="VAL"
VAR='VAL'
VAR: VAL
```

Практические правила:
- `#` начинает комментарий;
- пустые строки игнорируются;
- unquoted и double-quoted значения проходят interpolation;
- один и тот же файл может содержать обычные `key=value` строки.

## Precedence между `environment` и `env_file`

Если заданы оба:
- `environment` имеет приоритет над `env_file`.

Пример:

```yaml
env_file:
  - .env.local
environment:
  APP_ENV: local
```

Если в `.env.local` есть `APP_ENV=dev`, контейнер получит `APP_ENV=local`.

## Важное различие: `.env` для Compose и `env_file` для контейнера

Это две разные истории.

`env_file`:
- задает env vars внутри контейнера.

Compose interpolation через `${VAR}`:
- влияет на сам compose-файл во время его рендера;
- берет значения из shell environment, `.env` и `--env-file`.

Практически команды часто путают эти два механизма.

## Когда что использовать

`environment`:
- 2-6 очевидных параметров прямо в compose;
- не хочется прыгать по дополнительным файлам.

`env_file`:
- много локальных настроек;
- есть `.env.local`, `.env.test`, `.env.compose`;
- хочется отделить значения от структуры compose-файла.

## Security note

Для local dev `env_file` нормален.

Но для чувствительных данных лучше помнить:
- env vars легко протекают в логи и debug output;
- production-like сценарии лучше рассматривать через mounted secret files или external secret manager.
