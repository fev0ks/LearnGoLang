# AWS: основные сервисы

AWS — самый распространённый облачный провайдер, и хотя GCP/Azure тоже популярны, AWS остаётся "лингва франка" cloud-инфраструктуры. Senior backend должен ориентироваться в основных сервисах: что они делают, когда нужны, какие альтернативы, какие подводные камни.

Этот файл — **обзор сервисов**, не туториал по каждому. Цель — дать карту, чтобы знать на что смотреть в задаче.

## Содержание

- [Региональность и AZ](#региональность-и-az)
- [EC2 — виртуальные машины](#ec2-виртуальные-машины)
- [VPC — сетевая изоляция](#vpc-сетевая-изоляция)
- [Security Groups vs NACL](#security-groups-vs-nacl)
- [IAM — управление доступом](#iam-управление-доступом)
- [S3 — объектное хранилище](#s3-объектное-хранилище)
- [RDS — managed реляционные БД](#rds-managed-реляционные-бд)
- [DynamoDB — managed NoSQL](#dynamodb-managed-nosql)
- [SQS — очереди сообщений](#sqs-очереди-сообщений)
- [SNS — pub/sub и notifications](#sns-pubsub-и-notifications)
- [Lambda — serverless](#lambda-serverless)
- [EKS / ECS — оркестрация контейнеров](#eks-ecs-оркестрация-контейнеров)
- [CloudWatch — мониторинг](#cloudwatch-мониторинг)
- [Route 53 — DNS](#route-53-dns)
- [CloudFront — CDN](#cloudfront-cdn)
- [ELB / ALB / NLB — load balancers](#elb-alb-nlb-load-balancers)
- [Secrets Manager и Parameter Store](#secrets-manager-и-parameter-store)
- [Managed vs self-hosted](#managed-vs-self-hosted)
- [AWS SDK в Go](#aws-sdk-в-go)
- [Полезные команды AWS CLI](#полезные-команды-aws-cli)

---

## Региональность и AZ

AWS разделён на **регионы** (Region) — отдельные географические локации. Внутри каждого региона — несколько **Availability Zones (AZ)** — изолированных датацентров.

```
AWS глобально
├── us-east-1 (N. Virginia)        ← регион
│   ├── us-east-1a                 ← AZ
│   ├── us-east-1b
│   ├── us-east-1c
│   └── ...
├── eu-west-1 (Ireland)
│   ├── eu-west-1a
│   ├── eu-west-1b
│   └── eu-west-1c
└── ap-southeast-1 (Singapore)
    └── ...
```

**Что нужно знать:**

- **AZ — это независимые датацентры** в одном регионе, с low-latency связью (<2 мс) друг к другу
- **Multi-AZ deployment** — стандарт для production: если одна AZ упала, другие продолжают
- **Cross-region trafic** — платный и медленный (десятки-сотни мс)
- **us-east-1** исторически "первый", иногда падает целиком (как 7 дек 2021) — критичные сервисы лучше держать в нескольких регионах
- **Сервисы как S3, DynamoDB, IAM** — региональные (или глобальные), некоторые сервисы доступны не во всех регионах

---

## EC2 — виртуальные машины

**EC2 (Elastic Compute Cloud)** — VM в облаке. Запускаешь instance, получаешь Linux/Windows машину.

### Типы instance

Категории:
- **t** (burstable) — экономичные, для маленькой/непостоянной нагрузки. CPU credits.
- **m** (general purpose) — баланс CPU/memory
- **c** (compute optimized) — много CPU, для тяжёлой обработки
- **r** (memory optimized) — много RAM, для in-memory БД, caches
- **i** / **d** — много локального storage (SSD/HDD)
- **g** / **p** — GPU для ML, графики

Размер: `nano`, `micro`, `small`, `medium`, `large`, `xlarge`, `2xlarge`, ..., `24xlarge` — пропорционально удваивается.

Примеры: `t3.medium`, `m6i.xlarge`, `c7g.4xlarge`, `r6i.16xlarge`.

### Pricing model

- **On-Demand** — почасовая оплата, можешь остановить когда хочешь. Самый дорогой вариант.
- **Reserved Instances (RI)** — обязательство на 1-3 года, скидка 30-72%.
- **Savings Plans** — гибче RI: обязательство на $/час, можешь менять instance types.
- **Spot** — "лишние" мощности AWS, скидка до 90%. AWS может забрать в любой момент с предупреждением 2 мин.

**Стратегия:** baseline на RI/Savings, burst на on-demand, batch jobs на Spot.

### EBS — disks для EC2

EBS (Elastic Block Store) — block storage attach'ивающийся к EC2. Не путать с S3 (object storage).

- **gp3 / gp2** — general purpose SSD, $0.08-0.1/GB-month
- **io2** — high-IOPS SSD, для БД
- **st1** — HDD throughput, для batch обработки
- **sc1** — cold HDD, дёшево, медленно

Снапшоты EBS — в S3 (incremental, дёшево).

### Когда использовать EC2

- Когда нужен **полный контроль** над OS (custom kernel, специфические настройки)
- Stateful workloads, не помещающиеся в managed service
- Legacy software, требующий специфической среды

**Когда НЕ использовать:** для веб-серверов и API сейчас часто лучше ECS/EKS (контейнеры) или Lambda — меньше operational overhead.

---

## VPC — сетевая изоляция

**VPC (Virtual Private Cloud)** — изолированная виртуальная сеть в AWS. Здесь живут твои EC2, RDS, и другие ресурсы.

### Структура

```
VPC: 10.0.0.0/16  (твоя приватная сеть)
├── Public subnet:  10.0.1.0/24 (AZ: a)    ← с интернетом
├── Public subnet:  10.0.2.0/24 (AZ: b)
├── Private subnet: 10.0.10.0/24 (AZ: a)   ← без прямого интернета
├── Private subnet: 10.0.11.0/24 (AZ: b)
└── Private subnet: 10.0.12.0/24 (AZ: c)
```

**Public subnet** — имеет роут в **Internet Gateway (IGW)**, instance в нём может быть доступен из интернета (если есть public IP).

**Private subnet** — без IGW. Instance не доступен снаружи. Для исходящего трафика — **NAT Gateway** в public subnet.

### Типичная архитектура

```
Internet
   ↓
[Internet Gateway]
   ↓
[Public subnet]
   │
   ├── ALB (load balancer)
   │     ↓
   └── NAT Gateway
         ↓
[Private subnet]
   │
   ├── EC2 instances / ECS tasks   ← приложения
   ├── RDS instances                ← БД
   └── ElastiCache                  ← Redis
```

**Идея:**
- Frontend (ALB) — публичный
- Backend сервисы — приватные, доступны только через ALB
- БД — приватные, доступны только из backend

### VPC peering / Transit Gateway

Несколько VPC можно соединять:
- **VPC Peering** — point-to-point между двумя VPC
- **Transit Gateway** — hub-and-spoke, многие VPC через один gateway

Используется для:
- Connect разных environments (prod, staging)
- Connect dev VPC с corporate network
- Multi-account architectures

### VPC Endpoints

Доступ к AWS сервисам (S3, DynamoDB) **без выхода в интернет**:

- **Gateway Endpoint** — для S3 и DynamoDB, бесплатно
- **Interface Endpoint** — для остальных сервисов, ~$7/мес за каждый

Преимущества: безопасность (трафик не идёт через интернет), меньше costs (не платим за NAT Gateway трафик к AWS).

---

## Security Groups vs NACL

Два слоя сетевого firewall в VPC. Часто путают.

### Security Group (SG)

- Действует на **уровне instance** (точнее, ENI — Elastic Network Interface)
- **Stateful** — если разрешил входящий запрос, ответ автоматически разрешён
- Только **allow** rules (deny by default)
- Применяется к instance, не к подсети

Пример:
```yaml
WebServerSG:
  Ingress:
    - Port: 443
      Source: 0.0.0.0/0       # HTTPS из любого места
    - Port: 22
      Source: 10.0.0.0/16     # SSH только из VPC
  Egress:
    - Allow all                # стандартно
```

### NACL (Network ACL)

- Действует на **уровне subnet**
- **Stateless** — нужно явно разрешать обратный трафик
- **Allow и deny** rules
- Numbered rules (evaluated in order)

Используются реже, обычно когда нужно блокировать конкретные IP на уровне всей subnet. Большинство случаев — Security Groups достаточно.

### Best practices

- SG для каждого "tier" (web, app, db) с явными правилами
- **Reference SG** в правилах (вместо CIDR): "разрешить вход с web-sg" — лучше чем "разрешить с 10.0.1.0/24"
- Default deny, explicit allow
- Не открывать порты в `0.0.0.0/0` без необходимости (особенно SSH 22, RDP 3389)

---

## IAM — управление доступом

**IAM (Identity and Access Management)** — кто что может делать в твоём AWS аккаунте.

### Базовые сущности

**Users** — люди или приложения с long-term credentials (access key + secret).

**Groups** — наборы users. Удобно для управления permission.

**Roles** — assumed identity. Не имеет long-term credentials. Можно "приняться":
- EC2 instance role — EC2 запускается с ролью, получает temporary credentials автоматически
- Lambda role
- Cross-account role — пользователь из другого аккаунта может assume

**Policies** — JSON документы, описывающие разрешения.

### Пример policy

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject"
      ],
      "Resource": "arn:aws:s3:::my-bucket/*"
    },
    {
      "Effect": "Deny",
      "Action": "s3:DeleteObject",
      "Resource": "*"
    }
  ]
}
```

**Структура statement:**
- **Effect**: Allow или Deny
- **Action**: какие API calls (можно wildcards: `s3:*`)
- **Resource**: на какие ресурсы (ARN)
- **Condition** (optional): когда применяется

### Best practices

**1. Никогда не используй root account для daily work.**
Root — для billing settings и emergency. Создай IAM user с MFA для всего остального.

**2. Принцип least privilege.**
Не давай `AdministratorAccess` всем. Давай минимум что нужно для роли.

**3. Используй IAM roles вместо access keys.**
- EC2 → IAM Role (не embed credentials в код)
- Lambda → execution role
- Cross-account → assume role
- Federation для users (SSO с Okta, Google Workspace)

**4. Rotate access keys регулярно.**

**5. MFA для всех users.** Особенно для тех с привилегированными правами.

**6. IAM Access Analyzer** — находит publicly accessible ресурсы и unused permissions.

### Permission boundaries

Ограничение **maximum** permissions, который можно назначить. Полезно для:
- Devops self-service: разработчики могут создавать roles, но не выйти за boundary
- Multi-team environments

### Аналитика и audit

- **CloudTrail** — log всех API calls (кто что делал когда)
- **Access Analyzer** — кто имеет доступ куда
- **IAM Credential Report** — статус всех access keys и MFA

---

## S3 — объектное хранилище

**S3 (Simple Storage Service)** — массовое объектное хранилище. Файлы (объекты) в bucket'ах. Один из старейших и надёжнейших AWS сервисов.

### Базовая модель

- **Bucket** — контейнер для объектов. Имя глобально уникально по всему AWS.
- **Object** — файл + metadata. Идентифицируется ключом (фактически путём).
- **Region** — bucket принадлежит региону.

```
s3://my-bucket/users/alice/avatar.jpg
        ↑           ↑
      bucket    object key
```

### Storage classes

| Class | Цена storage | Цена retrieval | Use case |
|---|---|---|---|
| **Standard** | $0.023/GB | бесплатно | Active data |
| **Intelligent-Tiering** | автомат. | бесплатно | Unknown access pattern |
| **Standard-IA** (Infrequent Access) | $0.0125/GB | $0.01/GB | Backups |
| **One Zone-IA** | $0.01/GB | $0.01/GB | Re-creatable data |
| **Glacier Instant Retrieval** | $0.004/GB | $0.03/GB | Archives, occasional access |
| **Glacier Flexible** | $0.0036/GB | минуты-часы | Archives |
| **Glacier Deep Archive** | $0.00099/GB | 12+ часов | Long-term archive |

Lifecycle rules: автоматически переносить старые данные в более дешёвый класс.

### Использование

```go
// AWS SDK v2 в Go
import "github.com/aws/aws-sdk-go-v2/service/s3"

client := s3.NewFromConfig(cfg)

// Upload
_, err := client.PutObject(ctx, &s3.PutObjectInput{
    Bucket: aws.String("my-bucket"),
    Key:    aws.String("users/alice/avatar.jpg"),
    Body:   bytes.NewReader(imageData),
    ContentType: aws.String("image/jpeg"),
})

// Download
resp, err := client.GetObject(ctx, &s3.GetObjectInput{
    Bucket: aws.String("my-bucket"),
    Key:    aws.String("users/alice/avatar.jpg"),
})
defer resp.Body.Close()
data, _ := io.ReadAll(resp.Body)
```

### Presigned URLs

Дают временный доступ к private object без AWS credentials у клиента — и на скачивание, и на загрузку. Главный смысл для upload: байты файла **не идут через backend**, клиент кладёт их прямо в S3.

**Download (presigned GET):**

```go
presignClient := s3.NewPresignClient(client)
presigned, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
    Bucket: aws.String("my-bucket"),
    Key:    aws.String("private/file.pdf"),
}, s3.WithPresignExpires(15*time.Minute))

// presigned.URL — можно отдать пользователю, файл доступен 15 минут
```

**Upload напрямую, без proxy через сервер (presigned PUT):**

```go
presigned, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
    Bucket:      aws.String("my-bucket"),
    Key:         aws.String("uploads/alice/uuid.jpg"), // ключ генерит backend, не клиент
    ContentType: aws.String("image/jpeg"),
}, s3.WithPresignExpires(15*time.Minute))

// presigned.URL отдаём клиенту → он сам делает PUT в S3
```

**Как работает подпись.** Presigned URL — это обычный URL объекта плюс query-параметры, в которых зашита **HMAC-SHA256 подпись** запроса твоим secret access key (схема SigV4, `AWS4-HMAC-SHA256`). При загрузке S3 пересчитывает подпись своим экземпляром секрета и сверяет. Сам secret клиенту не передаётся — только производная подпись, жёстко привязанная к bucket + key + method + сроку (`X-Amz-Expires`). Поэтому ссылку нельзя переиспользовать для другого файла или после истечения.

**Поток для upload:** backend (1) проверяет права и генерит уникальный key, (2) выдаёт presigned PUT, (3) клиент грузит напрямую в S3; (4) о завершении backend узнаёт из **S3 Event Notification** (`s3:ObjectCreated:*` → SQS/SNS/Lambda), а не от клиента; (5) async-воркер валидирует/обрабатывает. Размер у presigned **PUT** не ограничить — если нужен лимит, берут **presigned POST** с policy `["content-length-range", 0, 10485760]`, и S3 сам отклонит превышение. Flow целиком: [File Upload Flow](../../05-system-design/external-request-flows/05-file-upload-and-background-processing-flow.md).

### Подпись в других облаках (GCS, Azure)

Идея — временный подписанный доступ + прямая загрузка клиент↔хранилище мимо backend — одинакова везде. Отличается **криптопримитив подписи**:

| Облако | Чем подписывается | Алгоритм |
|---|---|---|
| **AWS S3** | секретный ключ (shared secret) | HMAC-SHA256, `AWS4-HMAC-SHA256` (SigV4) |
| **GCP GCS** | приватный RSA-ключ сервис-аккаунта (асимметрично) | RSA-SHA256, `GOOG4-RSA-SHA256` (V4) |
| **Azure Blob** | account key или user-delegation key | SAS-токен (HMAC) |

Практическая разница с AWS:
- **GCS** по умолчанию подписывает **приватным ключом сервис-аккаунта**, а не общим секретом. Плюс: можно подписывать через IAM API `signBlob`, **не держа приватный ключ в сервисе** (ключ не покидает Google). В Go — `bucket.SignedURL(obj, &storage.SignedURLOptions{Method, Expires, Scheme: storage.SigningSchemeV4, GoogleAccessID, PrivateKey|SignBytes})`. Для S3-совместимости GCS также умеет **HMAC-ключи** (`GOOG4-HMAC-SHA256`) — тогда механика как у AWS.
- **Azure** использует **SAS** (Shared Access Signature) — токен с правами и сроком, подписанный account key либо user-delegation key (через Entra ID).
- **Resumable/большие файлы:** S3 — multipart upload; GCS — resumable session URI (POST инициирует, возвращает session URI); цель та же.

Итог: SDK и код разные, но архитектурный паттерн переносится один-в-один — выдать подписанную ссылку, клиент грузит напрямую, дальше async-обработка.

### Особенности

- **Strong consistency** (с 2020) — после write следующий read увидит новые данные
- **Eventual consistency** — версионирование, кросс-регион репликация
- **Versioning** — хранить все версии объектов
- **Encryption at rest** — SSE-S3 (managed by AWS), SSE-KMS (own keys), SSE-C (customer keys)
- **Encryption in transit** — HTTPS обязательно
- **Multi-part upload** — файлы > 100 MB лучше разбивать на parts (parallel + retry)
- **Pre-signed POST** — для browser uploads (без proxy)

### Private vs Public

**Public access blocking** — по умолчанию все bucket'ы приватные. Это **хорошо**. Большинство утечек "AWS S3 bucket exposed" — из-за случайно открытого доступа.

Перед открытием bucket'а — спроси себя: реально ли нужен public access? Часто лучше использовать **CloudFront перед S3** с access control.

---

## RDS — managed реляционные БД

**RDS (Relational Database Service)** — managed Postgres, MySQL, MariaDB, Oracle, SQL Server.

AWS управляет:
- Установка и обновление engine
- Backups (automatic snapshots)
- Patching
- Replication (read replicas, multi-AZ)
- Monitoring

Ты управляешь:
- Schema и queries
- Размер instance
- Storage type и size

### Multi-AZ

Standard production setup: primary + standby в другой AZ, синхронная репликация. При failover (через ~60-120 секунд) standby становится primary.

**Цена:** примерно × 2 от single-AZ.

### Read replicas

Async replicas (до 5) для read scaling. Можно cross-region для disaster recovery или low-latency reads ближе к пользователям.

### Aurora

Postgres/MySQL-совместимая БД, переписанная AWS:
- Storage отдельно от compute (replicated 6x across 3 AZ)
- Быстрее failover (~30 сек)
- Up to 15 read replicas
- Continuous backup без impact на performance
- Aurora Serverless — auto-scaling

Дороже обычной RDS, но для критичных production — обычно выбор по умолчанию.

### Когда использовать RDS

- Когда нужна PostgreSQL/MySQL без оверхеда self-hosted
- Production-grade backups и failover из коробки
- Не нужны кастомные extensions, недоступные в RDS

**Когда self-hosted:**
- Need superuser access
- Specific extensions недоступные в RDS
- Cost optimization (RDS дороже EC2 + manual)

---

## DynamoDB — managed NoSQL

**DynamoDB** — managed key-value / document БД. Сильно отличается от RDS.

### Особенности

- **Fully managed** — нет instance size, AWS сам масштабирует
- **Single-digit ms latency** даже на больших данных
- **Schemaless** — гибкая структура items
- **Provisioned vs On-demand** capacity
- **Global tables** — multi-region replication

### Когда использовать

- High-throughput key-value access (миллионы requests/sec)
- Unpredictable load (serverless с on-demand)
- Strict latency requirements
- Hot keys / specific access patterns

### Когда НЕ использовать

- Complex queries с JOIN, ad-hoc analytics — DynamoDB этого не умеет
- Сильно реляционные данные
- Когда нужны транзакции на много items (есть, но дорогие и ограниченные)

DynamoDB требует переосмысления модели данных — это не "drop-in" replace для Postgres.

---

## SQS — очереди сообщений

**SQS (Simple Queue Service)** — managed queue для async обработки.

### Типы очередей

**Standard queue:**
- At-least-once delivery
- Дубликаты возможны
- Best-effort ordering
- Unlimited throughput
- Cheap

**FIFO queue:**
- Exactly-once delivery
- Strict ordering
- 3000 messages/sec (с batching)
- Дороже Standard

### Использование

```go
// AWS SDK v2
client := sqs.NewFromConfig(cfg)

// Send
_, _ = client.SendMessage(ctx, &sqs.SendMessageInput{
    QueueUrl:    aws.String(queueURL),
    MessageBody: aws.String(`{"user_id": 42, "action": "send_email"}`),
})

// Receive (long polling — до 20 секунд)
resp, _ := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
    QueueUrl:            aws.String(queueURL),
    MaxNumberOfMessages: 10,
    WaitTimeSeconds:     20,  // long poll
    VisibilityTimeout:   30,  // секунд "невидимости" после receive
})

for _, msg := range resp.Messages {
    // Обработать сообщение
    processMessage(*msg.Body)

    // Delete после успешной обработки (иначе вернётся в очередь)
    client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
        QueueUrl:      aws.String(queueURL),
        ReceiptHandle: msg.ReceiptHandle,
    })
}
```

### Visibility timeout

Когда worker получает message, оно становится "невидимым" для других worker'ов на N секунд. Если worker не успел обработать и удалить — message вернётся в очередь.

**Если processing медленный — выставь больший timeout** или периодически вызывай `ChangeMessageVisibility` для extension.

### Dead Letter Queues (DLQ)

Если message обрабатывался N раз и всё ещё fails — отправляется в DLQ. Не теряем плохие messages, можем разбираться вручную.

### SQS vs Kafka vs RabbitMQ

| | SQS | Kafka | RabbitMQ |
|---|---|---|---|
| Управление | Managed | Self-host или MSK | Self-host или managed |
| Throughput | Очень высокий | Очень высокий | Средний |
| Order guarantees | FIFO опция | По partition | Через exchange settings |
| Retention | до 14 дней | Конфигурируется (дни-навсегда) | Пока не consumed |
| Replay | Нет | Да | Нет |
| Use case | Task queue | Event streaming, audit | Complex routing |

SQS — простой "send messages, work через workers". Не Kafka.

---

## SNS — pub/sub и notifications

**SNS (Simple Notification Service)** — pub/sub. Publisher отправляет в topic, subscriber'ы получают.

Subscribers могут быть:
- SQS queue (fan-out pattern)
- Lambda function
- HTTP endpoint
- Email / SMS
- Mobile push

### Fan-out pattern

```
Publisher: SendMessage to SNS topic "order-created"
                ↓
   ┌────────────┼────────────┐
   ↓            ↓            ↓
SQS: email    SQS: stats   Lambda: notify
worker        worker        admins
```

Один event → много consumers, каждый со своей очередью и speed.

---

## Lambda — serverless

**AWS Lambda** — run code без management сервера. Платишь за выполнение (invocations + compute time).

### Особенности

- **No servers to manage** — AWS поднимает контейнеры по требованию
- **Auto-scaling** — от 0 до тысяч параллельных executions
- **Pay per use** — только за реальные выполнения (100ms billing granularity)
- **Lots of triggers** — HTTP (через API Gateway), S3 events, SQS, DynamoDB streams, CloudWatch schedule, etc.

### Лимиты

- Max execution time: **15 минут**
- Memory: 128 MB - 10 GB
- Package size: 50 MB zipped (250 MB с layers), 10 GB через container image
- Concurrent executions per account: ~1000 (по умолчанию, можно увеличить)
- **Cold start** — первый запуск после простоя ~100-1000 мс (Go быстрее Python/Java)

### Когда использовать Lambda

- **Event-driven** — обработка S3 uploads, processing queue messages
- **Sporadic traffic** — мало запросов, нет смысла держать EC2
- **API for prototypes / low-traffic** — Lambda + API Gateway быстро и дёшево
- **Cron jobs** — Lambda + EventBridge schedule

### Когда НЕ использовать

- **High-latency-sensitive** — cold starts могут добавить 100-500 мс
- **Long-running** — 15 минут лимит
- **WebSockets** — Lambda не для держания соединений (можно через API Gateway WebSocket, но дорого)
- **High-throughput consistent load** — EC2/ECS дешевле при стабильной нагрузке

Подробнее про serverless — см. (planned) `serverless/01-edge-and-serverless.md`.

---

## EKS / ECS — оркестрация контейнеров

**ECS (Elastic Container Service)** — AWS-native контейнерный оркестратор.

**EKS (Elastic Kubernetes Service)** — managed Kubernetes.

### ECS

Проще EKS, но AWS-specific. Концепции:
- **Task** = один или несколько контейнеров, запускаются вместе
- **Service** = долгоживущий task с auto-scaling
- **Cluster** = группа compute (EC2 или Fargate)

**Fargate** — serverless compute для контейнеров (нет EC2 instance, AWS сам провижионит).

### EKS

Standard Kubernetes API, плюс AWS integrations:
- IAM auth (kubectl с AWS credentials)
- AWS Load Balancer Controller (ALB как Ingress)
- IRSA (IAM Roles for Service Accounts) — pod-level IAM permissions

Подходит когда:
- Уже знаком с Kubernetes
- Нужна portability (можно перенести на GKE/AKS)
- Используете Helm, operators, complex deployments

### Когда что

- **ECS + Fargate** — простота, AWS-native, не нужен Kubernetes
- **EKS** — стандартный Kubernetes, переносимость, ecosystem
- **EC2 raw** — legacy или специальные нужды

---

## CloudWatch — мониторинг

**CloudWatch** — AWS observability platform. Включает:

- **Metrics** — числовые метрики (CPU, memory, custom)
- **Logs** — лог aggregation (CloudWatch Logs)
- **Alarms** — алерты по threshold'ам
- **Dashboards** — визуализация
- **Insights** — query language для логов
- **X-Ray** — distributed tracing (отдельный сервис, интегрируется)

### Logs

```
Container stdout → CloudWatch Logs (через log driver)
EC2 application → CloudWatch Agent → CloudWatch Logs
Lambda → автоматически → CloudWatch Logs
```

Стоимость: ~$0.50/GB ingested + $0.03/GB-month storage. Может быть дорого для chatty логирования.

### Metrics

Стандартные метрики автоматом для большинства сервисов (EC2, RDS, ALB).

Custom metrics через API:
```go
import "github.com/aws/aws-sdk-go-v2/service/cloudwatch"

cw.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
    Namespace: aws.String("MyApp"),
    MetricData: []types.MetricDatum{
        {
            MetricName: aws.String("ProcessedOrders"),
            Value:      aws.Float64(42),
            Unit:       types.StandardUnitCount,
        },
    },
})
```

### Альтернативы

Многие команды используют **Datadog**, **New Relic**, **Grafana + Prometheus** вместо CloudWatch для метрик — больше фич, лучше UX. Логи иногда тоже идут в DataDog или OpenSearch.

CloudWatch — fallback или для базового мониторинга.

---

## Route 53 — DNS

**Route 53** — managed DNS сервис AWS.

Ключевые фичи:
- **Health checks** — мониторит endpoints
- **Weighted routing** — traffic split (canary deployments)
- **Latency-based routing** — направить к ближайшему региону
- **Geolocation routing** — направить по стране пользователя
- **Failover routing** — primary + secondary с health checks

Стоимость: $0.50/hosted zone/month + $0.40/million queries.

---

## CloudFront — CDN

**CloudFront** — CDN от AWS. Кеширует контент в edge locations по всему миру.

### Когда использовать

- **Static assets** — JS, CSS, images, video
- **API caching** — для read-heavy endpoints с long TTL
- **Защита от DDoS** — front-line firewall
- **HTTPS termination** — managed certificates через ACM

### Виды origin

- S3 bucket
- ALB / EC2
- Custom origin (любой HTTP server)
- Lambda@Edge / CloudFront Functions — code at edge

### Подводные камни

- **Cache invalidation** — $0.005 за path (после 1000 free/month). Лучше использовать versioned URLs (`app.v123.js`)
- **HTTPS только через ACM cert** в us-east-1 (даже если CloudFront global)
- **Origin Shield** — дополнительный кэш слой для origin protection

---

## ELB / ALB / NLB — load balancers

Три типа load balancer'ов в AWS:

| | ALB | NLB | CLB (legacy) |
|---|---|---|---|
| Layer | 7 (HTTP) | 4 (TCP/UDP) | 4 и 7 |
| Path-based routing | Да | Нет | Limited |
| Host-based routing | Да | Нет | No |
| WebSocket | Да | Да | Да |
| Static IP | Нет (DNS only) | Да | Нет |
| Performance | Высокая | Очень высокая | Низкая |
| Use case | Web apps, APIs | High-throughput TCP, gaming | Legacy |

**ALB (Application Load Balancer)** — для большинства HTTP-сервисов. Routing по path и host, native SSL termination, integration с Cognito/WAF.

**NLB (Network Load Balancer)** — extremely high throughput, low latency. Когда нужен static IP или millions of connections.

**Classic Load Balancer** — устаревший, не использовать в новых проектах.

---

## Secrets Manager и Parameter Store

Два способа хранить секреты в AWS:

### Secrets Manager

- Для **секретов** (passwords, API keys, DB credentials)
- **Automatic rotation** — для RDS, можно custom для остальных
- **Per-secret cost** — $0.40/month + $0.05 за 10k requests
- Подходит когда нужна ротация

### SSM Parameter Store

- Для **config + secrets**
- Standard parameters — **бесплатно** (до 10k)
- Advanced parameters (>4KB, larger limits) — $0.05/month each
- No built-in rotation
- Подходит для config и где не нужна ротация

### Использование в Go

```go
import "github.com/aws/aws-sdk-go-v2/service/secretsmanager"

sm := secretsmanager.NewFromConfig(cfg)
resp, _ := sm.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
    SecretId: aws.String("prod/db/password"),
})

var creds struct {
    Username string `json:"username"`
    Password string `json:"password"`
    Host     string `json:"host"`
}
json.Unmarshal([]byte(*resp.SecretString), &creds)
```

Best practice: **не embed credentials в код или env files**. Используй Secrets Manager или Parameter Store, читай при старте сервиса (или через sidecar).

См. также: [11-security/secrets-management/](../../11-security/secrets-management/).

---

## Managed vs self-hosted

Главное решение в AWS — что брать managed, что самим.

### Managed обычно выигрывает

- **Простые сервисы** (S3 vs self-hosted blob store) — managed просто всегда лучше
- **БД** (RDS Postgres vs EC2 Postgres) — operational overhead enormous, RDS делает 90% сам
- **Logs aggregation** (CloudWatch Logs vs ELK) — для маленьких/средних команд
- **Queues** (SQS vs Kafka) — если не нужен replay/streaming

### Self-hosted имеет смысл

- **Когда нужны фичи которых нет в managed** (Postgres extensions, custom Kafka configs)
- **Когда цена managed превышает cost самостоятельного управления**
- **Cross-cloud portability** — managed AWS привязывает к AWS

### Trade-off

```
Managed:
+ Меньше operational toil
+ Better reliability out of the box
+ Less expertise needed
- Cost premium (~30-100%)
- Vendor lock-in
- Limited customization

Self-hosted:
+ Cheaper at scale
+ Full control
+ Portability
- Need DevOps expertise
- Operational burden
- More to break
```

Правило: **начинай с managed**, переходи на self-hosted если уперся в лимиты или cost.

---

## AWS SDK в Go

```go
import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/s3"
)

func main() {
    cfg, err := config.LoadDefaultConfig(ctx,
        config.WithRegion("us-east-1"),
    )
    if err != nil { ... }

    s3Client := s3.NewFromConfig(cfg)
    // ...
}
```

### Credential resolution

AWS SDK ищет credentials в порядке:
1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
2. Shared credentials file (`~/.aws/credentials`)
3. IAM role (если на EC2/ECS/Lambda)
4. SSO

**Best practice:** на EC2/ECS/Lambda — всегда IAM role. Никогда не embed access keys в код.

### Retries и timeouts

SDK по умолчанию имеет retries с exponential backoff для idempotent операций. Для production:

```go
cfg, _ := config.LoadDefaultConfig(ctx,
    config.WithRegion("us-east-1"),
    config.WithRetryMaxAttempts(5),
)
```

---

## Полезные команды AWS CLI

```bash
# Текущая identity
aws sts get-caller-identity

# Список EC2 instances
aws ec2 describe-instances --query 'Reservations[].Instances[].[InstanceId,State.Name,Tags[?Key==`Name`]|[0].Value]'

# S3 cp/sync
aws s3 cp file.txt s3://my-bucket/
aws s3 sync ./local-dir s3://my-bucket/path/

# CloudWatch logs (tail)
aws logs tail /aws/lambda/my-function --follow

# RDS snapshot
aws rds create-db-snapshot --db-instance-identifier mydb --db-snapshot-identifier mydb-snap

# Costs
aws ce get-cost-and-usage \
  --time-period Start=2026-01-01,End=2026-01-31 \
  --granularity MONTHLY \
  --metrics UnblendedCost
```

---

См. также: [02-cloud-cost-and-architecture.md](./02-cloud-cost-and-architecture.md) — про стоимость AWS, выбор instance types, оптимизация.

## Полезные ссылки

- [AWS Documentation](https://docs.aws.amazon.com/)
- [AWS Well-Architected Framework](https://aws.amazon.com/architecture/well-architected/) — best practices
- [AWS Service Quotas](https://docs.aws.amazon.com/general/latest/gr/aws_service_limits.html) — лимиты по умолчанию
- [is.gd/AWS](https://github.com/donnemartin/awesome-aws) — большой список AWS resources
- [AWS Builders' Library](https://aws.amazon.com/builders-library/) — Amazon engineering articles
- [Last Week in AWS](https://www.lastweekinaws.com/) — Corey Quinn, news & snark
