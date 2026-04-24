# Cloud Pub/Sub: Google Cloud и AWS SNS+SQS

Cloud-managed messaging — no-ops альтернатива self-hosted Kafka/RabbitMQ. Платишь больше за каждое сообщение, но не управляешь инфраструктурой.

---

## Google Cloud Pub/Sub

### Архитектура: Topics, Subscriptions, Ack Deadline

```
Publisher
    │
    ▼
[Topic: "orders"]
    │
    ├── Subscription A ("fulfillment-service")  ← Pull или Push
    ├── Subscription B ("analytics-service")    ← независимая
    └── Subscription C ("notifications")        ← dead letter topic
```

**Topic** — канал публикации сообщений.

**Subscription** — подписка на topic. Каждая subscription получает **копию** каждого сообщения (at-least-once delivery). Разные subscription — независимы (как Kafka consumer groups).

**Ack Deadline** — время на обработку сообщения (default 10 сек, max 600 сек). Если не подтвердить за deadline — сообщение будет доставлено повторно.

### Go клиент

```go
import "cloud.google.com/go/pubsub"

// Publisher
func publishMessage(ctx context.Context, projectID, topicID string, data []byte) error {
    client, err := pubsub.NewClient(ctx, projectID)
    if err != nil {
        return err
    }
    defer client.Close()
    
    topic := client.Topic(topicID)
    result := topic.Publish(ctx, &pubsub.Message{
        Data: data,
        Attributes: map[string]string{
            "event_type": "order.created",
            "version":    "v1",
        },
    })
    
    // Ждём подтверждения (можно async через result.Get())
    _, err = result.Get(ctx)
    return err
}

// Subscriber (Pull)
func subscribeMessages(ctx context.Context, projectID, subID string) error {
    client, _ := pubsub.NewClient(ctx, projectID)
    defer client.Close()
    
    sub := client.Subscription(subID)
    sub.ReceiveSettings.MaxOutstandingMessages = 10 // prefetch
    
    return sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
        // Обработка
        if err := processOrder(ctx, msg.Data); err != nil {
            msg.Nack() // повторная доставка
            return
        }
        msg.Ack() // подтверждение
    })
}
```

### Dead Letter Topic

```go
// При создании subscription через API/gcloud/Terraform
// Сообщения попадают в DLT после N неудачных доставок
// Затем нужна отдельная subscription на DLT для анализа
```

```yaml
# gcloud CLI
gcloud pubsub subscriptions create fulfillment-sub \
  --topic=orders \
  --dead-letter-topic=orders-dlq \
  --max-delivery-attempts=5 \
  --ack-deadline=60
```

---

## AWS SNS + SQS: fan-out паттерн

### Архитектура

```
Publisher
    │
    ▼
[SNS Topic: "OrderEvents"]
    │
    ├── SQS Queue "fulfillment-queue"    → Consumer A
    ├── SQS Queue "analytics-queue"     → Consumer B
    └── SQS Queue "notifications-queue" → Consumer C
```

**SNS** (Simple Notification Service) — fan-out: publish одно сообщение → доставить в несколько SQS очередей.

**SQS** (Simple Queue Service) — очередь. Каждый consumer читает из своей очереди независимо.

Это аналог Kafka Consumer Groups: одна публикация → несколько независимых consumer groups.

### Go клиент (AWS SDK v2)

```go
import (
    "github.com/aws/aws-sdk-go-v2/service/sns"
    "github.com/aws/aws-sdk-go-v2/service/sqs"
)

// Публикация в SNS
func publishSNS(ctx context.Context, client *sns.Client, topicARN string, message []byte) error {
    _, err := client.Publish(ctx, &sns.PublishInput{
        TopicArn: &topicARN,
        Message:  aws.String(string(message)),
        MessageAttributes: map[string]types.MessageAttributeValue{
            "eventType": {
                DataType:    aws.String("String"),
                StringValue: aws.String("order.created"),
            },
        },
    })
    return err
}

// Получение из SQS
func consumeSQS(ctx context.Context, client *sqs.Client, queueURL string) error {
    for {
        result, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
            QueueUrl:            &queueURL,
            MaxNumberOfMessages: 10,
            WaitTimeSeconds:     20,  // long polling
            VisibilityTimeout:   30,  // время на обработку
        })
        if err != nil {
            return err
        }
        
        for _, msg := range result.Messages {
            if err := processMessage(ctx, []byte(*msg.Body)); err != nil {
                // Не удаляем — вернётся в очередь после VisibilityTimeout
                continue
            }
            // Успех: удаляем
            client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
                QueueUrl:      &queueURL,
                ReceiptHandle: msg.ReceiptHandle,
            })
        }
    }
}
```

### SQS Dead Letter Queue

```go
// При создании SQS через Terraform/CloudFormation:
// maxReceiveCount = 3 → после 3 неудачных попыток → DLQ
resource "aws_sqs_queue" "fulfillment_dlq" {
  name = "fulfillment-dlq"
}

resource "aws_sqs_queue" "fulfillment" {
  name = "fulfillment"
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.fulfillment_dlq.arn
    maxReceiveCount     = 3
  })
}
```

---

## Cloud vs Self-hosted

| | Cloud (GCP/AWS) | Self-hosted (Kafka/RabbitMQ) |
|---|---|---|
| Операционная нагрузка | Минимальная | Высокая |
| Стоимость | Pay-per-message (дороже при масштабе) | Дешевле при больших объёмах |
| Vendor lock-in | ✅ | ❌ |
| SLA | ≥ 99.9% | Зависит от тебя |
| Настройка | Минимальная | Полный контроль |
| Retention | До 7 дней (GCP), 4 дней (SQS) | Дни/недели/месяцы |
| Throughput | Авто-масштабирование | Ручной scaling |

**Cloud предпочтительнее когда:**
- Стартап / небольшая команда без Kafka expertise
- Не нужен долгосрочный replay (> 7 дней)
- Уже в AWS/GCP — меньше integration работы

**Self-hosted предпочтительнее когда:**
- Большой объём (сотни миллионов сообщений/день)
- Нужен replay > 7 дней
- Строгий data residency / compliance
- Команда имеет Kafka expertise

---

## Interview-ready answer

**Q: Чем SNS+SQS отличается от Kafka?**

SNS+SQS — managed сервисы с fan-out (SNS) и очередями (SQS), нет replay, retention до 4 дней. Kafka — distributed log с retention неделями/месяцами, consumer groups, replay с любого offset. SNS+SQS проще операционно, дороже при масштабе; Kafka сложнее, но гибче и дешевле при больших объёмах.
