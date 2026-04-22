# Kubernetes

Этот раздел про практическую сторону `Kubernetes` для backend-разработчика: не "как стать cluster admin", а как понимать деплой, отказоустойчивость и runtime-поведение Go-сервиса.

## Как читать

1. Сначала понять, какие проблемы `Kubernetes` решает по сравнению с Docker-only.
2. Разобраться с базовыми сущностями: Pod, Deployment, Service, ConfigMap.
3. Понять failover, rolling update и работу с конфигами.
4. Изучить probes и graceful shutdown — Go-специфичная часть, которую спрашивают на интервью.

## Материалы

- [01. Kubernetes: зачем нужен backend-разработчику](./01-kubernetes-basics-for-backend.md)
- [02. Core Objects And Deployment Flow](./02-core-objects-and-deployment-flow.md)
- [03. Pod vs Container](./03-pod-vs-container.md)
- [04. Probes и Graceful Shutdown в Go](./04-probes-and-graceful-shutdown.md)
- [05. Node Failure, Rollout And Config Delivery](./05-node-failure-rollout-and-config-delivery.md)
- [06. kubectl: команды с примерами](./06-kubectl-commands.md) — контексты/GKE, pods, logs, exec, deployments, port-forward, secrets, top, events, troubleshooting

## Что важно уметь объяснить

- зачем `Kubernetes` нужен поверх контейнеров и когда он избыточен;
- как связаны `Deployment`, `ReplicaSet`, `Pod` и `Service`;
- что происходит при падении ноды и почему нужно несколько реплик на разных нодах;
- в чем разница между readiness и liveness probe и что будет если их перепутать;
- как реализовать graceful shutdown в Go при получении SIGTERM;
- как `resources.requests/limits` влияют на Go runtime (GOMEMLIMIT, GOMAXPROCS);
- как CI/CD должен обращаться с image, ConfigMap и rollout.
