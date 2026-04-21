# Secrets Delivery Options

Главный practical вопрос не "где лежит секрет", а:
- кто может его прочитать;
- как он попадает в runtime;
- где он может случайно утечь;
- как его ротировать.

## Содержание

- [Что считается секретом](#что-считается-секретом)
- [Чего делать не стоит](#чего-делать-не-стоит)
- [Основные способы доставки](#основные-способы-доставки)
- [Быстрый выбор](#быстрый-выбор)
- [Важный practical rule](#важный-practical-rule)
- [Что могут спросить на интервью](#что-могут-спросить-на-интервью)

## Что считается секретом

Обычно это:
- DB passwords;
- API keys;
- JWT signing keys;
- broker credentials;
- cloud access keys;
- TLS private keys;
- webhook secrets.

## Чего делать не стоит

Плохие варианты:
- хардкодить секреты в исходники;
- коммитить `.env` с реальными значениями;
- зашивать секреты в Docker image;
- писать секреты в логи;
- отдавать один и тот же shared secret всем средам.

## Основные способы доставки

### 1. Environment variables

Примеры:
- `DB_PASSWORD`
- `JWT_SECRET`
- `REDIS_PASSWORD`

Плюсы:
- очень просто;
- хорошо поддерживается всеми платформами;
- удобно для 12-factor style конфигурации.

Минусы:
- секреты часто светятся в process environment;
- могут попасть в debug output, crash dumps, tooling и accidental logs;
- плохо подходят для больших multiline secrets, cert bundles и key files;
- ротация сложнее, если приложение читает значение только на старте.

Практически:
- для небольших runtime secrets это нормальный вариант;
- но не надо считать `env` автоматически "безопасным".

### 2. Secret files / mounted files

Примеры:
- TLS key/cert как файлы;
- cloud credentials json;
- mounted secret volume;
- docker/k8s secrets as files.

Плюсы:
- лучше для multiline content;
- удобнее для certs и private keys;
- приложение может читать секреты из конкретных файлов, а не из process env.

Минусы:
- чуть сложнее wiring;
- нужно аккуратно управлять путями, правами и lifecycle файлов.

### 3. External secret manager

Примеры:
- `HashiCorp Vault`
- `AWS Secrets Manager`
- `AWS SSM Parameter Store`
- `Google Secret Manager`
- `1Password Secrets Automation`

Плюсы:
- централизованное хранение;
- аудит доступа;
- лучше с ротацией;
- нормальный контроль прав.

Минусы:
- сложнее интеграция;
- нужен runtime access path;
- появляется зависимость от внешней системы.

### 4. Kubernetes Secret и производные

Примеры:
- `Secret`
- `External Secrets Operator`
- `Sealed Secrets`
- `SOPS` + GitOps flow

Это не отдельный тип секрета, а способ встроить secret delivery в k8s ecosystem.

## Быстрый выбор

Local dev:
- `.env.local`, `direnv`, локальный secret store, dev-only файлы.

Docker Compose:
- `env_file` для local dev;
- mounted secret files;
- не коммитить реальные значения.

Kubernetes:
- `Secret` как базовый минимум;
- при серьезной эксплуатации часто `External Secrets`, `Vault`, `SOPS`, `Sealed Secrets`.

Production outside k8s:
- чаще внешний secret manager + env/file injection при deploy.

## Важный practical rule

Build artifact должен быть отделен от секрета.

То есть:
- image собирается без production secrets;
- секреты подставляются на deploy/run time;
- одна и та же сборка едет в разные среды с разными secret values.

## Что могут спросить на интервью

- почему не стоит хранить секреты в репозитории;
- когда `env vars` нормальны, а когда уже нет;
- почему build и secret injection должны быть разделены;
- как ты бы организовал ротацию секретов;
- как уменьшить blast radius при утечке секрета.
