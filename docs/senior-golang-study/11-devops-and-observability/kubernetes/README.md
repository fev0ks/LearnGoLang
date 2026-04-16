# Kubernetes

Этот подпакет про практическую сторону `Kubernetes` для backend-разработчика: не "как стать cluster admin", а как понимать деплой, отказоустойчивость и runtime-поведение сервиса.

Как читать:
- сначала понять, какие проблемы `Kubernetes` решает по сравнению с Docker-only;
- затем пройти базовые сущности;
- после этого посмотреть failover, rollout и работу с конфигами.

Материалы:
- [Kubernetes Basics For Backend](./kubernetes-basics-for-backend.md)
- [Core Objects And Deployment Flow](./core-objects-and-deployment-flow.md)
- [Node Failure, Rollout And Config Delivery](./node-failure-rollout-and-config-delivery.md)
- [Pod vs Container](./pod-vs-container.md)

Что важно уметь объяснить:
- зачем `Kubernetes` вообще нужен поверх контейнеров;
- как связаны `Deployment`, `ReplicaSet`, `Pod` и `Service`;
- что происходит при падении ноды;
- как деплоить конфиги и секреты без ручной боли;
- когда `Kubernetes` полезен, а когда избыточен.
