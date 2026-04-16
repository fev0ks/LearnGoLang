# Kubernetes Secrets And External Managers

## `Kubernetes Secret` как базовый минимум

В `Kubernetes` базовый способ передать секрет в Pod:
- объект `Secret`;
- потом инжект через env vars или mounted files.

Это useful baseline, но важно понимать ограничения:
- `Secret` в k8s не равен полноценному secret manager;
- сам по себе это еще не стратегия ротации и аудита;
- безопасность зависит от RBAC, etcd encryption, доступа к namespace и cluster policy.

## Как секрет попадает в Pod

Обычно двумя способами.

### Через env vars

Плюсы:
- просто;
- удобно для небольших значений.

Минусы:
- process environment;
- не лучший вариант для certs/keys;
- приложение обычно читает значение только на старте.

### Через mounted volume

Плюсы:
- удобно для файлов;
- лучше для certs, keys, multiline secrets;
- часто более практично для runtime file-based конфигурации.

## Когда обычного `Secret` уже мало

Если нужны:
- централизованное хранение;
- rotation workflows;
- audit access;
- GitOps-friendly управление без хранения plaintext secrets в git;

то обычно добавляют другой слой.

## Частые варианты

### `External Secrets Operator`

Идея:
- секрет хранится во внешнем manager;
- в кластер подтягивается автоматически.

Подходит, когда:
- есть `AWS Secrets Manager`, `SSM`, `Google Secret Manager`, `Vault`;
- хочется declarative sync в k8s.

### `Sealed Secrets`

Идея:
- можно хранить encrypted secret manifest в git;
- расшифровка происходит уже в кластере.

Подходит, когда:
- нужен GitOps flow;
- команда хочет encrypted secrets в repo вместо plaintext.

### `SOPS`

Идея:
- секреты шифруются и хранятся в git;
- расшифровка идет через KMS/PGP/age-based workflow.

Подходит, когда:
- команда уже живет в GitOps;
- нужен более контролируемый encrypted-manifest подход.

### `Vault`

Полезен, когда:
- нужна сильная централизованная secret platform;
- важны lease, dynamic secrets, audit, short-lived credentials.

## Practical rule

Маленький или несложный кластер:
- `Kubernetes Secret` + нормальный RBAC + etcd encryption + file/env injection может быть достаточным минимумом.

Серьезная production эксплуатация:
- часто уже лучше `External Secrets`, `Vault`, `SOPS` или `Sealed Secrets`.

## Что важно уметь сказать на интервью

- `Kubernetes Secret` это useful primitive, но не вся secret strategy;
- env injection и file mounts решают разные задачи;
- GitOps требует отдельного подхода к encrypted secrets;
- внешний secret manager обычно лучше для централизованной ротации и аудита.
