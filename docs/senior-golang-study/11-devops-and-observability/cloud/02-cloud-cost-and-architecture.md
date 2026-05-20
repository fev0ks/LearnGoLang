# Cloud cost и архитектурные решения

Cloud billing — это **сложная инженерная задача**, а не "оплата хостинга". В AWS можно случайно потратить $50,000 за выходные, и таких историй много. С другой стороны — те же workloads могут стоить в 3-10 раз дешевле при правильной архитектуре.

Senior backend должен думать про cost так же как про latency и reliability. Это не "финансовый вопрос", это инженерный trade-off: дёшево / быстро / надёжно — выбирай два.

## Содержание

- [Почему cloud cost непредсказуем](#почему-cloud-cost-непредсказуем)
- [Основные категории расходов](#основные-категории-расходов)
- [Pricing models — основы экономики AWS](#pricing-models-основы-экономики-aws)
- [Выбор типа EC2 instance](#выбор-типа-ec2-instance)
- [Reserved Instances и Savings Plans](#reserved-instances-и-savings-plans)
- [Spot instances](#spot-instances)
- [Storage cost](#storage-cost)
- [Network cost — самое коварное](#network-cost-самое-коварное)
- [Скрытые расходы](#скрытые-расходы)
- [Cost monitoring и alerts](#cost-monitoring-и-alerts)
- [Архитектурные решения для экономии](#архитектурные-решения-для-экономии)
- [Известные истории горьких уроков](#известные-истории-горьких-уроков)
- [Cost optimization чек-лист](#cost-optimization-чек-лист)

---

## Почему cloud cost непредсказуем

В отличие от dedicated servers ($X/месяц за машину), cloud billing — это **сумма множества метрик**:
- Compute hours × instance type
- GB-month storage × class
- GB data transfer × direction × destination
- Requests per service × tier
- Spot/On-demand/Reserved mix
- Various add-ons (NAT Gateway hours, EBS IOPS, CloudWatch Logs ingestion, ...)

Это даёт **гибкость**, но и **непредсказуемость**. Bill за месяц может вырасти на 200% без изменений в коде — если поменялся паттерн использования.

### Классический story: "S3 bucket нечаянно стал public download endpoint"

Команда забыла настроить authentication → бот нашёл публичный bucket с гигабайтами файлов → начал скачивать → AWS считает egress на $0.09/GB → за выходные счёт $20,000.

Это **не баг AWS**, это **разработчик не понял экономику**.

---

## Основные категории расходов

Типичная разбивка AWS bill для backend сервиса:

| Категория | % bill |
|---|---|
| EC2 / ECS / EKS compute | 30-50% |
| Data transfer (egress) | 10-30% |
| RDS / DynamoDB | 10-25% |
| S3 storage и requests | 5-15% |
| CloudWatch (logs + metrics) | 5-15% |
| Load balancers, NAT GW | 5-10% |
| Прочее (Lambda, SQS, etc.) | 5% |

Cost optimization начинается с **знать где деньги уходят**. AWS Cost Explorer / Cost Categories — must-have для начала.

---

## Pricing models — основы экономики AWS

### Pay-per-use

Большинство сервисов:
- Compute — за секунду или минуту работы
- Storage — за GB-month
- Network — за GB transferred
- Requests — за миллион (S3, DynamoDB)

**Implication:** ресурс стоит когда работает. Остановил EC2 → не платишь за compute (но платишь за EBS volume).

### Commitment-based discounts

- **Reserved Instances** — обязательство на 1-3 года, скидка 30-72%
- **Savings Plans** — обязательство $X/час, скидка 30-66%
- **DynamoDB Reserved Capacity** — для steady-state DynamoDB

Для **predictable baseline** load — commitments экономят значительно.

### Free tier

- 12 месяцев бесплатно: 750 hours/month t2.micro, 5 GB S3, и т.д.
- Always free: 1M Lambda invocations/month, 25 GB DynamoDB
- Хорошо для dev/staging маленьких проектов

---

## Выбор типа EC2 instance

Маленькая разница в выборе → значительная разница в счёте. Ключевые соображения:

### Generation matters

Новые поколения **дешевле и быстрее**:
- m5 → m6i → m7i (Intel)
- m6g → m7g (Graviton, ARM)

Например, `m6i.xlarge` примерно на **15-20% дешевле** при **20-30% лучшей производительности** чем `m5.xlarge`. Просто меняй generation в Terraform/CloudFormation.

### Graviton (ARM) instances

AWS Graviton — собственные ARM-процессоры:
- ~20% дешевле эквивалентных x86
- ~40% лучше price/performance для многих workloads
- Go компилирует на ARM без проблем

**Подход:** для новых сервисов — начинай с Graviton. Тестируй performance. Большинство Go-сервисов работают отлично.

**Подводные камни:**
- Native C libraries должны быть скомпилированы для ARM (CGO)
- Некоторые Docker images не имеют ARM tags
- Тестирование на dev (Intel) и prod (ARM) — следи за регрессиями

### Right-sizing

Главное правило: **не overprovisioning'уй**.

Типичные паттерны overprovisioning:
- Запустили на m5.4xlarge "чтобы быстро", забыли уменьшить → платишь $400+/мес за неиспользуемое
- CPU 5-10% utilization average — instance в 5 раз больше чем нужно
- Memory под 30% — взяли r-type (memory optimized) вместо m-type

**Используй CloudWatch metrics:**
```
CPU average < 30% → instance слишком большой
Memory average < 40% → меньше memory или другой тип
Network low → не network-intensive workload
```

AWS Compute Optimizer — automatic recommendations.

### Burstable (t-family)

`t3`, `t4g` — burstable: получают CPU credits в простое, тратят при нагрузке.

**Плюсы:** дёшево при низкой/spiky нагрузке.
**Минусы:** если credits кончились → throttling, performance падает в разы.

**Когда использовать:** dev/staging, low-traffic services. Production-grade web servers — обычно `m`-family.

`t3.unlimited` mode — не throttling, но платишь extra если кредитов мало. Хорошо для безопасного "burstable production".

### Spot capacity

Самая большая экономия — Spot (см. ниже).

---

## Reserved Instances и Savings Plans

Для baseline нагрузки — обязательно использовать commitments.

### Reserved Instances (RI)

- Обязательство на конкретный instance type в конкретном регионе
- Сроки: 1 или 3 года
- Payment: All upfront / Partial / No upfront
- Скидка: 30-72% от On-Demand

**Convertible RI:** можешь менять instance type в рамках того же семейства. Гибче, но меньше скидка.

**Минусы:**
- Привязка к instance type — если архитектура меняется, RI "пропадают"
- Менее гибко чем Savings Plans

### Savings Plans (новее, гибче)

- Обязательство на $X/час compute
- Применяется автоматически к разным instance types
- 1 или 3 года
- Скидка похожая на RI

**Compute Savings Plans** — применяются ко всему compute (EC2, Fargate, Lambda). Самые гибкие.

**EC2 Instance Savings Plans** — привязаны к семейству instances, но дешевле.

**Strategy:**
- 70-80% baseline → Savings Plans (3 года, all upfront — максимум discount)
- 20-30% variable → On-Demand
- Batch/fault-tolerant → Spot

### Стоит ли коммитить

**Правило:** если уверен что будешь использовать ресурс минимум 6 месяцев из 12 → RI окупится.

**Калькулятор break-even:**
```
On-Demand $0.10/hour × 24 × 365 = $876/year
3-year RI $0.05/hour × ... = $438/year
Break-even: ~6 месяцев constant use
```

---

## Spot instances

**Spot** — "лишние" мощности AWS, скидка **до 90%**. Но AWS может забрать в любой момент (с предупреждением 2 минуты).

### Когда подходит

- **Stateless workers** — обработка очередей, batch jobs, transcoding
- **CI/CD runners** — fail OK, retry
- **Big data jobs** — Spark, Hadoop, могут recover
- **Dev/Staging environments**
- **Auto-scaling spike capacity** — на пик подключаем Spot

### Когда НЕ подходит

- Production database (нельзя терять)
- Stateful single-instance сервисы
- Strict deadline jobs

### Spot Fleet и mixed instance types

Чтобы снизить риск capacity unavailable:
- Запрос **multiple instance types** одновременно
- AWS выбирает доступные с лучшей ценой
- Spot Fleet с маркет-based pricing

```yaml
# EKS / ECS — mixed Spot + On-Demand
node_groups:
  - capacity_type: SPOT
    instance_types: [m5.large, m5a.large, m6i.large]  # diversity!
    desired_size: 5
  - capacity_type: ON_DEMAND
    instance_types: [m5.large]
    desired_size: 2  # baseline
```

5 Spot + 2 On-Demand: если Spot забрали — деградация, но не полное падение.

### Spot interruption handling

EC2 шлёт **interruption warning** за 2 минуты:
- Через instance metadata: `http://169.254.169.254/latest/meta-data/spot/instance-action`
- Через EventBridge event

Приложение должно:
1. Получить warning
2. Завершить текущие задачи или сохранить state
3. Drain соединения
4. Graceful shutdown

В Go:
```go
// Poll metadata endpoint
go func() {
    for {
        resp, err := http.Get("http://169.254.169.254/latest/meta-data/spot/instance-action")
        if err == nil && resp.StatusCode == 200 {
            // Spot interruption coming
            initiateGracefulShutdown()
            return
        }
        time.Sleep(5 * time.Second)
    }
}()
```

---

## Storage cost

Storage обычно дешевле compute, но может неожиданно вырасти.

### S3

- **Standard** — $0.023/GB-month
- **Standard-IA** — $0.0125/GB (но retrieval $0.01/GB)
- **Glacier** — $0.004/GB (но retrieval часы)

**Lifecycle rules** — автоматический перенос:

```yaml
rules:
  - id: archive-old
    filter: { prefix: "logs/" }
    transitions:
      - days: 30
        storage_class: STANDARD_IA
      - days: 90
        storage_class: GLACIER
      - days: 365
        storage_class: DEEP_ARCHIVE
    expiration:
      days: 2555  # 7 лет
```

**Главная экономия:** не держи всё в Standard. Большинство данных через месяц не нужны "горячими".

### S3 request cost

- PUT/POST — $0.005/1000
- GET — $0.0004/1000

Кажется мало, но при high traffic — заметная статья. **Reduce requests:**
- Batch operations
- Multipart upload вместо many small PUTs
- CloudFront cache перед S3 (CloudFront reads дешевле + кэшируется)

### EBS

- **gp3** — $0.08/GB-month + $0.005/provisioned IOPS-month
- **io2** — $0.125/GB + $0.065/provisioned IOPS — для high-IOPS БД

**Подводный камень:** EBS volumes **сохраняются после terminate EC2** (если не было `DeleteOnTermination=true`). "Orphaned" EBS — обычная находка cost-аудита.

```bash
# Find unattached EBS volumes
aws ec2 describe-volumes --filters Name=status,Values=available
```

### EBS Snapshots

Incremental, но накапливаются. Если делать snapshot каждую ночь полгода — это много петабайт-часов.

**Lifecycle policies для snapshots** — обязательно. Например: keep 7 daily, 4 weekly, 12 monthly.

---

## Network cost — самое коварное

Сетевые расходы — самая частая причина "billing surprise". Главные категории:

### Egress (out of AWS)

- **AWS → Internet** — **$0.09/GB** (после 1 GB free/month, дешевле в больших объёмах)
- **AWS → AWS другой регион** — $0.02/GB
- **AWS → CloudFront → Internet** — $0.085/GB (немного дешевле)

**Сколько это?** 1 TB egress = $90. 1 PB = $90,000.

### Внутри региона

- **Same AZ** — **бесплатно** (между EC2)
- **Cross-AZ within region** — $0.01/GB (each direction!)
- **VPC Peering same region** — $0.01/GB

**Подводный камень:** RDS multi-AZ — replication между AZs **не идёт** в AWS counted egress (managed service). Но твой EC2 в AZ-a → RDS в AZ-b — это cross-AZ, считается.

### NAT Gateway

- **$0.045/hour** ~$33/month
- **$0.045/GB** data processed (!)

Если приложение в private subnet делает много исходящих запросов (S3 API, DynamoDB, downloads) — каждый GB через NAT = $0.045.

**Экономия:**
- **VPC Endpoints** для S3, DynamoDB — бесплатно для S3/DynamoDB, не через NAT
- Architecture review: что **реально** должно быть в private subnet?

### Cross-region traffic

- **Egress to another AWS region** — $0.02/GB
- **Inter-region replication** (S3, DynamoDB) — $0.02/GB

Decision making: replicate ли в другой регион → платишь cross-region.

### CloudFront

- **From CloudFront to Internet** — $0.085/GB (cheaper than direct EC2/S3 egress)
- **Origin fetch** — обычно бесплатно or low

**Reduce origin fetches через хороший cache hit rate.**

### Реальный пример

Backend-сервис на AWS:
- 10M HTTP requests/day, average 100 KB response
- = 1 TB/day egress = $90/day = $2700/month
- На 1 PB/month = $90,000/month

**Это не CPU, не RAM. Это просто отдача ответов клиентам.**

Mitigations:
- **Сжатие** (gzip/brotli) — 70-90% reduction
- **CloudFront** перед API — дешевле + кэш
- **GraphQL** — клиент запрашивает только нужное
- **Pagination** — не отдавать всё сразу
- **Image optimization** — WebP, resize on demand

---

## Скрытые расходы

Категории cost которые часто пропускают.

### CloudWatch Logs

- **Ingestion**: $0.50/GB
- **Storage**: $0.03/GB-month
- Очень chatty приложения легко генерят 100+ GB/day = $1500+/month только на логи

**Mitigations:**
- Log level filtering (не log DEBUG в prod)
- Sampling для high-volume logs (логировать 1 из 100)
- Short retention (7 дней вместо forever)
- Ship to cheaper store (S3) для long-term

### CloudWatch Metrics

- **Custom metrics**: $0.30/metric/month (10 free)
- High cardinality (метрики per-user-per-endpoint) → может быть тысячи custom metrics → expensive

Better: aggregated metrics, dimensions instead of separate metrics.

### Idle resources

- Unused EBS volumes (после terminate без DeleteOnTermination)
- Forgotten EIPs (Elastic IPs) — $0.005/hour за **unattached** EIPs
- Old AMIs and snapshots
- Idle ELBs/ALBs ($16-20/month each)
- Dev environments running 24/7

**AWS Trusted Advisor / Cost Explorer** — находит idle ресурсы.

### Data transfer to/from S3 in same region

Same-region S3 traffic — **бесплатно** (between AWS services). Но из VPC через NAT Gateway → платный $0.045/GB.

**Use VPC Endpoints для S3** — бесплатно, обходит NAT.

### Forgotten test/load environments

- Запустили load test → забыли остановить cluster
- $thousands за день

**Tagging convention** + automatic cleanup для untagged resources.

---

## Cost monitoring и alerts

### AWS Cost Explorer

Веб-интерфейс для analyzing costs:
- Trends за периоды
- Breakdown по сервису / тегам / regions
- Forecasting

### Budgets

```bash
# Создать budget с alert
aws budgets create-budget --account-id 123456789012 --budget '{
  "BudgetName": "Monthly EC2",
  "BudgetLimit": {"Amount": "1000", "Unit": "USD"},
  "TimeUnit": "MONTHLY",
  "BudgetType": "COST",
  "CostFilters": {"Service": ["Amazon Elastic Compute Cloud - Compute"]}
}'
```

Alerts: при достижении 50%, 80%, 100% от budget — email/SNS.

### Cost Anomaly Detection

ML-based anomaly detection. Шлёт alert если cost вырос аномально.

**Обязательно включить** — поможет catch'ить incidents типа "S3 bucket стал публичным" быстрее.

### Tagging

Тегируй **всё**:

```yaml
tags:
  Environment: production
  Service: payment-api
  Team: payments
  Owner: alice@example.com
  CostCenter: 12345
```

Это даёт breakdown в Cost Explorer "сколько стоит payment-api в production".

**Enforce через IAM:** запретить создавать ресурсы без тегов:

```json
{
  "Effect": "Deny",
  "Action": "ec2:RunInstances",
  "Resource": "*",
  "Condition": {
    "StringNotEqualsIfExists": {
      "aws:RequestTag/Environment": ["production", "staging", "dev"]
    }
  }
}
```

---

## Архитектурные решения для экономии

### 1. CloudFront перед всем

CloudFront — самая универсальная экономия:
- Cache reduces origin load → меньше EC2
- CloudFront egress дешевле direct
- Защита от DDoS
- Free SSL через ACM

Использовать перед S3, ALB, even Lambda.

### 2. Auto-scaling

Pay for what you use:
- Scale-out утром, scale-in ночью
- Scale-out per traffic pattern
- Stateless services scale easily

**Подводные камни:**
- Cold-start delay на scale-out
- Auto-scaling tuning сложен
- Слишком aggressive scaling = thrashing

### 3. Architecture: managed services vs DIY

Managed RDS vs self-hosted Postgres on EC2:
- RDS dearer per-instance
- Но: no DBA hire, automated backups, multi-AZ
- В большинстве случаев managed cheaper TCO

### 4. Outbound traffic patterns

- Webhooks → outbound через NAT GW = $0.045/GB
- В public subnet вместо private — нет NAT cost
- Or: dedicated egress proxy с keep-alive

### 5. Caching layers

Cache → less DB / external API calls → less cost.

- Redis для DB cache
- CloudFront для HTTP cache
- In-memory cache в приложении

### 6. Right-sized regions

- Some regions cheaper than others
- us-east-1 (cheapest)
- Asian regions ~20% more expensive
- EU GDPR может требовать EU regions

Но: cross-region latency, compliance, customer proximity.

### 7. Serverless для variable load

Если traffic spiky (например, 100 req/sec normally, 10000 на пиках):
- EC2 Auto Scaling — paying for headroom
- Lambda — pay per invocation, no idle cost

Trade-off: lambda cold start, 15-min limit, vendor lock-in.

### 8. Multi-cloud arbitrage

Use services from cheapest provider:
- AWS S3 + GCP BigQuery
- Cloudflare workers (cheaper edge compute)
- DigitalOcean droplets для baseline EC2

Но: complexity, integration overhead, multiple bills.

---

## Известные истории горьких уроков

### Story 1: $50k за выходные

Startup deployed new feature, forgot rate-limiting. Bot detected open endpoint, fired millions of requests including S3 downloads. Egress charges $50k in 48 hours. AWS refunded after long appeal, but stressful weekend.

**Lesson:** **always have billing alerts**, even at lower thresholds.

### Story 2: NAT Gateway $30k/month

Microservices архитектура, 50 services all in private subnets, делающие много API calls к AWS (CloudWatch, S3, DynamoDB, Secrets Manager) через NAT Gateway. $0.045/GB × terabytes = $30k/month just for NAT.

**Lesson:** **VPC endpoints**. After adding endpoints for S3, DynamoDB, KMS, Secrets Manager — bill dropped to $3k/month.

### Story 3: Forgotten test cluster

Engineer launched EKS cluster for load testing, scaled to 50 nodes m5.4xlarge. Forgot to delete. Discovered 3 months later. ~$120k.

**Lesson:** automatic cleanup для untagged resources, mandatory tagging, regular cost review meetings.

### Story 4: Database backups exposion

DB snapshots set on hourly schedule, retention "indefinite". Over 2 years — 17000 snapshots, $50k/month just for snapshot storage.

**Lesson:** **lifecycle policies for everything**.

### Story 5: CloudWatch Logs из debug logging

Engineer added `log.Println` для каждого incoming request с full headers and body. Service handles 10000 req/sec. Logs grew to 5 TB/day = $2500/day = $75k/month.

**Lesson:** **log sampling** for high-volume, level filtering, log rotation/expiry.

---

## Cost optimization чек-лист

### Quick wins (часто экономят 20-50%)

- [ ] Enable Cost Anomaly Detection
- [ ] Set up Budget alerts (50%, 80%, 100% of expected)
- [ ] Tag everything (environment, service, team)
- [ ] Stop/terminate dev/staging on nights and weekends
- [ ] Right-size obviously oversized instances (CPU < 30%)
- [ ] Move to Graviton (ARM) where possible
- [ ] Enable S3 lifecycle policies for old data
- [ ] Set up CloudWatch Logs retention (не "Never expire")
- [ ] Delete unused EBS volumes and EIPs

### Architectural (для значительной экономии)

- [ ] Reserved Instances or Savings Plans для baseline
- [ ] CloudFront в front of EC2/S3
- [ ] VPC Endpoints для S3, DynamoDB
- [ ] Auto-scaling для variable load
- [ ] Spot instances для batch / stateless workers
- [ ] Compression (gzip/brotli) для HTTP responses
- [ ] Pagination и filtering в API (не отдавать tons of data)
- [ ] Caching layer (Redis or local in-memory)
- [ ] Log sampling для high-volume services

### Process (long-term hygiene)

- [ ] Monthly cost review meetings
- [ ] Cost attribution to teams (via tags)
- [ ] FinOps practice — coordinate finance & engineering
- [ ] Trusted Advisor recommendations regularly review
- [ ] AWS Compute Optimizer для right-sizing
- [ ] Mandatory tagging policy enforced through IAM

### Disaster prevention

- [ ] Billing alerts at multiple thresholds
- [ ] Cost Anomaly Detection enabled
- [ ] Rate limiting on public endpoints
- [ ] S3 bucket policies (no accidental public buckets)
- [ ] Egress monitoring и alerting
- [ ] Disaster recovery plan для overcharge scenarios

---

## Полезные ссылки

- [AWS Pricing Calculator](https://calculator.aws/)
- [AWS Cost Explorer](https://aws.amazon.com/aws-cost-management/aws-cost-explorer/) — analytics
- [Last Week in AWS](https://www.lastweekinaws.com/) — Corey Quinn, cost optimization expert
- [Cloudonaut Pricing Articles](https://cloudonaut.io/aws-pricing/)
- [FinOps Foundation](https://www.finops.org/) — community и frameworks
- [Vantage](https://www.vantage.sh/) — cost management tool с good UI

---

См. также: [01-aws-core-services.md](./01-aws-core-services.md) — общий обзор AWS сервисов.
