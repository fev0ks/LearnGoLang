# System Design

Здесь собирай design-разборы и шаблоны ответа на system design интервью.

Основные блоки:
- сбор требований и оценка нагрузки;
- SLA, SLO, latency budget;
- stateless vs stateful components;
- кэширование, rate limiting, backpressure;
- consistency, replication, partitioning;
- HA, failover, disaster recovery;
- idempotency и duplicate handling;
- observability как часть дизайна, а не постфактум.

Подпакеты:
- [Reliability Patterns](./reliability-patterns/README.md) — как сервис переживает нагрузку и не превращает чужую поломку в свою. Семь механизмов в коде, выстроенных по уровням защиты: таймаут ограничивает одно ожидание, повтор компенсирует кратковременный сбой, предохранитель прекращает бессмысленные попытки, ограничение частоты и отбрасывание защищают ёмкость, идемпотентность делает повторы безопасными, переборка не даёт одной зависимости занять все ресурсы. Плюс три практики вокруг кода — SLO и бюджет ошибок, постмортемы, chaos engineering
- [External Request Flows](./external-request-flows/README.md) — путь одного внешнего запроса через слои системы: edge, gateway, аутентификация, сервисы, кэш, БД, очереди, объектное хранилище. Отдельно разобраны read-heavy путь через CDN, асинхронная запись через очередь, загрузка файла и место, где появляются задержки и отказы. Нужен, чтобы отвечать на «что происходит после того, как клиент нажал кнопку» не общими словами, а по слоям
- [Experimentation And Feature Rollouts](./experimentation-and-feature-rollouts/README.md) — как менять продукт без релиза вслепую: feature flags, постепенная раскатка, canary, dark launch и A/B-тесты. Тема попадает в system design интервью потому, что задевает сразу несколько слоёв — сервис назначения вариантов, кэш флагов на клиенте, метрики, аналитику и откат. Есть разбор реализации клиента флагов на Go
- [Interview Cases](./interview-cases/README.md) — два десятка разборов конкретных задач (сокращатель ссылок, чат, платежи, маркетплейс, живые трансляции) в формате симуляции интервью. Каждый проходит все фазы — уточнение требований, оценка нагрузки, архитектура, deep dive, финальное резюме — и обосновывает решения числами, а не общими словами. Нужен, чтобы отрабатывать порядок рассуждения под таймер, а не заучивать готовые ответы; там же общий фреймворк и тайминг интервью

## Highload как отдельная тема

- [Highload Design Patterns](./highload-design-patterns.md) — стены на каждом уровне нагрузки (процессор, память, БД, сеть, координация, эксплуатация, стоимость), стратегии шардирования, горячие ключи, fan-out, backpressure, разбор реальных историй Twitter, Discord, WhatsApp, Stripe и Cloudflare

## Подборка

- [Google SRE Resources](https://sre.google/resources/)
- [The Site Reliability Workbook](https://sre.google/workbook/preface/)
- [AWS Well-Architected Framework](https://docs.aws.amazon.com/wellarchitected/latest/framework/welcome.html)
- [Azure Cloud Design Patterns](https://learn.microsoft.com/en-us/azure/architecture/patterns/)
- [Kubernetes Concepts](https://kubernetes.io/docs/concepts/index.html)

## Вопросы

- какие требования уточнить первыми до того, как рисовать архитектуру;
- где в системе будут single points of failure;
- как изменится дизайн при росте в 10 раз по write traffic;
- где уместен cache, а где он ломает consistency;
- как защитить систему от retry storm и thundering herd;
- какие метрики и алерты нужны уже в первой версии;
- как объяснить выбор именно такого storage и messaging layer.
