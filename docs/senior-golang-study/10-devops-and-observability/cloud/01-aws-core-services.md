# AWS: практический обзор для backend-разработчика

## Содержание

- [Ментальная модель AWS](#ментальная-модель-aws)
- [С чего начать новый проект](#с-чего-начать-новый-проект)
- [Карта выбора сервисов](#карта-выбора-сервисов)
- [Запуск приложений](#запуск-приложений)
- [Хранение данных](#хранение-данных)
- [Асинхронная работа и интеграции](#асинхронная-работа-и-интеграции)
- [Сеть и внешний трафик](#сеть-и-внешний-трафик)
- [IAM, аутентификация и секреты](#iam-аутентификация-и-секреты)
- [Сборка и доставка](#сборка-и-доставка)
- [Наблюдаемость и аудит](#наблюдаемость-и-аудит)
- [Три типовые архитектуры](#три-типовые-архитектуры)
- [Стоимость и границы ответственности](#стоимость-и-границы-ответственности)
- [Типичные ошибки](#типичные-ошибки)
- [Практический чек-лист](#практический-чек-лист)
- [Interview-ready answer](#interview-ready-answer)
- [Официальная документация](#официальная-документация)

AWS — набор облачных сервисов, из которых команда собирает среду выполнения
приложения. Backend-разработчику не требуется помнить весь каталог продуктов.
Важнее уметь пройти от задачи к минимально сложной архитектуре, безопасно дать
приложению доступ и понимать, какая эксплуатационная ответственность остаётся у
команды.

Основной вопрос этой статьи: как запустить Go-сервис в AWS, подключить данные,
очереди и наблюдаемость, не превращая первый production deployment в собственную
облачную платформу.

---

## Ментальная модель AWS

### Account, Region и Availability Zone

**AWS account** — основная граница владения ресурсами, IAM, квотами и счётом.
Production и non-production часто разделяют по разным accounts, объединённым в
AWS Organizations. Это уменьшает blast radius и упрощает раздельный контроль
прав и расходов.

**Region** — географическая область, например `eu-west-1`. Большинство ресурсов
создаются в конкретном регионе: VPC, EC2 instance, RDS database, Lambda function.

**Availability Zone (AZ)** — изолированная локация внутри региона. Одна AZ может
состоять из одного или нескольких дата-центров. Зоны соединены низколатентной
сетью, но отказ AZ всё равно считается отдельным сценарием отказа.

```text
AWS Organization
└── production account
    └── Region: eu-west-1
        ├── AZ: eu-west-1a
        │   ├── public subnet
        │   └── private subnet
        ├── AZ: eu-west-1b
        │   ├── public subnet
        │   └── private subnet
        └── regional services: S3, SQS, DynamoDB, Lambda
```

Не все сервисы имеют одинаковую область действия. IAM является глобальным
сервисом аккаунта, VPC — региональным, subnet — зональным, а S3 bucket
создаётся в регионе. Имя bucket должно быть уникально в пределах AWS partition.

### Data plane и control plane

У облачного ресурса полезно различать два пути:

- **Control plane** — создание и изменение ресурса через Console, AWS CLI,
  Terraform или API: создать bucket, изменить security group, задать autoscaling.
- **Data plane** — рабочие запросы приложения: прочитать object из S3, записать
  item в DynamoDB, получить message из SQS.

Права deployer и runtime-приложения поэтому не совпадают. CI может обновлять ECS
service, но запущенному контейнеру обычно не нужно право изменять собственную
инфраструктуру. Приложению нужны только операции data plane на его ресурсах.

### Shared responsibility

Managed-сервис снимает часть работы, но не всю ответственность.

| Уровень | AWS обычно делает | Команда всё ещё решает |
| --- | --- | --- |
| EC2 | оборудование и гипервизор | ОС, patches, процессы, scaling |
| ECS/Fargate | размещение containers | image, task, scaling, rollout |
| RDS | СУБД, backups, часть failover | schema, queries, pools, RPO/RTO |
| S3 | durability и storage fleet | IAM, lifecycle, versioning |
| SQS | хранение и доставка | idempotency, retry, DLQ, alerts |

Слово `managed` означает изменение границы ответственности, а не отсутствие
архитектурных решений.

---

## С чего начать новый проект

До первого deployment полезно создать минимальный фундамент.

### 1. Разделить accounts и окружения

Для небольшого проекта достаточно как минимум отделить production от
non-production. В более крупной организации добавляют accounts для security,
centralized logging и shared services.

AWS Organizations и Service Control Policies (SCP) задают внешнюю границу: какие
действия вообще разрешены в дочернем account. SCP не выдаёт право сам по себе, а
ограничивает права, которые могут выдать IAM policies внутри account.

### 2. Настроить доступ людей через федерацию

Для сотрудников рекомендуемый путь — IAM Identity Center или внешний identity
provider. Пользователь получает временные credentials и assume role, а не
постоянный access key IAM user.

Root user нужен только для ограниченного набора account-level операций. Для него
включают MFA, не создают access keys и не используют в ежедневной работе.

### 3. Выбрать основной регион

Регион выбирают по нескольким факторам:

- близость к пользователям и зависимым системам;
- требования к размещению данных;
- доступность нужных сервисов и instance families;
- стоимость compute, storage и network transfer;
- disaster recovery strategy.

Самый дешёвый регион не обязательно даёт самый дешёвый сервис целиком. Если база
далеко от пользователей или соседней системы, экономия на instance может
превратиться в latency и сетевой счёт.

### 4. Включить audit и cost controls

До production traffic настраивают:

- CloudTrail для аудита API-вызовов;
- AWS Budgets и Cost Anomaly Detection;
- cost allocation tags или account-level attribution;
- владельца уведомлений и процедуру реакции;
- ограничения autoscaling и service quotas там, где бесконечный рост опасен.

Budget уведомляет о расходе, но не является жёстким spending cap. Подробнее
про контроль стоимости — в
[Cloud cost и архитектурные решения](./02-cloud-cost-and-architecture.md).

### 5. Управлять инфраструктурой как кодом

VPC, IAM roles, databases, queues и alarms лучше создавать через Terraform,
CloudFormation или AWS CDK. Воспроизводимая конфигурация позволяет сравнить
окружения, провести review и восстановить ресурс без ручного поиска console
settings.

CI/CD при этом обычно владеет версией приложения, а IaC — долгоживущей
инфраструктурой. Если Terraform и deployment pipeline одновременно меняют image
tag ECS service, инструменты начинают откатывать изменения друг друга.

---

## Карта выбора сервисов

| Задача | Отправная точка | Когда смотреть дальше |
| --- | --- | --- |
| Container API | ECS + Fargate | EKS для Kubernetes; EC2 для ОС |
| Короткий event handler | Lambda | ECS для долгой или постоянной нагрузки |
| Kubernetes workload | EKS | ECS, если Kubernetes API не нужен |
| Виртуальная машина | EC2 + ASG | ECS/EKS для containers |
| Объекты и файлы | S3 | EFS для POSIX; EBS для диска VM |
| SQL OLTP-база | RDS | Aurora для отдельных HA/scale требований |
| Key-value/document | DynamoDB | RDS для joins и гибких транзакций |
| Cache | ElastiCache | Durable DB, если значение нельзя потерять |
| Очередь задач | SQS | Kafka/MSK, если нужны replay и потоковая история |
| Fan-out | SNS + SQS | EventBridge для routing по правилам |
| Долгий workflow | Step Functions | Код для короткой атомарной операции |
| Секрет | Secrets Manager | Parameter Store для простой конфигурации |
| Метрики, логи, alarms | CloudWatch | OpenTelemetry и общий backend |

Таблица задаёт старт, а не окончательный ответ. Выбор меняют access patterns,
пиковая нагрузка, команда, RPO/RTO, compliance и стоимость отказа.

---

## Запуск приложений

### EC2

Amazon Elastic Compute Cloud (EC2) предоставляет виртуальные машины. Команда
выбирает AMI, instance type, диск, сеть и способ обновления ОС.

EC2 подходит, когда нужны:

- контроль операционной системы, kernel settings или специальных daemons;
- legacy software, которое трудно упаковать в managed runtime;
- GPU, специальные accelerator или local instance storage;
- предсказуемая постоянно загруженная машина;
- собственный container runtime или Kubernetes nodes.

Для production одну вручную созданную VM заменяют связкой:

```text
Launch Template
      │
      ▼
Auto Scaling Group across 2+ AZ
      │
      ▼
Application Load Balancer
```

Launch Template фиксирует AMI, instance type, IAM instance profile, storage и
network settings. Auto Scaling Group поддерживает нужное количество instances и
заменяет unhealthy nodes. ALB распределяет HTTP(S)-трафик.

Выбор семейства начинается с профиля нагрузки:

- `t` — burstable CPU для небольших и нерегулярных нагрузок;
- `m` — общий баланс CPU и памяти;
- `c` — CPU-intensive обработка;
- `r` — memory-intensive рабочая нагрузка;
- `i` и другие storage families — локальный высокий I/O;
- `g`, `p`, `inf`, `trn` — GPU и accelerators.

Размер instance нельзя выбирать только по среднему CPU. Проверяют p95/p99,
память, GC, network, disk I/O, latency приложения, сезонность и запас на отказ
части fleet.

### ECS и Fargate

Amazon Elastic Container Service (ECS) — AWS-native оркестратор контейнеров.

Основные сущности:

- **Task definition** — версия описания контейнеров, CPU, памяти, ports, roles и
  logging.
- **Task** — запущенный экземпляр task definition.
- **Service** — поддерживает заданное число tasks, выполняет rollout и связывает
  их с load balancer.
- **Cluster** — логическая группа capacity, на которой запускаются tasks.

Fargate предоставляет compute для ECS tasks без управления EC2 nodes. Команда
оплачивает запрошенные vCPU и память task, а AWS выбирает и обслуживает host.

Практический путь для обычного Go API:

```text
Container image in ECR
        │
        ▼
ECS service on Fargate across 2+ AZ
        │
        ▼
Application Load Balancer
        │
        ├── health checks
        ├── autoscaling by CPU/request count
        └── CloudWatch logs and metrics
```

ECS + Fargate — хорошая отправная точка, если приложение уже контейнеризировано,
но команде не нужны Kubernetes API, operators и собственная cluster platform.

Fargate не делает приложение автоматически stateless. Локальный файл принадлежит
одной task и исчезает вместе с ней; постоянные данные выносят в S3, database или
подходящую shared file system.

### Lambda

AWS Lambda запускает функцию по событию без постоянного fleet. Типичные triggers:

- HTTP через API Gateway или Application Load Balancer;
- S3 event;
- SQS messages;
- EventBridge event или schedule;
- DynamoDB Streams и Kinesis.

Lambda подходит для коротких event-driven операций, нерегулярной нагрузки и
интеграционного glue code. Максимальная длительность одного invocation — 15
минут. Длительность тарифицируется с округлением до 1 миллисекунды, а квоты
concurrency задаются для account и region.

Cold start зависит от runtime, package, initialization, VPC settings и выбранной
памяти. Его нельзя оценивать универсальным числом. Для latency-sensitive пути
измеряют распределение cold/warm latency и при необходимости рассматривают
Provisioned Concurrency или постоянно запущенный container service.

Lambda не подходит как автоматический выбор, если:

- обработка дольше лимита invocation;
- процесс должен постоянно держать соединение или локальное состояние;
- нагрузка стабильна и выделенная capacity оказывается дешевле;
- приложению нужен нестандартный host-level runtime;
- большое число функций усложняет локальную разработку и observability flow.

### EKS

Amazon Elastic Kubernetes Service (EKS) предоставляет managed Kubernetes control
plane. Worker capacity остаётся на EC2 managed node groups, Fargate или смешанной
модели.

EKS оправдан, когда команда действительно использует возможности Kubernetes:

- единая platform model для многих сервисов;
- operators и Custom Resource Definitions;
- service mesh, admission policies и сложное размещение;
- переносимые Helm charts и общий Kubernetes toolchain;
- специальные DaemonSets, sidecars или node pools.

Цена выбора — cluster upgrades, node lifecycle, add-ons, network policies,
capacity planning и диагностика нескольких уровней. Managed control plane не
убирает эту работу.

Для доступа pod к AWS API используют EKS Pod Identity, когда он подходит. AWS
рекомендует его как более простой путь для EKS. IAM Roles for Service Accounts
(IRSA) остаётся альтернативой, в том числе для сценариев, где нужна основанная на
OIDC модель или совместимость за пределами обычного EKS.

### Как выбрать compute

| Вопрос | Lambda | ECS + Fargate | EKS | EC2 |
| --- | --- | --- | --- | --- |
| Единица | invocation | task | pod | VM |
| Управление nodes | нет | нет | зависит от режима | да |
| Долгий процесс | ограничен | да | да | да |
| Kubernetes API | нет | нет | да | только если поставить самостоятельно |
| Контроль ОС | нет | нет | частично через nodes | полный |
| Первый выбор | событие | container API | platform | host-specific ПО |

Если требований мало, выбирают минимально сложный runtime. Возможность перенести
контейнер между платформами сама по себе не окупает постоянную эксплуатацию EKS.

---

## Хранение данных

### S3

Amazon Simple Storage Service (S3) хранит objects в buckets. Это не POSIX file
system: приложение работает с object key и API `PutObject/GetObject`, а не с
обычными файловыми блокировками и произвольной записью в середину файла.

S3 подходит для:

- пользовательских uploads;
- статических assets и backups;
- data lake и архивов;
- больших immutable objects;
- обмена файлами между асинхронными этапами.

S3 обеспечивает strong read-after-write consistency для object operations и
list. Cross-Region Replication работает асинхронно и решает другую задачу:
копирование данных между регионами для compliance или disaster recovery.

Минимальный upload через AWS SDK for Go v2:

```go
func putObject(
    ctx context.Context,
    client *s3.Client,
    bucket string,
    key string,
    body io.Reader,
) error {
    _, err := client.PutObject(ctx, &s3.PutObjectInput{
        Bucket: aws.String(bucket),
        Key:    aws.String(key),
        Body:   body,
    })
    if err != nil {
        return fmt.Errorf("put s3://%s/%s: %w", bucket, key, err)
    }
    return nil
}
```

Клиент SDK создают один раз и переиспользуют. Region и credentials приходят из
конфигурации среды, а не зашиваются в handler.

Для browser upload backend обычно выдаёт presigned URL:

```text
1. Client → API: запросить upload
2. API: проверить пользователя и сгенерировать object key
3. API → Client: presigned PUT/POST с коротким сроком
4. Client → S3: загрузить bytes напрямую
5. S3 Event → SQS/Lambda: проверить и обработать object
```

Presigned PUT ограничивает method, key и срок, но не даёт такого же удобного
policy-ограничения размера, как presigned POST. После загрузки backend не должен
доверять только ответу клиента: финальный object проверяет асинхронный worker.
Полный flow разобран в
[File Upload Flow](../../05-system-design/external-request-flows/05-file-upload-and-background-processing-flow.md).

Для production явно задают:

- Block Public Access и bucket policy;
- versioning, если нужна защита от случайного overwrite/delete;
- lifecycle rules для перехода в холодные storage classes и удаления;
- encryption и KMS key policy, если нужен customer-managed key;
- multipart upload cleanup для незавершённых загрузок;
- access logs или CloudTrail data events для нужного уровня аудита;
- replication и restore procedure по требованиям RPO/RTO.

Storage class выбирают по частоте доступа и допустимому времени retrieval. Более
дешёвый GB может иметь minimum storage duration, retrieval fee и более дорогие
requests.

### EBS и EFS

Elastic Block Store (EBS) предоставляет block volume для EC2. Обычно volume
привязан к одной Availability Zone и используется как диск VM или database node.
Удаление EC2 не всегда удаляет связанный volume: поведение задаёт
`DeleteOnTermination`.

Elastic File System (EFS) предоставляет managed NFS file system, которую могут
монтировать несколько clients. EFS нужен для общего POSIX-like доступа, но не
заменяет object storage: модель производительности и стоимость операций другие.

| Требование | Сервис |
| --- | --- |
| Object по key, огромный scale | S3 |
| Block device для одной VM | EBS |
| Общая NFS file system | EFS |

### RDS и Aurora

Amazon Relational Database Service (RDS) управляет PostgreSQL, MySQL, MariaDB,
Oracle, SQL Server и Db2 в поддерживаемых вариантах. AWS обслуживает instance,
backups и часть failover, но schema, indexes, query plans и connection pools
остаются ответственностью команды.

Для production решают отдельно:

- нужен ли Multi-AZ deployment;
- какой RPO/RTO дают backups и point-in-time recovery;
- как приложение переживает DNS change и reconnect при failover;
- сколько соединений откроют все replicas приложения;
- нужны ли read replicas и допустим ли replication lag;
- где хранится password или используется IAM database authentication;
- как регулярно проверяется restore.

Multi-AZ DB instance содержит primary и синхронный standby в другой AZ. Standby
нужен для failover и не является обычной read replica. Для этой модели AWS
указывает типичное время failover 60–120 секунд, но большая транзакция или recovery
могут увеличить его.

Read replica принимает чтение и реплицируется асинхронно. Она помогает read
scaling или disaster recovery, но приложение должно учитывать lag. Текущие
service quotas и engine-specific ограничения проверяют перед проектированием, а
не фиксируют в архитектуре числом из статьи.

Aurora — MySQL- и PostgreSQL-compatible managed database с отделённым от compute
распределённым storage. Aurora рассматривают, когда её availability, replica
model, fast failover или serverless capacity соответствуют требованиям. Для
обычного небольшого CRUD backend RDS PostgreSQL остаётся нормальной отправной
точкой; Aurora не исправляет плохие запросы и неограниченный pool.

### DynamoDB

DynamoDB — managed key-value/document database. Она хорошо работает, когда
access patterns известны заранее и запрос начинается с partition key.

Сильные стороны:

- автоматическое распределение данных и высокий throughput;
- on-demand и provisioned capacity modes;
- conditional writes и транзакционные операции;
- Time to Live для автоматического удаления устаревших items;
- DynamoDB Streams для change events;
- Global Tables для multi-region replication.

Модель проектируют от запросов, а не от нормализованных entities. Partition key
должен распределять нагрузку; один горячий ключ ограничивает параллелизм даже при
большой общей capacity. Secondary indexes ускоряют дополнительные access
patterns, но добавляют storage и write cost.

DynamoDB не выбирают как drop-in замену PostgreSQL, если нужны ad-hoc queries,
joins и гибкие multi-row transactions. Наличие transaction API не превращает
key-value модель в реляционную.

### ElastiCache

ElastiCache предоставляет managed Valkey, Redis OSS и Memcached в поддерживаемых
вариантах. Типичные задачи — cache, sessions, rate limiting и временное быстрое
состояние.

Cache должен иметь определённое поведение при miss, eviction, failover и полной
недоступности. Если потеря значения нарушает бизнес-инвариант, это уже не только
cache и нужен durable source of truth.

### Как выбрать хранилище

| Access pattern | Кандидат |
| --- | --- |
| `PUT/GET` большого object по key | S3 |
| Block storage для EC2 | EBS |
| Общая POSIX-like file system | EFS |
| Реляционные транзакции и joins | RDS PostgreSQL/MySQL |
| Отдельные требования Aurora к HA/scale | Aurora |
| Key-value/document по известным ключам | DynamoDB |
| Cache и временное состояние | ElastiCache |

Формат данных не определяет выбор. JSON можно хранить и в PostgreSQL, и в S3, и
в DynamoDB; важны операции, транзакционные границы, latency, рост и restore path.

---

## Асинхронная работа и интеграции

### SQS

Amazon Simple Queue Service (SQS) хранит сообщения, пока consumer не обработает
и не удалит их.

**Standard queue** даёт at-least-once delivery, best-effort ordering и очень
высокую пропускную способность. Consumer обязан переживать дубликаты.

**FIFO queue** сохраняет порядок внутри `MessageGroupId` и дедуплицирует отправку
по `MessageDeduplicationId` в пределах deduplication interval. AWS называет это
exactly-once processing, но это не гарантирует exactly-once бизнес-эффект во
внешней базе или API. Если worker записал данные и умер до `DeleteMessage`, то же
сообщение станет видимым снова. Handler всё равно делают идемпотентным.

Пропускная способность FIFO зависит от режима, региона, batching и распределения
по message groups. Фиксировать старое число `3000 messages/sec` как общий предел
нельзя. Один message group обрабатывается последовательно; параллелизм получают
несколькими независимыми groups.

Безопасный consumer flow:

```text
ReceiveMessage with long polling
        │
        ▼
validate payload and idempotency key
        │
        ▼
perform business transaction
        │
        ├── error → do not delete; message becomes visible again
        │
        └── success → DeleteMessage
```

Упрощённый Go-код показывает только lifecycle одного сообщения:

```go
func handleMessage(
    ctx context.Context,
    client *sqs.Client,
    queueURL string,
    message types.Message,
) error {
    if message.Body == nil || message.ReceiptHandle == nil {
        return errors.New("SQS message has no body or receipt handle")
    }

    if err := processIdempotently(ctx, *message.Body); err != nil {
        return fmt.Errorf("process SQS message: %w", err)
    }

    _, err := client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
        QueueUrl:      aws.String(queueURL),
        ReceiptHandle: message.ReceiptHandle,
    })
    if err != nil {
        return fmt.Errorf("delete processed SQS message: %w", err)
    }
    return nil
}
```

Visibility timeout должен покрывать нормальное время обработки с запасом. Для
долгой операции worker продлевает его через `ChangeMessageVisibility`. После
настроенного числа неуспешных попыток message отправляют в dead-letter queue
(DLQ), а backlog и age of oldest message включают в alerts.

### SNS

Simple Notification Service (SNS) публикует одно сообщение нескольким
subscribers. Для надёжного fan-out backend-системы часто подписывают отдельную SQS
queue каждого consumer:

```text
orders-api → SNS topic: order-created
                 ├── SQS billing → billing-worker
                 ├── SQS email   → email-worker
                 └── SQS audit   → audit-writer
```

Медленный email-worker не блокирует billing-worker, потому что у них разные
queues, retries и DLQ. Прямая HTTP subscription возможна, но тогда доступность и
повторы внешнего endpoint надо проектировать отдельно.

### EventBridge

Amazon EventBridge маршрутизирует события по rules. Он полезен для интеграции AWS
services, SaaS sources и domain events, когда consumers выбираются по полям event,
а не только по имени topic.

SNS проще для прямого fan-out. EventBridge удобнее, когда нужны event bus,
content-based routing, archive/replay или несколько независимых правил. Ни один
из сервисов не заменяет durable task queue автоматически: если consumer должен
контролировать скорость и backlog, target часто остаётся SQS.

### Step Functions

Step Functions хранит состояние workflow и координирует шаги Lambda, ECS tasks и
AWS API calls. Сервис подходит для процесса, где нужны retries отдельных шагов,
ожидание, timeout, compensation и видимый execution state.

Короткую атомарную операцию яснее оставить в коде. Оркестратор окупается, когда
шаги имеют разные времена жизни и failure policy.

### SQS, SNS, EventBridge или Kafka

| Требование | Выбор |
| --- | --- |
| Task queue для workers | SQS |
| Fan-out одного сообщения | SNS + SQS |
| Маршрутизация событий по полям | EventBridge |
| Replayable ordered event log | Kafka/MSK или Kinesis по задаче |
| Долгий workflow с состоянием | Step Functions |

SQS хранит задачу до удаления, но не является долговременным event log для
произвольного replay. Если новый consumer должен перечитать историю за месяц,
сначала рассматривают streaming log или отдельное durable archive.

---

## Сеть и внешний трафик

### VPC, subnets и routes

Virtual Private Cloud (VPC) — региональная виртуальная сеть. Subnet принадлежит
одной Availability Zone. Route table определяет следующий hop для destination,
а internet gateway подключает VPC к интернету.

Типовая схема:

```text
Internet
   │
   ▼
Internet Gateway
   │
   ▼
Public subnets across AZs
   ├── Application Load Balancer
   └── NAT Gateway per AZ when required
              │
              ▼
Private subnets across AZs
   ├── ECS tasks / EC2 / EKS nodes
   └── RDS / ElastiCache in isolated data subnets
```

Public subnet определяется route к internet gateway, а не названием. Ресурсу для
прямого интернет-доступа дополнительно нужен public IPv4/IPv6 и разрешающие
security rules.

NAT Gateway даёт исходящий IPv4-доступ private resources и не принимает
произвольные входящие соединения. Он оплачивается за время и обработанные bytes.
Для устойчивости обычно размещают NAT Gateway в каждой используемой AZ и ведут
traffic local-AZ route, иначе отказ одной зоны или cross-AZ transfer становится
частью рабочего пути.

### Security Groups и NACL

| Свойство | Security Group | Network ACL |
| --- | --- | --- |
| Применяется к | network interface/resource | subnet |
| Состояние соединения | stateful | stateless |
| Правила | allow | allow и deny |
| Роль | основная граница workload | дополнительная граница subnet |

Security Group разрешает входящий или исходящий поток. Ответный трафик
разрешённого соединения учитывается автоматически. В правилах можно ссылаться на
другую security group: например, database принимает TCP 5432 только от group
приложения, а не от всего subnet CIDR.

Network ACL проверяет каждый direction отдельно. Из-за stateless-модели ошибка в
ephemeral ports легко ломает ответы. Для большинства приложений основную модель
строят на security groups, а NACL добавляют при явной subnet-level потребности.

### VPC Endpoints и private access

VPC endpoint позволяет обратиться к поддерживаемому AWS service без маршрута
через public internet и NAT Gateway.

- **Gateway endpoint** используется для S3 и DynamoDB и добавляется в route
  tables.
- **Interface endpoint** создаёт private network interfaces через AWS PrivateLink
  и оплачивается за время endpoint в каждой AZ и обработанные данные.

Endpoint не является автоматически бесплатным способом доступа. Для S3 и
DynamoDB gateway endpoint часто уменьшает NAT cost. Для interface endpoint надо
сравнить hourly/data processing charge, количество AZ и объём traffic.

### ALB и NLB

Application Load Balancer (ALB) работает на L7 и понимает HTTP(S): host/path
routing, redirects, TLS termination, health checks и интеграцию с WAF.

Network Load Balancer (NLB) работает на L4 и пересылает TCP/UDP/TLS traffic с
низкой latency и высокой пропускной способностью. Он нужен для не-HTTP протоколов,
source IP preservation и сценариев со static IP per AZ.

NLB может передавать WebSocket как TCP traffic, но не понимает WebSocket routing
на уровне приложения. Для обычного HTTP API выбирают ALB.

### Route 53, CloudFront, WAF и Shield

Route 53 предоставляет authoritative DNS, health checks и routing policies:
weighted, latency-based, geolocation и failover.

CloudFront — content delivery network. Он кеширует static и cacheable HTTP
content ближе к пользователю. Эффект зависит от cache hit ratio: динамический
персонализированный API с `Cache-Control: no-store` не становится дешевле только
из-за CloudFront.

AWS Shield Standard предоставляет базовую DDoS-защиту поддерживаемых AWS
resources. AWS WAF фильтрует HTTP requests по rules. CloudFront сам по себе не
является WAF: эти роли надо различать.

Certificate Manager (ACM) выпускает и обновляет TLS certificates для
поддерживаемых endpoints. Certificate для CloudFront запрашивают в `us-east-1`,
даже если origin расположен в другом регионе.

---

## IAM, аутентификация и секреты

### IAM identities и policies

IAM отвечает на вопрос: какой principal может выполнить какое API action над
каким resource и при каких conditions.

Основные сущности:

- **IAM role** — identity без постоянного пароля/access key, которую можно
  временно assume;
- **IAM policy** — JSON-документ с `Effect`, `Action`, `Resource` и `Condition`;
- **resource-based policy** — policy на S3 bucket, SQS queue, KMS key и других
  поддерживаемых resources;
- **permission boundary** — максимальная граница прав IAM principal;
- **SCP** — внешняя граница разрешённых действий account в Organization.

Явный `Deny` имеет приоритет над `Allow`. Разрешение должно пройти все применимые
границы: organization policy, identity/resource policy, permission boundary и
session policy.

Пример runtime-policy для чтения одного prefix S3:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject"
      ],
      "Resource": "arn:aws:s3:::orders-prod/invoices/*"
    }
  ]
}
```

`s3:ListBucket` использует ARN bucket без `/*` и при необходимости ограничивается
condition по prefix. Один wildcard `s3:*` на `*` скрывает модель доступа и
увеличивает blast radius.

### Identity для workload

Приложение получает отдельную role:

- EC2 — instance profile;
- ECS — task role, отличная от task execution role;
- Lambda — execution role;
- EKS — Pod Identity или IRSA;
- внешний CI/CD — OIDC federation и `AssumeRoleWithWebIdentity`.

Task execution role нужна ECS agent для pull image и отправки logs. Task role
получает само приложение для S3, SQS или DynamoDB. Объединение этих ролей обычно
даёт runtime лишние infrastructure permissions.

### AWS SDK for Go v2

`config.LoadDefaultConfig` использует credential provider chain. Локально это
может быть профиль IAM Identity Center в shared config. В AWS runtime SDK получает
временные credentials из роли среды. Один и тот же код работает без встроенных
access keys:

```go
func newS3Client(ctx context.Context, region string) (*s3.Client, error) {
    cfg, err := config.LoadDefaultConfig(
        ctx,
        config.WithRegion(region),
    )
    if err != nil {
        return nil, fmt.Errorf("load AWS config: %w", err)
    }
    return s3.NewFromConfig(cfg), nil
}
```

Environment variables с постоянным access key допустимы только как ограниченный
legacy-вариант. В production предпочтительны temporary credentials роли. SDK
обновляет их автоматически.

Retries SDK не означают, что любая бизнес-операция безопасна для повторения.
Повтор `GetObject` обычно безопасен, а повтор внешнего payment request требует
idempotency key. Timeout и cancellation передают через `context.Context`.

### Secrets Manager и Parameter Store

Secrets Manager хранит versioned secrets и поддерживает rotation workflows.
Systems Manager Parameter Store подходит для конфигурации и простых secure
parameters. Конкретный выбор зависит от rotation, размера, throughput, integration
и цены.

Секрет читают через IAM role приложения. Ошибки чтения и JSON parsing нельзя
игнорировать:

```go
func loadDatabaseSecret(
    ctx context.Context,
    client *secretsmanager.Client,
    secretID string,
) (databaseCredentials, error) {
    output, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
        SecretId: aws.String(secretID),
    })
    if err != nil {
        return databaseCredentials{}, fmt.Errorf("get database secret: %w", err)
    }
    if output.SecretString == nil {
        return databaseCredentials{}, errors.New("database secret is not a string")
    }

    var credentials databaseCredentials
    rawSecret := []byte(*output.SecretString)
    if err := json.Unmarshal(rawSecret, &credentials); err != nil {
        return databaseCredentials{}, fmt.Errorf("decode database secret: %w", err)
    }
    return credentials, nil
}
```

Читать secret на каждый HTTP request дорого и создаёт новую runtime dependency.
Обычно используют startup load или client-side cache с контролируемым refresh.
При rotation приложение должно уметь получить новую версию и переподключиться.

Key Management Service (KMS) управляет cryptographic keys и операциями
encrypt/decrypt/sign. KMS key не заменяет secret: key остаётся в KMS, а secret —
значение, которое приложение должно получить.

Подробнее способы доставки разобраны в
[Secrets delivery options](../../11-security/secrets-management/01-secrets-delivery-options.md).

---

## Сборка и доставка

### ECR

Elastic Container Registry (ECR) хранит container images. Production deployment
лучше привязывать к immutable digest, а не к перезаписываемому tag `latest`.

```text
git commit
    │
    ▼
CI: tests → build image → vulnerability scan
    │
    ▼
ECR: image@sha256:...
    │
    ▼
ECS/EKS deployment → health checks → gradual rollout
```

Lifecycle policy удаляет старые untagged images. Перед удалением проверяют, не
нужны ли они для rollback и audit.

### CI/CD и IaC

Pipeline обычно выполняет:

1. Tests и static analysis.
2. Сборку одного immutable artifact.
3. Публикацию artifact в ECR или S3.
4. Deployment новой revision/task definition.
5. Health checks и controlled rollout.
6. Автоматический rollback или остановку rollout при ошибке.

CI получает AWS credentials через OIDC federation и короткую role session. Access
key в repository secret создаёт долгоживущий credential, который сложнее
ограничить и отозвать.

CodeBuild и CodePipeline могут реализовать этот flow внутри AWS, но команда может
использовать GitHub Actions, GitLab CI или другой CI. Важнее границы прав и
immutable artifact, а не название orchestration product.

---

## Наблюдаемость и аудит

### CloudWatch

CloudWatch объединяет несколько ролей:

| Сигнал | Инструмент | Практическое использование |
| --- | --- | --- |
| Metrics | CloudWatch Metrics | request rate, errors, latency, saturation |
| Logs | CloudWatch Logs | structured application и platform logs |
| Alarms | CloudWatch Alarms | уведомление или ограниченное automated action |
| Dashboards | CloudWatch Dashboards | обзор service health и SLO signals |
| Traces | X-Ray / OpenTelemetry integration | путь запроса между сервисами |

EC2 автоматически публикует host-visible metrics вроде CPU, network и disk
operations. Memory и filesystem usage находятся внутри гостевой ОС, поэтому для
них нужен CloudWatch Agent или другой collector.

ECS с `awslogs` driver и Lambda отправляют `stdout/stderr` в CloudWatch Logs. Для
поиска приложение пишет structured JSON с постоянными полями `service`,
`environment`, `request_id`, `trace_id`, `operation` и `error`.

Retention задают явно. `Never expire` на verbose logs превращает временную
диагностику в постоянный storage cost.

### CloudTrail и Config

CloudTrail записывает account activity и API events. Management events и data
events имеют разный объём и стоимость; data events для S3 object access или
Lambda invocation включают осознанно.

AWS Config отслеживает конфигурацию поддерживаемых ресурсов и её изменения. Это
помогает проверять правила вроде «S3 bucket не public» или «security group не
открывает SSH в интернет», но не заменяет runtime metrics и application logs.

### Минимальный набор сигналов

До production нужны:

- request rate, error rate и latency API;
- CPU/memory и saturation compute;
- число healthy targets и deployment failures;
- database connections, latency, storage и replica lag;
- SQS backlog, age of oldest message и DLQ depth;
- Lambda errors, throttles, duration и concurrency;
- log ingestion volume и retention;
- alarms с владельцем и понятным runbook.

Лог `error` без metric и alarm не создаёт наблюдаемость: никто не обязан читать
все logs вручную.

---

## Три типовые архитектуры

### Небольшой container API

```text
Client
  │ HTTPS
  ▼
Route 53 → Application Load Balancer
                         │
                         ▼
              ECS service on Fargate
                  ├── RDS PostgreSQL
                  ├── S3
                  └── SQS worker queue

Runtime identity: ECS task role
Secrets: Secrets Manager
Signals: CloudWatch Metrics, Logs and Alarms
```

Это хороший старт для небольшой команды с контейнеризированным Go-сервисом. Нет
Kubernetes cluster и EC2 fleet, но команда всё ещё проектирует database HA,
connection pool, idempotency, autoscaling bounds и alerts.

### Serverless event-driven backend

```text
Client → API Gateway → Lambda → DynamoDB
                           │
                           ├── S3
                           └── SQS → Lambda worker

Events: EventBridge
Secrets: Secrets Manager
Signals: CloudWatch + X-Ray/OpenTelemetry
```

Модель подходит для нерегулярной нагрузки и коротких handlers. Ограничения
concurrency защищают downstream: если Lambda масштабируется быстрее database или
partner API, очередь и reserved concurrency должны сгладить поток.

### Kubernetes platform

```text
Route 53 → CloudFront + WAF → ALB
                               │
                               ▼
                              EKS
                       ├── stateless APIs
                       ├── SQS consumers
                       └── internal services
                               │
                 ┌─────────────┼─────────────┐
                 ▼             ▼             ▼
              Aurora      ElastiCache       S3
```

EKS не требует переносить database, queue и object storage внутрь cluster.
Managed dependencies обычно уменьшают operational burden. Stateful component в
Kubernetes выбирают, когда контроль или portability действительно важнее
managed-варианта.

---

## Стоимость и границы ответственности

В AWS нет единой модели оплаты. EC2 и RDS держат выделенную capacity, Lambda и
Fargate считают runtime resources, S3 учитывает storage, requests и transfer, а
NAT Gateway — время и обработанные bytes.

Перед выбором сервиса считают:

1. Steady-state capacity.
2. Peak capacity.
3. Failure capacity после потери AZ или node pool.
4. Объём хранения и срок retention.
5. Network flow между AZ, регионами и интернетом.
6. Число requests, messages, log bytes и custom metrics.
7. Стоимость эксплуатации командой.

Managed-сервис может быть дороже по строке счёта и дешевле по total cost of
ownership. Self-hosted PostgreSQL на EC2 экономит часть service premium, но
добавляет patching, backups, failover, monitoring и круглосуточную ответственность.

Конкретные цены зависят от региона и меняются. Практический расчёт, Savings Plans,
Spot, network cost и защитные меры разобраны в
[Cloud cost и архитектурные решения](./02-cloud-cost-and-architecture.md).

---

## Типичные ошибки

### Один account и одна role для всего

Общая граница ускоряет первый запуск, но увеличивает blast radius. Ошибка
non-production pipeline получает путь к production, а расходы и quotas разных
систем смешиваются. Accounts и runtime roles разделяют по ответственности.

### Постоянные access keys

Access key в `.env`, CI secret или container image может жить месяцами после
утечки. Люди используют federation, workloads — IAM roles, внешний CI — OIDC.

### Public subnet как замена нормальному egress design

Перенос workload в public subnet ради экономии NAT не делает его безопасным и не
убирает стоимость public IPv4. Сначала определяют, какой outbound traffic нужен,
используют endpoints для AWS services и сравнивают NAT, proxy и IPv6 paths.

### Одна VM вместо отказоустойчивого service

Snapshot не превращает одну EC2 instance в high availability. Нужны как минимум
health checks, replace mechanism, несколько AZ и проверенный deployment/restore
flow.

### RDS Multi-AZ как read scaling

Standby классической Multi-AZ DB instance не обслуживает обычные reads. Read
scaling дают read replicas или подходящая cluster architecture, но они добавляют
replication lag и routing decisions.

### DynamoDB без access patterns

Таблицу нельзя проектировать как набор нормализованных entities, а затем ожидать
произвольные queries. Сначала записывают access patterns, partition key и
transaction boundaries.

### SQS consumer без идемпотентности

At-least-once delivery допускает повтор. Бизнес-эффект защищают idempotency key,
conditional write, inbox/outbox или уникальный constraint, а message удаляют
только после успешного эффекта.

### Secrets Manager на каждом request

Каждый HTTP request получает дополнительную latency, цену и dependency на control
service. Secret кешируют с продуманным refresh и поведением при rotation.

### CloudFront перед любым API

CDN экономит origin traffic только при достаточном cache hit ratio. Для
персонализированного ответа без cache он добавляет ещё один слой и отдельные
requests.

### Logs без retention

Verbose payloads, headers и debug messages одновременно создают cost, риск утечки
данных и сложность поиска. Структуру, sampling, redaction и retention проектируют
до incident.

---

## Практический чек-лист

### До создания ресурсов

- [ ] Production и non-production разделены осознанной account boundary.
- [ ] Люди входят через IAM Identity Center или federation с MFA.
- [ ] Root user защищён MFA и не имеет access keys.
- [ ] Выбран основной region с учётом users, data, compliance и cost.
- [ ] CloudTrail, Budgets и Cost Anomaly Detection включены до traffic.
- [ ] Tags и владельцы расходов определены заранее.

### Для приложения

- [ ] Выбран минимально сложный compute runtime.
- [ ] Runtime role отделена от deployment/execution role.
- [ ] AWS SDK использует default credential chain без встроенных keys.
- [ ] Secrets хранятся вне image и repository.
- [ ] Autoscaling имеет допустимые minimum/maximum bounds.
- [ ] Локальный диск disposable runtime не хранит критичное состояние.

### Для данных и messaging

- [ ] Storage выбран по access pattern и transaction boundary.
- [ ] Backups, retention, RPO/RTO и restore procedure проверены.
- [ ] Connection pool умножен на максимальное число replicas приложения.
- [ ] Queue consumers идемпотентны.
- [ ] Visibility timeout, retry и DLQ согласованы со временем обработки.
- [ ] Lifecycle policies очищают старые objects, snapshots и multipart uploads.

### Для production

- [ ] Workload распределён минимум по двум AZ, если это требует SLO.
- [ ] Security Groups открывают только необходимые flows.
- [ ] VPC endpoints и NAT paths выбраны по traffic и cost.
- [ ] Есть dashboards, alarms, structured logs и trace correlation.
- [ ] Deployment использует immutable artifact и controlled rollout.
- [ ] Проверены сценарии потери task, node, AZ и доступа к dependency.

---

## Interview-ready answer

**1. Как выбирать между Lambda, ECS, EKS и EC2?**

- Короткие события — Lambda подходит для ограниченного event-driven handler без
  постоянного fleet.
- Контейнерный сервис — ECS с Fargate даёт managed container runtime без
  Kubernetes.
- Kubernetes platform — EKS нужен при зависимости от Kubernetes API, operators
  и общей platform model.
- Полный контроль — EC2 выбирают для ОС, legacy software и специальных host
  requirements.
- Цена выбора — чем ниже уровень абстракции, тем больше контроля и
  эксплуатационной ответственности.

**2. Как приложение безопасно обращается к AWS APIs?**

- Identity — workload получает отдельную IAM role.
- Credentials — AWS runtime выдаёт краткоживущие credentials, а SDK обновляет их
  через provider chain.
- Authorization — policy разрешает только нужные actions над конкретными
  resources.
- Люди и CI — сотрудники используют federation, внешний CI использует OIDC, а не
  постоянные access keys.

**3. Чем SQS отличается от SNS и EventBridge?**

- SQS — durable queue хранит работу для consumer и регулирует backlog.
- SNS — topic делает прямой fan-out нескольким subscribers.
- EventBridge — event bus маршрутизирует события по rules и полям payload.
- Надёжный fan-out — SNS или EventBridge часто доставляет каждую ветку в
  отдельную SQS queue.

**4. Чем S3, EBS и EFS отличаются друг от друга?**

- S3 — object storage с API по key, высокой durability и практически
  неограниченным scale.
- EBS — block volume для EC2 в конкретной AZ.
- EFS — общая managed NFS file system для нескольких clients.
- Выбор — определяется API доступа и временем жизни данных, а не только ценой
  за GB.

**5. Что даёт Multi-AZ в RDS и чем оно отличается от read replica?**

- Multi-AZ — standby или cluster topology повышает availability и обслуживает
  failover.
- Read replica — асинхронная копия разгружает чтения и может иметь lag.
- Приложение — должно переживать reconnect, DNS change и временные errors при
  failover.
- Проверка — backups и standby недостаточны без регулярного restore/failover
  test.

**6. Что важно знать про сеть AWS?**

- Scope — VPC региональна, а subnet принадлежит одной Availability Zone.
- Security — Security Group stateful и привязана к network interface; NACL
  stateless и действует на subnet.
- Ingress — ALB выбирают для HTTP(S), NLB для L4 traffic.
- Egress — NAT Gateway, endpoints, public IPv4 и cross-AZ paths имеют разные
  security и cost trade-offs.

**7. Какие AWS cost-ошибки наиболее опасны?**

- Data transfer — internet, cross-AZ, cross-region и NAT processing считаются по
  разным meters.
- Idle capacity — EC2, RDS, NAT Gateway, load balancers и volumes стоят денег
  без traffic.
- Unbounded scale — Lambda concurrency, autoscaling и log ingestion могут расти
  быстрее downstream и budget alerts.
- Commitment — Savings Plans и RI покупают по измеренному baseline, а не по
  текущему размеру fleet.

---

## Официальная документация

- [AWS global infrastructure](https://aws.amazon.com/about-aws/global-infrastructure/)
- [AWS Organizations](https://docs.aws.amazon.com/organizations/latest/userguide/orgs_introduction.html)
- [IAM security best practices](https://docs.aws.amazon.com/IAM/latest/UserGuide/best-practices.html)
- [AWS SDK for Go v2 configuration](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-gosdk.html)
- [Amazon EC2](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/concepts.html)
- [Amazon ECS](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/Welcome.html)
- [AWS Fargate](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/AWS_Fargate.html)
- [AWS Lambda quotas](https://docs.aws.amazon.com/lambda/latest/dg/gettingstarted-limits.html)
- [AWS Lambda pricing](https://aws.amazon.com/lambda/pricing/)
- [Amazon EKS](https://docs.aws.amazon.com/eks/latest/userguide/what-is-eks.html)
- [EKS Pod Identity and IRSA](https://docs.aws.amazon.com/eks/latest/userguide/service-accounts.html)
- [Amazon S3 consistency model](https://docs.aws.amazon.com/AmazonS3/latest/userguide/Welcome.html#ConsistencyModel)
- [Amazon RDS](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Welcome.html)
- [RDS Multi-AZ failover](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.MultiAZ.Failover.html)
- [Amazon DynamoDB](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/Introduction.html)
- [Amazon SQS message quotas](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/quotas-messages.html)
- [SQS FIFO exactly-once processing](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/FIFO-queues-exactly-once-processing.html)
- [Amazon SNS](https://docs.aws.amazon.com/sns/latest/dg/welcome.html)
- [Amazon EventBridge](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-what-is.html)
- [AWS Step Functions](https://docs.aws.amazon.com/step-functions/latest/dg/welcome.html)
- [Amazon VPC](https://docs.aws.amazon.com/vpc/latest/userguide/what-is-amazon-vpc.html)
- [Elastic Load Balancing](https://docs.aws.amazon.com/elasticloadbalancing/latest/userguide/what-is-load-balancing.html)
- [Amazon CloudFront](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/Introduction.html)
- [AWS Secrets Manager](https://docs.aws.amazon.com/secretsmanager/latest/userguide/intro.html)
- [Amazon CloudWatch](https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/WhatIsCloudWatch.html)
- [AWS CloudTrail](https://docs.aws.amazon.com/awscloudtrail/latest/userguide/cloudtrail-user-guide.html)
