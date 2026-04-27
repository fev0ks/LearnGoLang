# Тестирование Kafka и брокеров сообщений

Kafka тестируется через testcontainers — реальный брокер в Docker. Для быстрых тестов логики producer/consumer код изолируется через интерфейсы.

## Содержание

- [testcontainers Kafka](#testcontainers-kafka)
- [Тестирование producer'а](#тестирование-producerа)
- [Тестирование consumer'а](#тестирование-consumerа)
- [Тестирование outbox паттерна](#тестирование-outbox-паттерна)
- [Тестирование dead letter queue](#тестирование-dead-letter-queue)
- [Unit-тест логики без брокера](#unit-тест-логики-без-брокера)

---

## testcontainers Kafka

```go
//go:build integration

package messaging_test

import (
    "context"
    "log"
    "os"
    "testing"

    "github.com/testcontainers/testcontainers-go"
    tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
)

var testBroker string

func TestMain(m *testing.M) {
    ctx := context.Background()

    kc, err := tckafka.Run(ctx,
        "confluentinc/cp-kafka:7.6.1",
        tckafka.WithClusterID("test-cluster"),
    )
    if err != nil {
        log.Fatalf("start kafka container: %v", err)
    }
    defer testcontainers.TerminateContainer(kc)

    brokers, err := kc.Brokers(ctx)
    if err != nil {
        log.Fatalf("get brokers: %v", err)
    }
    testBroker = brokers[0]

    os.Exit(m.Run())
}
```

### Вспомогательные функции

```go
import (
    "github.com/twmb/franz-go/pkg/kadm"
    "github.com/twmb/franz-go/pkg/kgo"
)

// Создать топик перед тестом
func createTopic(t *testing.T, broker, topic string, partitions int32) {
    t.Helper()
    ctx := context.Background()

    client, err := kgo.NewClient(kgo.SeedBrokers(broker))
    require.NoError(t, err)
    defer client.Close()

    admin := kadm.NewClient(client)
    _, err = admin.CreateTopics(ctx, partitions, 1, nil, topic)
    require.NoError(t, err)

    t.Cleanup(func() {
        admin.DeleteTopics(context.Background(), topic)
    })
}

// Прочитать N сообщений из топика с таймаутом
func consumeN(t *testing.T, broker, topic string, n int, timeout time.Duration) []kgo.Record {
    t.Helper()
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    client, err := kgo.NewClient(
        kgo.SeedBrokers(broker),
        kgo.ConsumeTopics(topic),
        kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
    )
    require.NoError(t, err)
    defer client.Close()

    var records []kgo.Record
    for len(records) < n {
        fetches := client.PollFetches(ctx)
        if fetches.IsClientClosed() || ctx.Err() != nil {
            break
        }
        fetches.EachRecord(func(r *kgo.Record) {
            records = append(records, *r)
        })
    }
    return records
}
```

---

## Тестирование producer'а

```go
func TestOrderEventProducer_PublishOrderCreated(t *testing.T) {
    topic := "orders.events"
    createTopic(t, testBroker, topic, 1)

    client, err := kgo.NewClient(kgo.SeedBrokers(testBroker))
    require.NoError(t, err)
    t.Cleanup(func() { client.Close() })

    producer := NewOrderEventProducer(client, topic)

    event := OrderCreatedEvent{
        OrderID:    "order-123",
        CustomerID: "customer-456",
        TotalAmount: 1500,
        Currency:   "USD",
    }

    err = producer.Publish(context.Background(), event)
    require.NoError(t, err)

    // Прочитать сообщение из Kafka и проверить
    records := consumeN(t, testBroker, topic, 1, 10*time.Second)
    require.Len(t, records, 1)

    var got OrderCreatedEvent
    require.NoError(t, json.Unmarshal(records[0].Value, &got))

    assert.Equal(t, event.OrderID, got.OrderID)
    assert.Equal(t, event.CustomerID, got.CustomerID)
    assert.Equal(t, event.TotalAmount, got.TotalAmount)
}

func TestOrderEventProducer_KeyByOrderID(t *testing.T) {
    topic := "orders.events.keyed"
    createTopic(t, testBroker, topic, 3)  // несколько партиций

    client, err := kgo.NewClient(kgo.SeedBrokers(testBroker))
    require.NoError(t, err)
    t.Cleanup(func() { client.Close() })

    producer := NewOrderEventProducer(client, topic)

    // Несколько событий с одним OrderID должны попасть в одну партицию
    orderID := "order-123"
    for i := 0; i < 3; i++ {
        err = producer.Publish(context.Background(), OrderCreatedEvent{OrderID: orderID})
        require.NoError(t, err)
    }

    records := consumeN(t, testBroker, topic, 3, 10*time.Second)
    require.Len(t, records, 3)

    partitions := make(map[int32]struct{})
    for _, r := range records {
        partitions[r.Partition] = struct{}{}
    }
    assert.Len(t, partitions, 1, "all messages with same key should go to same partition")
}
```

---

## Тестирование consumer'а

```go
type OrderConsumer struct {
    svc OrderService
}

func (c *OrderConsumer) Handle(ctx context.Context, record *kgo.Record) error {
    var event OrderCreatedEvent
    if err := json.Unmarshal(record.Value, &event); err != nil {
        return fmt.Errorf("unmarshal: %w", err)
    }
    return c.svc.ProcessOrder(ctx, event)
}

func TestOrderConsumer_Handle_Success(t *testing.T) {
    topic := "orders.in"
    createTopic(t, testBroker, topic, 1)

    ctrl := gomock.NewController(t)
    svc := NewMockOrderService(ctrl)

    consumer := &OrderConsumer{svc: svc}

    event := OrderCreatedEvent{OrderID: "order-123", TotalAmount: 500}
    payload, _ := json.Marshal(event)

    svc.EXPECT().
        ProcessOrder(gomock.Any(), gomock.Cond(func(e OrderCreatedEvent) bool {
            return e.OrderID == "order-123"
        })).
        Return(nil).
        Times(1)

    record := &kgo.Record{
        Topic: topic,
        Value: payload,
    }
    err := consumer.Handle(context.Background(), record)
    require.NoError(t, err)
}

// Полный цикл: publish + consume
func TestOrderPipeline_EndToEnd(t *testing.T) {
    topic := "orders.pipeline"
    createTopic(t, testBroker, topic, 1)

    // Producer: опубликовать событие
    producerClient, _ := kgo.NewClient(kgo.SeedBrokers(testBroker))
    t.Cleanup(func() { producerClient.Close() })

    producer := NewOrderEventProducer(producerClient, topic)
    require.NoError(t, producer.Publish(context.Background(), OrderCreatedEvent{
        OrderID: "order-e2e", TotalAmount: 200,
    }))

    // Consumer: прочитать и обработать
    processed := make(chan OrderCreatedEvent, 1)

    ctrl := gomock.NewController(t)
    svc := NewMockOrderService(ctrl)
    svc.EXPECT().ProcessOrder(gomock.Any(), gomock.Any()).
        DoAndReturn(func(_ context.Context, e OrderCreatedEvent) error {
            processed <- e
            return nil
        })

    consumerClient, _ := kgo.NewClient(
        kgo.SeedBrokers(testBroker),
        kgo.ConsumeTopics(topic),
        kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
    )
    t.Cleanup(func() { consumerClient.Close() })

    consumer := &OrderConsumer{svc: svc}

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    go func() {
        fetches := consumerClient.PollFetches(ctx)
        fetches.EachRecord(func(r *kgo.Record) {
            consumer.Handle(ctx, r)
        })
    }()

    select {
    case event := <-processed:
        assert.Equal(t, "order-e2e", event.OrderID)
    case <-ctx.Done():
        t.Fatal("timeout waiting for message to be processed")
    }
}
```

---

## Тестирование outbox паттерна

Outbox: события сохраняются в БД в одной транзакции с основными данными. Отдельный relay читает outbox и публикует в Kafka.

```go
func TestOutboxRelay_PublishesAndMarksSent(t *testing.T) {
    topic := "orders.outbox"
    createTopic(t, testBroker, topic, 1)
    truncate(t, testPool, "outbox_messages")

    // Вставить событие в outbox
    _, err := testPool.Exec(context.Background(), `
        INSERT INTO outbox_messages (id, aggregate_type, aggregate_id, event_type, payload, created_at)
        VALUES ($1, 'order', $2, 'order.created', $3, NOW())
    `, uuid.New(), "order-123", `{"order_id": "order-123"}`)
    require.NoError(t, err)

    // Запустить relay
    producerClient, _ := kgo.NewClient(kgo.SeedBrokers(testBroker))
    t.Cleanup(func() { producerClient.Close() })

    relay := NewOutboxRelay(testPool, producerClient, topic)

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    require.NoError(t, relay.ProcessPending(ctx))

    // Проверить что сообщение опубликовано в Kafka
    records := consumeN(t, testBroker, topic, 1, 5*time.Second)
    require.Len(t, records, 1)

    var payload map[string]any
    require.NoError(t, json.Unmarshal(records[0].Value, &payload))
    assert.Equal(t, "order-123", payload["order_id"])

    // Проверить что запись в outbox помечена как отправленная
    var sentAt *time.Time
    err = testPool.QueryRow(context.Background(),
        `SELECT sent_at FROM outbox_messages WHERE aggregate_id = $1`, "order-123",
    ).Scan(&sentAt)
    require.NoError(t, err)
    assert.NotNil(t, sentAt, "outbox message should be marked as sent")
}
```

---

## Тестирование dead letter queue

```go
func TestOrderConsumer_SendsToDLQ_OnProcessingError(t *testing.T) {
    mainTopic := "orders.main"
    dlqTopic := "orders.dlq"
    createTopic(t, testBroker, mainTopic, 1)
    createTopic(t, testBroker, dlqTopic, 1)

    // Consumer которому сервис всегда возвращает ошибку
    ctrl := gomock.NewController(t)
    svc := NewMockOrderService(ctrl)
    svc.EXPECT().ProcessOrder(gomock.Any(), gomock.Any()).
        Return(errors.New("processing failed")).
        AnyTimes()

    dlqClient, _ := kgo.NewClient(kgo.SeedBrokers(testBroker))
    t.Cleanup(func() { dlqClient.Close() })

    consumer := NewOrderConsumerWithDLQ(svc, dlqClient, dlqTopic)

    event := OrderCreatedEvent{OrderID: "bad-order"}
    payload, _ := json.Marshal(event)

    record := &kgo.Record{Topic: mainTopic, Value: payload}
    err := consumer.Handle(context.Background(), record)
    require.NoError(t, err, "consumer should not return error — message goes to DLQ")

    // Проверить что сообщение оказалось в DLQ
    dlqRecords := consumeN(t, testBroker, dlqTopic, 1, 5*time.Second)
    require.Len(t, dlqRecords, 1)

    // DLQ-сообщение должно содержать оригинальный payload
    assert.Equal(t, payload, dlqRecords[0].Value)

    // И метаданные об ошибке
    for _, header := range dlqRecords[0].Headers {
        if string(header.Key) == "x-error" {
            assert.Contains(t, string(header.Value), "processing failed")
        }
    }
}
```

---

## Unit-тест логики без брокера

Логику обработки сообщений можно тестировать через fake publisher — без Kafka.

```go
// Интерфейс брокера — изолирует бизнес-логику от Kafka
type EventPublisher interface {
    Publish(ctx context.Context, topic string, key, value []byte) error
}

// Fake для unit-тестов
type spyPublisher struct {
    mu       sync.Mutex
    messages []publishedMessage
    err      error
}

type publishedMessage struct {
    Topic string
    Key   []byte
    Value []byte
}

func (s *spyPublisher) Publish(ctx context.Context, topic string, key, value []byte) error {
    if s.err != nil {
        return s.err
    }
    s.mu.Lock()
    s.messages = append(s.messages, publishedMessage{topic, key, value})
    s.mu.Unlock()
    return nil
}

func TestOrderService_Checkout_PublishesEvent(t *testing.T) {
    pub := &spyPublisher{}
    repo := newFakeOrderRepo()
    svc := NewOrderService(repo, pub)

    order, err := svc.Checkout(context.Background(), CheckoutRequest{
        CustomerID: "c1",
        ProductID:  "p1",
        Quantity:   2,
    })
    require.NoError(t, err)

    require.Len(t, pub.messages, 1)
    assert.Equal(t, "orders.events", pub.messages[0].Topic)

    var event OrderCreatedEvent
    require.NoError(t, json.Unmarshal(pub.messages[0].Value, &event))
    assert.Equal(t, order.ID, event.OrderID)
    assert.Equal(t, 2, event.Quantity)
}

func TestOrderService_Checkout_PublisherError_DoesNotFail(t *testing.T) {
    pub := &spyPublisher{err: errors.New("broker unavailable")}
    repo := newFakeOrderRepo()
    svc := NewOrderService(repo, pub)

    // Ошибка публикации не должна отменять создание заказа
    _, err := svc.Checkout(context.Background(), CheckoutRequest{
        CustomerID: "c1",
        ProductID:  "p1",
        Quantity:   1,
    })
    require.NoError(t, err, "order should be created even if publish fails")
}
```
