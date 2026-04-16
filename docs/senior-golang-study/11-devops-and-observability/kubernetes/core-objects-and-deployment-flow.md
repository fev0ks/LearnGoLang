# Core Objects And Deployment Flow

## Содержание

- [Базовая цепочка](#базовая-цепочка)
- [Pod](#pod)
- [Deployment](#deployment)
- [ReplicaSet](#replicaset)
- [Service](#service)
- [ConfigMap и Secret](#configmap-и-secret)
- [Как это выглядит как процесс](#как-это-выглядит-как-процесс)
- [Что важно проговорить на интервью](#что-важно-проговорить-на-интервью)

## Базовая цепочка

Когда ты деплоишь сервис в `Kubernetes`, обычно в голове должна быть такая схема:

```text
Deployment -> ReplicaSet -> Pod -> Containers
                      \
                       -> Service
```

## Pod

`Pod`:
- минимальная единица запуска;
- внутри одного Pod может быть один или несколько контейнеров;
- у контейнеров внутри Pod общий network namespace и lifecycle.

Практически:
- backend-сервис чаще всего живет в одном основном контейнере;
- дополнительные контейнеры в одном Pod нужны реже и обычно для sidecar-patterns.

## Deployment

`Deployment`:
- описывает желаемое состояние;
- управляет количеством реплик;
- делает rollout и rollback;
- следит, чтобы нужное количество Pod'ов оставалось запущенным.

Обычно именно `Deployment` это основной объект для stateless Go API.

## ReplicaSet

`ReplicaSet`:
- промежуточный слой, который реально держит нужное число Pod'ов;
- обычно ты им управляешь не напрямую, а через `Deployment`.

## Service

`Service`:
- дает стабильную точку доступа к Pod'ам;
- делает service discovery;
- балансирует трафик между репликами.

Практически это значит:
- Pod'ы могут пересоздаваться и менять IP;
- `Service` скрывает эту нестабильность от клиентов.

## ConfigMap и Secret

`ConfigMap`:
- non-secret конфигурация;
- feature flags, hostnames, режимы работы, app config.

`Secret`:
- чувствительные данные;
- tokens, passwords, DSN fragments, keys.

Практическое правило:
- конфиг и секреты не должны быть зашиты в image;
- image собирается отдельно от runtime-конфигурации.

## Как это выглядит как процесс

1. Собираешь Docker image.
2. Деплоишь `Deployment`.
3. `Deployment` создает `ReplicaSet`.
4. `ReplicaSet` создает `Pod`'ы.
5. `kubelet` на нодах запускает контейнеры.
6. `Service` дает стабильную точку доступа к Pod'ам.

## Что важно проговорить на интервью

- `Pod` это минимальная единица запуска;
- `Deployment` управляет rollout и числом реплик;
- `Service` дает стабильную сеть;
- `ConfigMap` и `Secret` отделяют runtime-конфиг от image.
