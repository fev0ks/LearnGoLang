# Integration, Contract и E2E тесты

Integration тесты дороже unit, но именно они ловят самые болезненные production-баги: неправильный SQL, проблемы схемы, несовместимость API на границе сервисов.

## Содержание

- [Когда integration тест лучше unit](#когда-integration-тест-лучше-unit)
- [Integration тест HTTP-клиента](#integration-тест-http-клиента)
- [Contract тесты](#contract-тесты)
- [E2E тесты](#e2e-тесты)
- [Организация и разделение слоёв](#организация-и-разделение-слоёв)

---

## Когда integration тест лучше unit

**Unit test не поможет, если:**
- SQL-запрос синтаксически верный, но логически сломан
- UNIQUE constraint нарушается при определённом порядке вставки
- JSON-поле называется `created_at`, а в ответе API `createdAt`
- gRPC клиент неправильно обрабатывает streaming error
- HTTP client не обрабатывает 429 и не делает retry

```
unit tests       — бизнес-логика, branching, mapping, validation
integration      — SQL, transport, schema, сериализация
contract         — границы между сервисами
e2e              — 2-5 критичных пользовательских сценариев
```

---

## Integration тест HTTP-клиента

`httptest.NewServer` надёжнее чем mock `http.Client` — проверяется реальный HTTP-транспорт с заголовками, статус-кодами и телом ответа.

### Базовый сценарий

```go
func TestPaymentClient_Charge_Success(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Проверить что клиент правильно формирует запрос
        assert.Equal(t, "/v1/charges", r.URL.Path)
        assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
        assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

        var req ChargeRequest
        require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
        assert.Equal(t, 1500, req.Amount)

        w.WriteHeader(http.StatusCreated)
        json.NewEncoder(w).Encode(ChargeResponse{
            TransactionID: "txn-123",
            Status:        "success",
        })
    }))
    defer srv.Close()

    client := NewPaymentClient(srv.URL, "test-key")
    resp, err := client.Charge(context.Background(), ChargeRequest{Amount: 1500, Currency: "USD"})

    require.NoError(t, err)
    assert.Equal(t, "txn-123", resp.TransactionID)
}
```

### Тест retry-логики

```go
func TestPaymentClient_Retry_On503(t *testing.T) {
    attempts := 0
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        attempts++
        if attempts < 3 {
            w.WriteHeader(http.StatusServiceUnavailable)
            return
        }
        json.NewEncoder(w).Encode(ChargeResponse{TransactionID: "txn-ok"})
    }))
    defer srv.Close()

    client := NewPaymentClient(srv.URL, "key", WithMaxRetries(3), WithRetryDelay(0))
    _, err := client.Charge(context.Background(), ChargeRequest{Amount: 100})

    require.NoError(t, err)
    assert.Equal(t, 3, attempts, "should retry until success")
}

func TestPaymentClient_NoRetry_On400(t *testing.T) {
    attempts := 0
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        attempts++
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]string{"error": "invalid amount"})
    }))
    defer srv.Close()

    client := NewPaymentClient(srv.URL, "key", WithMaxRetries(3))
    _, err := client.Charge(context.Background(), ChargeRequest{Amount: -1})

    require.Error(t, err)
    assert.Equal(t, 1, attempts, "should not retry on 4xx")
}
```

### Тест таймаута

```go
func TestPaymentClient_Timeout(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(500 * time.Millisecond)  // медленный сервер
        w.WriteHeader(http.StatusOK)
    }))
    defer srv.Close()

    client := NewPaymentClient(srv.URL, "key", WithTimeout(100*time.Millisecond))
    _, err := client.Charge(context.Background(), ChargeRequest{Amount: 100})

    require.Error(t, err)
    assert.True(t, errors.Is(err, context.DeadlineExceeded) ||
        strings.Contains(err.Error(), "timeout"),
        "should return timeout error")
}
```

---

## Contract тесты

Contract тест проверяет что данные, отправляемые на границе между сервисами, соответствуют ожидаемой схеме.

### JSON response contract

```go
// Проверить что ответ API соответствует контракту который ожидают потребители
func TestUserAPI_ResponseSchema(t *testing.T) {
    h := NewUserHandler(NewUserService(newFakeUserRepo()))
    // Заранее вставить пользователя
    // ...

    req := httptest.NewRequest(http.MethodGet, "/users/u1", nil)
    rec := httptest.NewRecorder()
    h.GetUser(rec, req)

    require.Equal(t, http.StatusOK, rec.Code)
    require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

    var resp map[string]any
    require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

    // Обязательные поля — не должны исчезнуть при рефакторинге
    assert.Contains(t, resp, "id", "response must contain 'id'")
    assert.Contains(t, resp, "email", "response must contain 'email'")
    assert.Contains(t, resp, "name", "response must contain 'name'")
    assert.Contains(t, resp, "created_at", "response must contain 'created_at'")

    // Типы полей
    assert.IsType(t, "", resp["id"], "'id' must be string")
    assert.IsType(t, "", resp["email"], "'email' must be string")

    // Формат даты — ISO 8601
    createdAt, ok := resp["created_at"].(string)
    require.True(t, ok)
    _, err := time.Parse(time.RFC3339, createdAt)
    assert.NoError(t, err, "'created_at' must be RFC3339 format")
}
```

### Kafka event schema contract

```go
// Проверить что событие которое публикует сервис соответствует ожидаемой схеме
func TestOrderCreatedEvent_Schema(t *testing.T) {
    pub := &spyPublisher{}
    svc := NewOrderService(newFakeOrderRepo(), pub)

    _, err := svc.Checkout(context.Background(), CheckoutRequest{
        CustomerID: "c1",
        ProductID:  "p1",
        Quantity:   2,
    })
    require.NoError(t, err)
    require.Len(t, pub.messages, 1)

    // Десериализовать как generic map — проверить схему
    var event map[string]any
    require.NoError(t, json.Unmarshal(pub.messages[0].Value, &event))

    requiredFields := []string{"order_id", "customer_id", "total_amount", "currency", "occurred_at"}
    for _, field := range requiredFields {
        assert.Contains(t, event, field, "event must contain field: %s", field)
    }
}
```

---

## E2E тесты

E2E проходит через весь стек — от HTTP-запроса до реальной БД. Минимальный набор: 2-5 критичных сценариев.

```go
//go:build e2e

package e2e_test

// baseURL берётся из окружения — тестируем реальный деплой
var baseURL = os.Getenv("E2E_BASE_URL")

func TestUserRegistration_Flow(t *testing.T) {
    if baseURL == "" {
        t.Skip("E2E_BASE_URL not set")
    }

    client := &http.Client{Timeout: 10 * time.Second}

    // 1. Зарегистрировать пользователя
    body, _ := json.Marshal(map[string]string{
        "email":    "e2e-test@example.com",
        "password": "secret123",
        "name":     "E2E User",
    })
    resp, err := client.Post(baseURL+"/register", "application/json", bytes.NewReader(body))
    require.NoError(t, err)
    require.Equal(t, http.StatusCreated, resp.StatusCode)

    var registerResp struct {
        UserID string `json:"user_id"`
        Token  string `json:"token"`
    }
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&registerResp))
    require.NotEmpty(t, registerResp.Token)

    // 2. Получить профиль с токеном
    req, _ := http.NewRequest(http.MethodGet, baseURL+"/users/me", nil)
    req.Header.Set("Authorization", "Bearer "+registerResp.Token)

    resp, err = client.Do(req)
    require.NoError(t, err)
    require.Equal(t, http.StatusOK, resp.StatusCode)

    var profile struct {
        ID    string `json:"id"`
        Email string `json:"email"`
    }
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&profile))
    assert.Equal(t, "e2e-test@example.com", profile.Email)
    assert.Equal(t, registerResp.UserID, profile.ID)
}
```

### Изоляция E2E тестов

```go
// Уникальный email на каждый запуск — не ломаются при повторах
func uniqueEmail(t *testing.T) string {
    t.Helper()
    return fmt.Sprintf("e2e-%s@test.example.com", t.Name()[:8]+uuid.New().String()[:8])
}

func TestCheckout_Flow(t *testing.T) {
    email := uniqueEmail(t)
    // Регистрация → Добавить товар в корзину → Оформить заказ → Проверить статус заказа
}
```

---

## Организация и разделение слоёв

### Build tags

```go
// integration_test.go
//go:build integration

// e2e_test.go
//go:build e2e
```

```bash
# Только unit (pre-commit, быстро)
go test ./...

# Unit + integration (CI на PR)
go test -tags=integration ./...

# E2E (pre-deploy, staging)
go test -tags=e2e ./tests/e2e/...
```

### Структура директорий

```
.
├── internal/
│   └── user/
│       ├── service.go
│       ├── service_test.go          # unit
│       ├── repository.go
│       └── repository_test.go       # integration (//go:build integration)
└── tests/
    └── e2e/
        ├── registration_test.go     # (//go:build e2e)
        └── checkout_test.go
```

### GitHub Actions

```yaml
jobs:
  unit:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
      - run: go test -race ./...

  integration:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env: { POSTGRES_DB: testdb, POSTGRES_USER: test, POSTGRES_PASSWORD: test }
    steps:
      - run: go test -tags=integration -timeout=5m ./...

  e2e:
    runs-on: ubuntu-latest
    needs: [unit, integration]
    if: github.ref == 'refs/heads/main'
    steps:
      - run: go test -tags=e2e -timeout=10m ./tests/e2e/...
        env:
          E2E_BASE_URL: ${{ secrets.STAGING_URL }}
```
