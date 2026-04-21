# Kubernetes

Этот раздел про практическую сторону `Kubernetes` для backend-разработчика: не "как стать cluster admin", а как понимать деплой, отказоустойчивость и runtime-поведение Go-сервиса.

## Как читать

1. Сначала понять, какие проблемы `Kubernetes` решает по сравнению с Docker-only.
2. Разобраться с базовыми сущностями: Pod, Deployment, Service, ConfigMap.
3. Понять failover, rolling update и работу с конфигами.
4. Изучить probes и graceful shutdown — Go-специфичная часть, которую спрашивают на интервью.

## Материалы

- [Kubernetes: зачем нужен backend-разработчику](./kubernetes-basics-for-backend.md)
- [Pod vs Container](./pod-vs-container.md)
- [Core Objects And Deployment Flow](./core-objects-and-deployment-flow.md)
- [Node Failure, Rollout And Config Delivery](./node-failure-rollout-and-config-delivery.md)
- [Probes и Graceful Shutdown в Go](./probes-and-graceful-shutdown.md)

## Что важно уметь объяснить

- зачем `Kubernetes` нужен поверх контейнеров и когда он избыточен;
- как связаны `Deployment`, `ReplicaSet`, `Pod` и `Service`;
- что происходит при падении ноды и почему нужно несколько реплик на разных нодах;
- в чем разница между readiness и liveness probe и что будет если их перепутать;
- как реализовать graceful shutdown в Go при получении SIGTERM;
- как `resources.requests/limits` влияют на Go runtime (GOMEMLIMIT, GOMAXPROCS);
- как CI/CD должен обращаться с image, ConfigMap и rollout.
