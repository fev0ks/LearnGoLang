# Стоимость AWS: расчёт, контроль и архитектурные решения

## Содержание

- [Ментальная модель счёта](#ментальная-модель-счёта)
- [С чего начинать анализ](#с-чего-начинать-анализ)
- [Как привязать расход к владельцу](#как-привязать-расход-к-владельцу)
- [Основные модели оплаты](#основные-модели-оплаты)
- [Стоимость вычислений](#стоимость-вычислений)
- [Reserved Instances и Savings Plans](#reserved-instances-и-savings-plans)
- [Spot Instances](#spot-instances)
- [Стоимость хранилищ и логов](#стоимость-хранилищ-и-логов)
- [Стоимость сети](#стоимость-сети)
- [Budgets, поиск аномалий и защитные ограничения](#budgets-поиск-аномалий-и-защитные-ограничения)
- [Три проверяемых расчёта](#три-проверяемых-расчёта)
- [Порядок оптимизации](#порядок-оптимизации)
- [Типичные ошибки](#типичные-ошибки)
- [Практический чек-лист](#практический-чек-лист)
- [Interview-ready answer](#interview-ready-answer)
- [Официальная документация](#официальная-документация)

Cloud cost — инженерное свойство системы. Архитектура определяет не только CPU и
память, но и количество копий данных, межзональный traffic, число requests,
retention logs и скорость автоматического масштабирования.

Эта статья не является прайс-листом. Цены AWS зависят от региона и меняются.
Числа ниже используются только в явно обозначенных примерах с датой и
допущениями; для production-решения их пересчитывают через актуальную pricing page
и AWS Pricing Calculator.

---

## Ментальная модель счёта

Счёт складывается из нескольких независимых meters:

```text
monthly cost
    = allocated capacity
    + consumed requests and runtime
    + stored data over time
    + transferred and processed bytes
    + managed-service control resources
    + support and marketplace charges
```

Один пользовательский request может затронуть сразу несколько строк:

```text
Client
  │ internet data transfer out
  ▼
CloudFront / ALB
  │ request + processed bytes
  ▼
ECS task
  ├── compute capacity
  ├── CloudWatch log ingestion
  ├── cross-AZ request to RDS
  └── NAT Gateway call to external API
```

Поэтому фраза «сервис почти не использует CPU» ничего не говорит о полном счёте.
Расход может находиться в egress, NAT processing, logs, storage или неиспользуемой
database capacity.

### Средняя нагрузка, пик и failure capacity

Для разных решений нужны разные числа:

- **Средняя нагрузка** определяет накопление данных и базовый объём потребления.
- **Пиковая нагрузка** определяет autoscaling, quotas и максимальную capacity.
- **Failure capacity** показывает, выдержит ли система пик после потери AZ, node
  group или dependency.

Если сервис обычно использует 40 instances, на пике 80, а после потери одной из
трёх AZ оставшиеся зоны должны принять traffic, commitment на 80 instances не
следует автоматически из peak. Сначала определяют длительность пика, минимальный
устойчивый baseline и допустимый риск недоиспользовать обязательство.

### Цена ресурса и цена сервиса

Цена одной EC2 instance — только часть total cost of ownership (TCO). В
self-hosted database входят инженеры on-call, upgrades, backups и failover tests.
Добавляется и стоимость ошибки. Managed RDS может быть дороже за час и дешевле
для команды целиком.

Обратное тоже возможно: managed abstraction не гарантирует экономию. Fargate или
Lambda удобны для variable load, но постоянно загруженный workload иногда дешевле
на правильно подобранной EC2 capacity. Это проверяется измерением, а не названием
сервиса.

---

## С чего начинать анализ

Оптимизация начинается не с замены instance family, а с локализации расхода.

### Шаг 1. Выбрать корректный cost view

Сначала фиксируют:

- период анализа;
- payer account или linked accounts;
- amortized или unblended cost;
- включение credits, refunds, taxes и support;
- granularity: месяц, день или час;
- scope: вся организация, environment, service или feature.

Для Reserved Instances и Savings Plans обычный unblended view может скрывать
экономический смысл upfront payment. Для сравнения workloads чаще нужен
amortized cost, который распределяет обязательство по сроку.

### Шаг 2. Разложить расход по измерениям

В AWS Cost Explorer последовательно смотрят:

1. Service.
2. Linked account.
3. Region.
4. Usage type и operation.
5. Cost allocation tag или Cost Category.

Рост `EC2-Other` может оказаться EBS или data transfer, а не compute instances.
Рост `DataTransfer-Regional-Bytes` указывает на другой flow, чем internet egress.
Название service без usage type часто недостаточно.

### Шаг 3. Сравнить cost с usage

Счёт объясняют рабочей метрикой:

- EC2 cost — instance-hours, vCPU-hours и utilization;
- RDS cost — instance-hours, storage, I/O и backup storage;
- S3 cost — GB-month, request count, retrieval и transfer;
- NAT Gateway cost — gateway-hours и processed GB;
- CloudWatch cost — ingested/stored/query-scanned bytes и custom metrics;
- Lambda cost — invocations, GB-seconds и provisioned concurrency.

Если стоимость выросла на 50%, а business traffic — на 5%, нужна причина:
изменился request size, cache hit ratio, retention, replica count или путь данных.

### Шаг 4. Найти дату изменения

Дневная granularity связывает cost spike с deployment, migration, load test или
настройкой retention. Для аномалии полезна последовательность:

```text
first expensive day
    → usage type
    → resource/account/tag
    → deployment or configuration change
    → owner and rollback/mitigation
```

Без даты легко оптимизировать давно существующий дешёвый ресурс вместо нового
дорогого flow.

---

## Как привязать расход к владельцу

### Accounts, tags и Cost Categories

Надёжная attribution строится слоями:

- account разделяет environment или крупную business boundary;
- tag связывает ресурс с `service`, `team`, `environment` и `cost_center`;
- Cost Category объединяет accounts, tags и charge types в финансовую модель.

Рекомендуемый минимальный набор tags:

```yaml
Environment: production
Service: orders-api
Team: commerce
CostCenter: cc-142
ManagedBy: terraform
```

User-defined cost allocation tag нужно отдельно активировать в Billing. До
активации tag на ресурсе не появляется в cost reports, а прошлые расходы не
переразмечаются автоматически.

Не все AWS resources поддерживают одинаковое tagging behavior. Политика
`Deny ec2:RunInstances` без корректного учёта resource types и create-time tags
может сломать запуск связанных volumes и network interfaces. Enforcement
тестируют на реальных API calls и дополняют Tag Policies, IaC checks и inventory.

### CUR 2.0 и Data Exports

Cost Explorer удобен для интерактивного анализа. Для детальных запросов,
внутреннего dashboard и регулярного allocation используют AWS Data Exports с Cost
and Usage Report 2.0 (CUR 2.0).

Типичный flow:

```text
AWS billing data
      │
      ▼
Data Exports / CUR 2.0 → S3 → Athena/BI → team and service reports
```

CUR содержит line items, pricing dimensions, reservations, Savings Plans и tags.
Это позволяет отвечать на вопросы, которых нет в готовом Cost Explorer view:
например, стоимость одного tenant или доля cross-AZ transfer конкретного сервиса.

### Unit economics

Абсолютный AWS bill растёт вместе с бизнесом. Для оценки эффективности полезнее
стоимость единицы:

```text
cost per order = monthly amortized service cost / completed orders
```

Если сервис стоит `$30,000/month` и обрабатывает `6,000,000` заказов:

```text
$30,000 / 6,000,000 = $0.005 per order
```

После роста до `$36,000/month` при `9,000,000` заказов:

```text
$36,000 / 9,000,000 = $0.004 per order
```

Счёт вырос на 20%, но стоимость заказа снизилась на 20%. Без unit economics это
можно ошибочно принять за регрессию.

---

## Основные модели оплаты

### Выделенная capacity

EC2, RDS, ElastiCache, NAT Gateway и load balancers имеют оплачиваемую capacity
или control resource, даже когда пользовательских requests нет. Остановка EC2
прекращает compute charge, но EBS volumes и public IPv4 могут продолжать
оплачиваться.

Для выделенной capacity основные рычаги:

- right-sizing;
- расписание выключения non-production;
- autoscaling;
- современная instance family и подходящая architecture;
- commitments для измеренного baseline;
- удаление idle resources.

### Pay per request или runtime

Lambda, S3, SQS, DynamoDB on-demand и другие services считают requests, runtime,
bytes или их комбинацию. У них меньше idle cost, но быстрый autoscaling способен
быстро увеличить счёт и перегрузить downstream.

Serverless cost оценивают по полной формуле:

```text
requests × price per request
+ runtime × allocated memory/CPU
+ data transfer
+ logs and traces
+ downstream operations
```

### Storage over time

`GB-month` — не разовая цена за загрузку. Если средний объём месяца равен 10 TB,
платёж зависит от времени, которое эти bytes хранятся в выбранном class. Lifecycle
и retention поэтому являются частью data model.

### Free Tier и credits

Модель AWS Free Tier изменилась для новых клиентов 15 июля 2025 года. Для новых
accounts действует credit-based free plan сроком до шести месяцев с суммой
кредитов до `$200`; для более старых accounts применяются legacy-условия. Всегда
проверяют дату создания account и страницу Billing, а не переносят в расчёт старое
правило про `t2.micro` на 12 месяцев.

Free Tier не является production cost model. После окончания credits архитектура
и traffic остаются, поэтому steady-state стоимость считают заранее.

---

## Стоимость вычислений

### Right-sizing без средних процентов

Правило `average CPU < 30% → уменьшить instance` опасно. Среднее скрывает короткие
пики, а CPU не показывает память, network, disk I/O и latency.

Перед уменьшением capacity проверяют:

- p95/p99 CPU и memory utilization;
- application latency и queueing;
- GC pauses и memory headroom;
- network packets/bytes и connection count;
- EBS throughput, IOPS и queue depth;
- пиковые и сезонные периоды;
- запас после потери AZ или instance;
- скорость scale-out и warm-up.

AWS Compute Optimizer использует lookback period, percentiles и настраиваемый
headroom. Рекомендация остаётся гипотезой: её проверяют canary deployment или load
test с production-like traffic.

### Семейство, поколение и architecture

Новое поколение instance часто улучшает price/performance, но универсального
процента нет. Результат зависит от CPU model, memory bandwidth, EBS/network limits,
compiler и access pattern.

Graviton может быть выгоден для Go-сервиса, потому что Go поддерживает `linux/arm64`.
Проверяют:

- наличие multi-architecture container images;
- CGO и native dependencies;
- профили CPU и latency на ARM;
- производительность encryption/compression;
- стоимость и доступность capacity в выбранном регионе.

Правильный вывод звучит не «ARM всегда дешевле», а «на benchmark этого workload
тип `c7g` даёт нужный SLO при меньшей amortized cost».

### Burstable instances

Семейства `t` используют CPU credits. Они подходят для нагрузки с низким baseline
и короткими bursts. В unlimited mode перерасход credits может оплачиваться
отдельно; в standard mode исчерпание credits ограничивает CPU.

Production web service может работать на `t` family, если credit balance и
unlimited charges наблюдаются, а профиль нагрузки действительно bursty. Правило
«production всегда на `m`» так же неточно, как правило «всё запускать на `t`».

### Autoscaling и downstream

Autoscaling снижает idle capacity, но не знает business constraints сам по себе.
Если Lambda или ECS быстро добавляет workers, RDS connection limit и partner API
могут исчерпаться раньше CPU.

Maximum capacity задают из downstream budget:

```text
max application replicas
    ≤ floor(database connection budget / pool size per replica)
```

Если database допускает 600 application connections, а pool одной replica имеет
20 connections:

```text
floor(600 / 20) = 30 replicas
```

Часть connection budget оставляют для migrations, admin access и failure mode,
поэтому фактический maximum будет меньше 30.

---

## Reserved Instances и Savings Plans

Commitment покупают после измерения baseline. Скидка не компенсирует
неиспользуемое обязательство.

### Различия моделей

| Модель | Что фиксируется | Гибкость | Capacity reservation |
| --- | --- | --- | --- |
| Compute Savings Plans | `$ / hour` | EC2, Fargate, Lambda | нет |
| EC2 Instance Savings Plans | family и region | size, OS, tenancy | нет |
| Regional RI | matching usage | AZ и часть size flexibility | нет |
| Zonal Reserved Instance | matching usage в одной AZ | меньше | да |

Standard RI нельзя обменять на другой offering. Convertible RI можно обменять на
другой Convertible RI с изменёнными атрибутами, включая instance family, type,
platform, scope или tenancy, если выполняются правила обмена. Упрощение «только
другой size в том же family» для Convertible RI неверно.

### Coverage и utilization

Две метрики отвечают на разные вопросы:

- **Coverage** — какая доля подходящего On-Demand usage покрыта commitment.
- **Utilization** — какая доля купленного commitment реально использована.

Высокая coverage при низкой utilization означает, что обязательство слишком
велико или workload изменился. Высокая utilization при низкой coverage означает,
что существующий commitment используется, но baseline может позволять осторожно
добавить покрытие.

### Как принимать решение

Безопасная последовательность:

1. Собрать несколько недель или месяцев репрезентативного usage.
2. Отделить стабильный baseline от peak и временных workloads.
3. Учесть запланированные migrations, Graviton, Fargate/Lambda и закрытие систем.
4. Посмотреть рекомендации Cost Explorer и пересчитать сценарии вручную.
5. Покрыть только ту часть baseline, потерю которой команда готова оплачивать весь
   срок.
6. Регулярно проверять coverage и utilization после покупки.

Стратегия `80% на три года all upfront` не является универсальной. Молодой
продукт, миграция между architectures или быстро меняющийся traffic оправдывают
меньшее покрытие и короткий срок, даже если номинальная скидка ниже.

### Break-even

Пусть reservation за весь срок стоит `R`, а On-Demand rate — `D` за час. Минимум
использованных часов для окупаемости:

```text
break-even hours = R / D
```

Если условная годовая reservation стоит `$500`, а On-Demand rate равен
`$0.10/hour`:

```text
$500 / $0.10 = 5,000 hours
5,000 / 8,760 ≈ 57% of the year
```

Это иллюстрация формулы, а не AWS quote. Реальный расчёт учитывает upfront,
recurring fee, normalization, eligible usage и альтернативную стоимость денег.

---

## Spot Instances

Spot использует свободную EC2 capacity со скидкой, но AWS может прервать instance.
Когда доступно предупреждение об interruption, оно приходит примерно за две
минуты. Rebalance Recommendation может прийти раньше, но не гарантируется перед
каждым interruption.

Spot подходит для:

- stateless queue consumers;
- CI runners;
- batch processing и transcoding;
- fault-tolerant distributed jobs;
- дополнительной capacity Auto Scaling Group или EKS node group.

Spot не подходит как единственная capacity для singleton stateful service,
database primary или job, который не умеет checkpoint/retry и обязан закончиться
к жёсткому deadline.

### Устойчивая схема

```text
Auto Scaling Group / EKS node group
├── On-Demand base capacity
└── Spot capacity
    ├── multiple instance families
    ├── multiple sizes
    ├── multiple Availability Zones
    └── capacity-optimized allocation
```

Приложение должно:

1. Перестать принимать новую работу.
2. Завершить, вернуть в очередь или checkpoint текущую работу.
3. Удалиться из load balancer targets.
4. Завершиться до принудительного interruption.

Вместо самодельного бесконечного polling через IMDSv1 используют поддерживаемый
IMDSv2 client, EventBridge events, lifecycle hooks и готовые controllers вроде
AWS Node Termination Handler для EKS. Поведение проверяют через AWS Fault
Injection Service, а не только читают в конфигурации.

Spot price не является главным сигналом доступности. Diversification и allocation
strategy уменьшают вероятность массового interruption лучше, чем поиск одного
самого дешёвого instance type.

---

## Стоимость хранилищ и логов

### S3

S3 считает несколько независимых составляющих:

- объём и время хранения;
- PUT/GET/LIST и другие requests;
- retrieval из холодных classes;
- minimum storage duration для отдельных classes;
- data transfer;
- replication и дополнительные features.

Lifecycle rule переводит objects между classes или удаляет их по возрасту. Rule
должно следовать access pattern: автоматический перевод через 30 дней в класс с
retrieval fee может увеличить счёт, если objects читаются каждую неделю.

Multipart upload разбивает один большой object на parts для parallel upload и
retry. Он не объединяет множество маленьких `PUT` и не является способом снизить
число requests. Незавершённые multipart uploads продолжают хранить parts, поэтому
для них задают lifecycle cleanup.

### EBS gp3

В gp3 baseline 3,000 IOPS и 125 MiB/s включён в storage price. Дополнительно
оплачиваются provisioned IOPS и throughput выше baseline. Формула для volume:

```text
gp3 cost
    = provisioned GiB
    + max(0, IOPS - 3,000)
    + max(0, throughput MiB/s - 125)
```

Коэффициенты зависят от региона. Перенос gp2 → gp3 часто полезен, но после него
проверяют, что provisioned performance соответствует реальному workload.

Остановленная EC2 instance продолжает хранить и оплачивать EBS volumes. Unattached
volume также оплачивается до удаления.

### EBS snapshots

Snapshots инкрементальны на уровне изменённых blocks. Число snapshots не равно
сумме полных размеров volumes: `100 snapshots × 1 TiB` не означает автоматически
100 TiB billable storage. Стоимость зависит от уникальных сохранённых blocks,
изменений данных и retention.

Lifecycle policy полезна, но перед удалением проверяют требования восстановления.
AWS управляет зависимостями инкрементальной цепочки: удаление промежуточного
snapshot не должно трактоваться как удаление всех данных следующих snapshots.

### CloudWatch Logs

Logs могут тарифицироваться за ingestion, storage, query scanning, delivery и
дополнительные features. Основные controls:

- structured logging вместо дублирования полного payload;
- level filtering и sampling;
- redaction secrets и personal data;
- retention вместо `Never expire`;
- metric из события вместо постоянного поиска по всем bytes;
- export в подходящее долгосрочное хранилище при необходимости.

High-cardinality поля полезны в logs, но создание отдельной custom metric для
каждого user или URL может породить большое число time series и cost.

---

## Стоимость сети

Сетевой расчёт начинают с направления каждого data flow:

```text
source → destination → path → GB/month → charge on each hop
```

Нельзя взять одну цену `$ per GB` и применить ко всей архитектуре. Internet
egress, cross-AZ, cross-region, NAT processing, Transit Gateway и CloudFront имеют
разные meters.

### Internet data transfer

На 2 сентября 2026 года AWS указывает 100 GB бесплатного data transfer out в
интернет в месяц, агрегированного по поддерживаемым сервисам и регионам, кроме
China и GovCloud. После этого действуют региональные и объёмные tiers.

Линейная оценка `1 PB × $0.09/GB = $90,000` полезна только как верхнеуровневая
арифметика с допущением flat rate. Реальный 1 PB попадает в несколько pricing
tiers и должен считаться по каждому диапазону.

Сжатие, pagination и image resizing уменьшают bytes независимо от cloud provider.
GraphQL не является автоматической экономией: плохо спроектированный query может
передать больше данных и создать больше backend work.

### Cross-AZ и cross-region

Трафик между Availability Zones одного региона часто оплачивается по обе стороны
или по service-specific rules. Расположение ECS task в AZ-a и RDS writer в AZ-b
может создать постоянный cross-AZ path.

Оптимизация не означает поместить всё в одну AZ. Multi-AZ availability имеет
ценность. Задача — увидеть traffic, локализовать chatty paths и принять осознанный
trade-off между cost и failure tolerance.

Cross-region replication добавляет transfer cost и хранение второй копии. Его
выбирают из RPO/RTO и data residency, а не потому, что «multi-region надёжнее» без
сценария переключения.

### NAT Gateway

NAT Gateway оплачивается за gateway-hours и processed bytes. Дополнительно может
возникнуть обычный internet или cross-AZ data transfer.

Способы уменьшить лишний NAT flow:

- gateway endpoints для S3 и DynamoDB;
- interface endpoints для подходящих services после отдельного расчёта;
- local-AZ NAT routing;
- IPv6 egress-only path для поддерживаемых destinations;
- private connectivity к partner или shared services;
- удаление ненужных внешних downloads из request path.

Keep-alive уменьшает connection setup, но NAT Gateway считает обработанные bytes.
Один egress proxy с keep-alive сам по себе не уменьшает byte-based processing
charge.

Перенос приложения в public subnet ради NAT cost — не универсальное решение. Он
меняет security model и добавляет public IPv4 charge.

### Public IPv4

С 1 февраля 2024 года AWS оплачивает все public IPv4 addresses, включая in-use и
idle Elastic IP. Поэтому проверяют не только unattached EIP, но и public addresses
EC2, managed services и другие выделенные IPv4.

IPv6 может уменьшить зависимость от public IPv4 и NAT44, но требует поддержки
clients, dependencies, security rules и observability. Это архитектурная миграция,
а не переключение одного billing flag.

### VPC Endpoints

Gateway endpoints для S3 и DynamoDB не имеют hourly/data processing charge. Они
часто убирают большой NAT path к этим services.

Interface endpoints создаются по AZ и оплачиваются за endpoint-hours и processed
data. Если endpoint используется редко, его fixed cost может быть выше NAT share.
Если через него проходят большие volumes или требуется private connectivity,
результат может быть обратным.

### CloudFront

CloudFront экономит origin traffic только для cacheable content с достаточным hit
ratio. Полная модель:

```text
CloudFront cost
    = viewer requests
    + viewer data transfer
    + origin requests on cache miss
    + origin data transfer under service-specific rules
    + optional invalidation/functions/security features
```

Если cache hit ratio равен 90%, origin получает примерно 10% cacheable requests.
Если response персонализирован и hit ratio близок к нулю, CloudFront добавляет
слой и не даёт ожидаемой экономии compute.

---

## Budgets, поиск аномалий и защитные ограничения

### AWS Budgets

Budget сравнивает actual или forecasted cost с threshold и отправляет
notification. Billing data обновляется с задержкой; AWS указывает обновление до
трёх раз в сутки с типичным интервалом 8–12 часов. За это время расход может
продолжить расти.

Budget не является универсальным жёстким лимитом. Budget Actions могут применить
IAM/SCP policy или остановить отдельные EC2/RDS resources, но такое действие надо
проектировать по workload: автоматическая остановка production database способна
создать более дорогой incident.

Команда настраивает как минимум:

- actual thresholds, например 50%, 80% и 100%;
- forecasted threshold;
- email/SNS destination с реальным владельцем;
- отдельные budgets для account, service или tag;
- runbook: кто проверяет usage type и что имеет право остановить.

CLI-команда `create-budget` создаёт alert только при передаче
`--notifications-with-subscribers`. Один `--budget` без subscribers создаёт бюджет
без email/SNS notification.

```bash
aws budgets create-budget \
  --account-id 123456789012 \
  --budget file://budget.json \
  --notifications-with-subscribers file://notifications.json
```

Файл `notifications.json` содержит threshold и получателя:

```json
[
  {
    "Notification": {
      "NotificationType": "ACTUAL",
      "ComparisonOperator": "GREATER_THAN",
      "Threshold": 80,
      "ThresholdType": "PERCENTAGE"
    },
    "Subscribers": [
      {
        "SubscriptionType": "EMAIL",
        "Address": "cloud-cost-owner@example.com"
      }
    ]
  }
]
```

Адрес в примере — placeholder. Для production используют рабочий group address
или SNS topic и подтверждают subscription.

### Cost Anomaly Detection

Cost Anomaly Detection ищет расход, отклоняющийся от исторического pattern. Он
дополняет fixed budget:

- budget отвечает «вышли ли мы за запланированный уровень»;
- anomaly detection отвечает «появилось ли необычное изменение».

Новый сервис может быть аномалией при маленьком общем bill, а постепенный рост
может превысить budget без резкого anomaly score. Нужны оба сигнала.

### Guardrails

Защита от runaway cost строится из нескольких ограничений:

- service quotas и quota alarms;
- maximum capacity в Auto Scaling, ECS и Lambda concurrency;
- rate limits и authentication на public endpoints;
- S3 Block Public Access;
- IAM/SCP restrictions на дорогие regions и resource types, если это допустимо;
- log retention и sampling;
- lifecycle для snapshots, images и objects;
- automatic cleanup временных environments;
- owner и TTL tags для экспериментальных ресурсов.

Guardrail не должен ломать recovery. Например, слишком низкая EC2 quota может
помешать scale-out после потери AZ.

---

## Три проверяемых расчёта

Все цены в этом разделе — допущения примера, а не обещание текущего тарифа.

### NAT Gateway для 10 TB в месяц

Допустим:

- два NAT Gateways для двух AZ;
- 730 часов в месяце;
- `$0.045/hour` за gateway;
- `$0.045/GB` processed data;
- 10 TB считаем как 10,000 GB;
- cross-AZ и internet transfer пока не включаем.

Hourly component:

```text
2 × 730 × $0.045 = $65.70/month
```

Data processing:

```text
10,000 GB × $0.045 = $450/month
```

Итого по двум meters:

```text
$65.70 + $450 = $515.70/month
```

Если все 10 TB идут в S3 через NAT, gateway endpoint может убрать `$450` NAT
processing. Hourly component останется, если NAT нужен для другого egress.

### 50 забытых EC2 instances на 90 дней

Допустим, условный On-Demand rate одной instance равен `$0.768/hour`:

```text
$0.768 × 50 × 24 × 90 = $82,944
```

Утверждение «примерно `$120,000`» нельзя вывести из этих данных без дополнительных
расходов или другого региона. EBS, load balancers, control plane, support и data
transfer надо перечислить отдельно, а не прятать в итог.

### 5 TB logs в день

Допустим:

- 5 TB/day считаем как 5,000 GB/day;
- ingestion стоит условные `$0.50/GB`;
- месяц содержит 30 дней;
- tiers, storage и query cost не учитываем.

```text
5,000 GB/day × $0.50/GB = $2,500/day
$2,500/day × 30 days = $75,000/month
```

Арифметика воспроизводима, но результат меняется с регионом, pricing tier и
реальным количеством bytes. Поэтому число подписывают допущениями, а не называют
«известной историей» без источника.

---

## Порядок оптимизации

### 1. Остановить аномальный рост

Сначала ограничивают public abuse, runaway autoscaling, debug logging или
ошибочный data loop. Покупка Savings Plan во время incident закрепляет расход, но
не устраняет причину.

### 2. Удалить idle и забытое

Проверяют:

- stopped/unused EC2 и старые Auto Scaling Groups;
- unattached EBS volumes;
- старые snapshots и AMIs;
- idle load balancers, NAT Gateways и interface endpoints;
- public IPv4;
- non-production environments, работающие круглосуточно;
- старые RDS instances и replicas;
- ECR images и незавершённые S3 multipart uploads.

Удаление material resource требует owner confirmation и проверки restore/rollback.

### 3. Исправить retention и data flow

Lifecycle, log retention, cache hit ratio, cross-AZ traffic и NAT paths часто дают
экономию без изменения business capacity.

### 4. Выполнить right-sizing

Изменение instance проверяют по percentiles, SLO и failure capacity. Сначала
canary или часть fleet, затем весь service.

### 5. Улучшить architecture

К этому уровню относятся:

- queue для сглаживания peak;
- cache для дорогого повторного чтения;
- подходящий storage class;
- Graviton после benchmark;
- serverless для редкой нагрузки или allocated capacity для постоянной;
- VPC endpoints для измеренного NAT flow;
- CloudFront для cacheable content.

Архитектурная миграция имеет engineering cost. Экономия `$500/month` не всегда
окупает квартал разработки и новый operational risk.

### 6. Купить commitments

Reserved Instances и Savings Plans идут после удаления waste и выбора целевой
architecture. Иначе команда покупает обязательство на resource, который собирается
заменить.

---

## Типичные ошибки

### Точные цены без региона и даты

Строка `S3 = $0.023/GB-month` выглядит как контракт, хотя class, region, объём и
дата не указаны. В учебном материале сохраняют формулу и дают ссылку на pricing;
если число нужно для примера, рядом фиксируют допущения.

### Проценты «типичного AWS bill» как факт

У API, video platform, data warehouse и SaaS control plane разные cost profiles.
Универсальная таблица `compute 40%, network 20%` создаёт ложную точность. Сначала
смотрят фактический Cost Explorer/CUR конкретной системы.

### Commitments до right-sizing

Высокая скидка на завышенную instance всё равно оставляет waste. Сначала выбирают
целевой размер и architecture, затем покрывают устойчивый baseline.

### Average CPU как единственный сигнал

Низкий average может сочетаться с ежедневным latency spike. Нужны percentiles,
memory, I/O, SLO и запас на отказ.

### CloudFront перед всем

CloudFront помогает cacheable traffic и edge delivery. Dynamic private API с
нулевым hit ratio получает дополнительный слой без ожидаемой экономии.

### Public subnet вместо NAT

Это меняет attack surface и добавляет public IPv4. Сетевой путь выбирают по
security и traffic, а не только по одной строке NAT Gateway.

### Multi-cloud arbitrage по цене одного сервиса

Связка `AWS S3 + BigQuery` может добавить internet egress, две IAM models,
cross-cloud incident response и data consistency problems. Multi-cloud выбирают
по business requirement и полной стоимости пути данных.

### Budget как hard cap

Billing data приходит с задержкой, а alert не останавливает все ресурсы. Нужны
autoscaling bounds, quotas, IAM и runbook.

### Snapshot count как полный объём

EBS snapshots инкрементальны. Стоимость нельзя получить умножением числа snapshots
на полный размер volume; нужны changed blocks и retention.

### NAT proxy как экономия bytes

Keep-alive уменьшает connection overhead, но NAT Gateway тарифицирует обработанные
bytes. Proxy полезен для policy, observability или connection management, но не
доказывает снижение NAT data processing.

---

## Практический чек-лист

### Видимость расходов

- [ ] Cost Explorer настроен на корректный amortized/unblended view.
- [ ] Production и non-production разделены accounts или явными boundaries.
- [ ] Cost allocation tags активированы в Billing.
- [ ] CUR 2.0/Data Export доступен для детального анализа.
- [ ] Есть unit metric: cost per request, order, tenant или processed GB.

### Compute

- [ ] Idle EC2, RDS, ElastiCache и load balancers найдены.
- [ ] Right-sizing учитывает p95/p99, memory, I/O и failure capacity.
- [ ] Graviton или новое поколение проверено benchmark, а не только прайсом.
- [ ] Autoscaling maximum согласован с database и partner limits.
- [ ] Non-production имеет расписание или TTL cleanup.

### Commitments и Spot

- [ ] Savings Plans/RI покупаются только на измеренный baseline.
- [ ] Coverage и utilization проверяются регулярно.
- [ ] Planned migrations учтены до срока commitment.
- [ ] Spot распределён по families, sizes и AZ.
- [ ] Interruption flow протестирован, а On-Demand base соответствует SLO.

### Storage и network

- [ ] S3 lifecycle соответствует реальной частоте чтения.
- [ ] EBS gp3 baseline и extra IOPS/throughput рассчитаны отдельно.
- [ ] Snapshot retention основан на RPO/RTO и changed blocks.
- [ ] Internet, cross-AZ, cross-region и NAT traffic измерены отдельно.
- [ ] VPC endpoints сравнены с NAT по hourly и data processing cost.
- [ ] Public IPv4 inventory включает in-use и idle addresses.
- [ ] CloudFront используется там, где измерен cache hit ratio.

### Защита

- [ ] Budgets имеют actual и forecasted notifications с владельцем.
- [ ] Cost Anomaly Detection включён для основных scopes.
- [ ] Autoscaling и Lambda concurrency имеют безопасные bounds.
- [ ] Public endpoints защищены authentication и rate limiting.
- [ ] Logs имеют level, sampling, redaction и retention.
- [ ] Для cost incident существует runbook и право на mitigation.

---

## Interview-ready answer

**1. Из чего складывается AWS bill?**

- Capacity — EC2, RDS, caches, load balancers и NAT оплачиваются во времени.
- Consumption — Lambda, requests, I/O и messages зависят от использования.
- Data — storage, retrieval, replication и retention оплачиваются отдельно.
- Network — internet, cross-AZ, cross-region и managed transit имеют разные meters.
- Operations — logs, metrics, backups и managed features тоже входят в
  архитектурную стоимость.

**2. Как правильно выполнять right-sizing?**

- Метрики — используют p95/p99 CPU и памяти, I/O, network и application
  latency.
- Период — захватывают пики, сезонность и failure mode, а не один спокойный день.
- Headroom — оставляют запас на рост и потерю части capacity.
- Проверка — рекомендацию подтверждают canary или load test перед массовым
  уменьшением.

**3. Чем Savings Plans отличаются от Reserved Instances?**

- Savings Plans — фиксируют почасовое commitment и дают разную гибкость по
  compute usage.
- Regional RI — применяет discount к matching EC2 usage и может давать AZ/size
  flexibility.
- Zonal RI — дополнительно резервирует capacity в конкретной AZ.
- Решение — принимают по baseline, сроку, planned migrations, coverage и utilization.

**4. Когда использовать Spot?**

- Подходящая работа — stateless, retryable и checkpointable workload.
- Устойчивость — несколько instance types и AZ уменьшают зависимость от одного
  capacity pool.
- Base capacity — критичная часть остаётся на On-Demand или другой
  гарантированной модели.
- Interruption — draining/checkpoint flow тестируется заранее.

**5. Почему NAT Gateway часто создаёт неожиданный счёт?**

- Два meters — оплачиваются gateway-hours и processed bytes.
- Дополнительный путь — отдельно могут добавиться cross-AZ и internet transfer.
- AWS services — traffic к S3/DynamoDB через NAT можно часто вывести в gateway endpoints.
- Ограничение — keep-alive не отменяет byte-based processing charge.

**6. Является ли AWS Budget жёстким лимитом?**

- Нет — Budget сравнивает billing data с threshold и отправляет notification с
  задержкой обновления данных.
- Actions — отдельные budget actions могут ограничить часть ресурсов, но требуют
  безопасного design.
- Guardrails — реальную защиту дополняют quotas, autoscaling bounds, IAM, rate
  limits и incident runbook.

**7. В каком порядке оптимизировать AWS cost?**

- Сначала incident — останавливают аномальный рост и public abuse.
- Затем waste — удаляют idle resources и исправляют retention/data paths.
- Потом efficiency — выполняют right-sizing и benchmark architecture.
- В конце commitment — покупают Savings Plans/RI на уже очищенный устойчивый baseline.

---

## Официальная документация

- [AWS Pricing Calculator](https://calculator.aws/)
- [AWS Free Tier FAQ](https://aws.amazon.com/free/free-tier-faqs/)
- [AWS Cost Explorer](https://docs.aws.amazon.com/cost-management/latest/userguide/ce-what-is.html)
- [AWS Data Exports and CUR 2.0](https://docs.aws.amazon.com/cur/latest/userguide/what-is-data-exports.html)
- [Cost allocation tags](https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/cost-alloc-tags.html)
- [AWS Budgets](https://docs.aws.amazon.com/cost-management/latest/userguide/budgets-managing-costs.html)
- [AWS Budgets best practices](https://docs.aws.amazon.com/cost-management/latest/userguide/budgets-best-practices.html)
- [AWS Cost Anomaly Detection](https://docs.aws.amazon.com/cost-management/latest/userguide/manage-ad.html)
- [AWS Compute Optimizer](https://docs.aws.amazon.com/compute-optimizer/latest/ug/what-is-compute-optimizer.html)
- [Compute Optimizer rightsizing preferences](https://docs.aws.amazon.com/compute-optimizer/latest/ug/rightsizing-preferences.html)
- [Savings Plans](https://docs.aws.amazon.com/savingsplans/latest/userguide/what-is-savings-plans.html)
- [Reserved Instance types](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/reserved-instances-types.html)
- [Reserved Instance discount application](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/apply_ri.html)
- [EC2 Spot best practices](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/spot-best-practices.html)
- [EC2 On-Demand and data transfer pricing](https://aws.amazon.com/ec2/pricing/on-demand/)
- [Amazon VPC pricing](https://aws.amazon.com/vpc/pricing/)
- [Amazon EBS gp3](https://docs.aws.amazon.com/ebs/latest/userguide/general-purpose.html)
- [Amazon EBS pricing](https://aws.amazon.com/ebs/pricing/)
- [Amazon S3 pricing](https://aws.amazon.com/s3/pricing/)
- [Amazon CloudWatch pricing](https://aws.amazon.com/cloudwatch/pricing/)
- [Cost Optimization Pillar](https://docs.aws.amazon.com/wellarchitected/latest/cost-optimization-pillar/welcome.html)
