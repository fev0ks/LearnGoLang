# Задача 4: HTTP-клиент платёжного сервиса

## Содержание

- [Формулировка](#формулировка)
- [Исходный код и контракт](#исходный-код-и-контракт)
- [Что нужно найти](#что-нужно-найти)
- [Исправленное решение](#исправленное-решение)
- [Повторы и идемпотентность](#повторы-и-идемпотентность)
- [Пример теста](#пример-теста)
- [Что проверить тестами](#что-проверить-тестами)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Это задача примерно на 30 минут. Ожидаемый результат — найти критичные ошибки,
исправить HTTP-клиент и кратко объяснить правила повторов. Полноценные метрики,
трассировка, circuit breaker и настройка connection pool находятся за рамками
задачи.

---

## Формулировка

Дан код отправки платежа. Нужно:

1. Найти проблемы и объяснить их последствия.
2. Отрефакторить код до приемлемого production-варианта.
3. Определить, какие ошибки можно повторять без риска двойного платежа.

Ориентир по времени:

- 5 минут — компиляция и сверка контракта;
- 10 минут — ресурсы, timeout и обработка ошибок;
- 10 минут — рефакторинг;
- 5 минут — идемпотентность, retry и тесты.

---

## Исходный код и контракт

```go
package main

import (
    "fmt"
    "errors"
)

// PaymentRequest входной контракт.
type PaymentRequest struct {
    UserID int
    Amount float64 `json:"amount"`
}

// отправляет запрос на перевод
func SendPayment(req PaymentRequest) (string, error) {
    client := &http.Client{}
    body, _ := json.Marshal(req)

    resp, err := client.Post(
        "http://payments.internal/process",
        "application/json",
        bytes.NewReader(body),
    )
    if err != nil {
        return "", fmt.Errorf("client.Post: %v", err)
    }

    data, err := io.ReadAll(resp.Body)
    if resp.StatusCode != 200 {
        return "", errors.New("payment service error")
    }
    resp.Body.Close()

    return string(data), nil
}
```

Псевдоконтракт не указывает валюту. Сумма без валюты неоднозначна, поэтому до
production нужно уточнить одно из двух:

- сервис работает только с одной заранее зафиксированной валютой;
- контракт принимает `currency` в формате ISO 4217.

Дальше используется второй вариант — контракт расширен полем `currency`:

```http
Content-Type: application/json
Accept: application/json
Idempotency-Key: <уникальный ключ логического платежа>

{"user_id":12345,"amount":199.99,"currency":"RUB"}
```

| Status | Тело ответа | Смысл |
| --- | --- | --- |
| `200` | `{"payment_id":"pay_8f3a91c2","status":"ok"}` | Успех |
| `400` | `invalid_request` | Невалидный запрос |
| `409` | `idempotency_conflict` | Тот же ключ использован с другим payload |
| `422` | `payment_rejected` | Платёж отклонён |
| `500` | `internal_error` | Внутренняя ошибка |
| `503` | `service_unavailable` | Сервис временно недоступен |

---

## Что нужно найти

| Приоритет | Проблема | Последствие |
| --- | --- | --- |
| Блокер | Нет импортов `http`, `json`, `bytes`, `io`; `err` после `ReadAll` не используется | Код не компилируется |
| Критично | У `UserID` нет тега `json:"user_id"` | Сервис получает `UserID`, а не `user_id` |
| Критично | Нет `Idempotency-Key` | Неизвестный результат нельзя безопасно повторить |
| Критично | `Body` закрывается поздно и не на всех ветках | Утечка ресурсов, ухудшение повторного использования соединений |
| Критично | Нет timeout и `context.Context` | Вызов может зависнуть, отмена сверху не дойдёт до HTTP-запроса |
| Критично | Ошибки `Marshal` и `ReadAll` игнорируются | Можно отправить или принять повреждённые данные |
| Критично | В контракте нет валюты | Нельзя однозначно интерпретировать сумму и её точность |
| Важно | Деньги представлены `float64` | Ошибки двоичного округления |
| Важно | Все неуспешные статусы превращаются в одну строковую ошибку | Вызывающий код не различает отказ, конфликт и временный сбой |
| Важно | Ответ полностью читается в `[]byte` и возвращается строкой | Лишнее выделение памяти и отсутствие проверки JSON-контракта |
| Важно | Клиент и URL создаются или задаются внутри функции | Сложнее конфигурировать и тестировать |
| Важно | Ошибка оборачивается через `%v`, а не `%w` | Теряется цепочка для `errors.Is` и `errors.As` |
| Желательно | Нет `Accept: application/json` | Запрос не полностью соответствует контракту |

`http.Client{}` использует общий `http.DefaultTransport`, поэтому утверждать, что
каждый вызов обязательно создаёт новый connection pool, неверно. Проблема здесь
в отсутствии явной конфигурации, timeout и удобной подмены клиента в тестах.

---

## Исправленное решение

Ниже одна HTTP-попытка. Retry лучше размещать уровнем выше, потому что именно
там известен общий deadline и можно гарантировать повтор с теми же ключом и
payload.

<details>
<summary>Показать решение</summary>

```go
package payment

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "math/rand/v2"
    "net"
    "net/http"
    "net/url"
    "strings"
    "time"
)

type PaymentRequest struct {
    UserID         int64       `json:"user_id"`
    Amount         json.Number `json:"amount"`
    CurrencyCode   string      `json:"currency"`
    IdempotencyKey string      `json:"-"`
}

type PaymentResponse struct {
    PaymentID string `json:"payment_id"`
    Status    string `json:"status"`
}

type ServiceError struct {
    StatusCode int    `json:"-"`
    Code       string `json:"error"`
    Message    string `json:"message"`
}

func (e *ServiceError) Error() string {
    return fmt.Sprintf("payment service: status=%d code=%q message=%q",
        e.StatusCode, e.Code, e.Message)
}

func (e *ServiceError) Retryable() bool {
    return e.StatusCode == http.StatusInternalServerError ||
        e.StatusCode == http.StatusServiceUnavailable
}

type Client struct {
    httpClient *http.Client
    endpoint   string
}

func NewClient(httpClient *http.Client, endpoint string) (*Client, error) {
    if httpClient == nil || httpClient.Timeout <= 0 {
        return nil, errors.New("http client with positive timeout is required")
    }

    parsed, err := url.Parse(endpoint)
    if err != nil || parsed.Scheme == "" || parsed.Host == "" {
        return nil, fmt.Errorf("invalid payment endpoint %q", endpoint)
    }

    return &Client{
        httpClient: httpClient,
        endpoint:   strings.TrimRight(endpoint, "/"),
    }, nil
}

func (c *Client) SendPayment(
    ctx context.Context,
    input PaymentRequest,
) (PaymentResponse, error) {
    body, err := json.Marshal(input)
    if err != nil {
        return PaymentResponse{}, fmt.Errorf("marshal payment request: %w", err)
    }

    request, err := http.NewRequestWithContext(
        ctx,
        http.MethodPost,
        c.endpoint+"/process",
        bytes.NewReader(body),
    )
    if err != nil {
        return PaymentResponse{}, fmt.Errorf("create payment request: %w", err)
    }
    request.Header.Set("Content-Type", "application/json")
    request.Header.Set("Accept", "application/json")
    request.Header.Set("Idempotency-Key", input.IdempotencyKey)

    response, err := c.httpClient.Do(request)
    if err != nil {
        return PaymentResponse{}, fmt.Errorf("send payment request: %w", err)
    }
    defer response.Body.Close()

    if response.StatusCode != http.StatusOK {
        serviceErr := &ServiceError{
            StatusCode: response.StatusCode,
        }
        if err := json.NewDecoder(response.Body).Decode(serviceErr); err != nil {
            return PaymentResponse{}, fmt.Errorf(
                "decode payment error with status %d: %w",
                response.StatusCode,
                err,
            )
        }
        return PaymentResponse{}, serviceErr
    }

    var result PaymentResponse
    if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
        return PaymentResponse{}, fmt.Errorf("decode payment response: %w", err)
    }
    if result.PaymentID == "" || result.Status == "" {
        return PaymentResponse{}, errors.New("incomplete payment response")
    }

    return result, nil
}

func (c *Client) SendPaymentWithRetry(
    ctx context.Context,
    input PaymentRequest,
) (PaymentResponse, error) {
    const (
        maxAttempts = 3
        baseDelay   = 100 * time.Millisecond
    )

    var lastErr error
    delay := baseDelay
    for attempt := 0; attempt < maxAttempts; attempt++ {
        result, err := c.SendPayment(ctx, input)
        if err == nil {
            return result, nil
        }
        lastErr = err
        if attempt == maxAttempts-1 || !shouldRetry(err) {
            return PaymentResponse{}, err
        }

        jitter := time.Duration(rand.IntN(100)) * time.Millisecond
        timer := time.NewTimer(delay + jitter)

        select {
        case <-ctx.Done():
            timer.Stop()
            return PaymentResponse{}, ctx.Err()
        case <-timer.C:
        }
        delay *= 2
    }

    return PaymentResponse{}, lastErr
}

func shouldRetry(err error) bool {
    if errors.Is(err, context.Canceled) ||
        errors.Is(err, context.DeadlineExceeded) {
        return false
    }

    var serviceErr *ServiceError
    if errors.As(err, &serviceErr) {
        return serviceErr.Retryable()
    }

    var networkErr net.Error
    return errors.As(err, &networkErr)
}

```

</details>

`json.Number` сохраняет десятичное представление суммы без преобразования в
`float64`. В доменной модели вместо него обычно используют decimal-тип либо пару
`amount_minor + currency`: scale зависит от валюты — например, у JPY нет
дробной части, а у KWD три знака. Допустимые `UserID`, сумму, валюту и наличие
ключа идемпотентности проверяет бизнес-слой до вызова HTTP-клиента. Клиент
отвечает только за wire-контракт и транспорт.

Если сервис одновалютный, это нужно явно записать в контракте, а поле суммы
назвать конкретно, например `AmountKopecks` для RUB, а не обобщённо
`AmountCents`.

---

## Повторы и идемпотентность

| Результат попытки | Повторять? | Почему |
| --- | --- | --- |
| `400`, `409`, `422` | Нет | Повтор того же запроса не исправит причину |
| `500`, `503` | Ограниченно | Ошибка может быть временной |
| Сетевой сбой после отправки | Только с тем же ключом и payload | Неизвестно, успел ли сервис провести платёж |
| `context.Canceled`, общий deadline исчерпан | Нет | Операция отменена вызывающей стороной |

Ключ создаётся один раз на логическую попытку оплаты и сохраняется вместе с ней.
Все сетевые повторы используют тот же ключ и байт-в-байт тот же payload, включая
сумму и валюту. Строка вроде `user123-amount199` годится как пример заголовка,
но не как алгоритм: пользователь может законно сделать два одинаковых платежа.

Retry должен быть ограничен числом попыток и общим deadline; между попытками
нужны exponential backoff и jitter. В `SendPaymentWithRetry` первая попытка
выполняется сразу, а перед второй и третьей добавляется задержка. Пока клиент
ждёт, отмена `context` немедленно завершает операцию.

---

## Пример теста

`httptest.Server` позволяет проверить реальный HTTP-запрос без внешнего
платёжного сервиса. Handler сохраняет наблюдаемый wire-контракт в канал, а все
assertions выполняются в goroutine самого теста.

```go
func TestClientSendPayment_SendsWireContract(test *testing.T) {
    type observedRequest struct {
        method      string
        path        string
        contentType string
        accept      string
        key         string
        body        []byte
    }

    observed := make(chan observedRequest, 1)
    server := httptest.NewServer(http.HandlerFunc(func(
        response http.ResponseWriter,
        request *http.Request,
    ) {
        body, err := io.ReadAll(request.Body)
        if err != nil {
            response.WriteHeader(http.StatusInternalServerError)
            return
        }
        observed <- observedRequest{
            method:      request.Method,
            path:        request.URL.Path,
            contentType: request.Header.Get("Content-Type"),
            accept:      request.Header.Get("Accept"),
            key:         request.Header.Get("Idempotency-Key"),
            body:        body,
        }

        response.Header().Set("Content-Type", "application/json")
        if _, err := io.WriteString(
            response,
            `{"payment_id":"pay-42","status":"ok"}`,
        ); err != nil {
            return
        }
    }))
    defer server.Close()

    client, err := NewClient(
        &http.Client{Timeout: time.Second},
        server.URL,
    )
    if err != nil {
        test.Fatalf("NewClient: %v", err)
    }

    input := PaymentRequest{
        UserID:         12345,
        Amount:         json.Number("199.99"),
        CurrencyCode:   "RUB",
        IdempotencyKey: "payment-attempt-7",
    }
    result, err := client.SendPayment(context.Background(), input)
    if err != nil {
        test.Fatalf("SendPayment: %v", err)
    }
    if result.PaymentID != "pay-42" || result.Status != "ok" {
        test.Fatalf("response = %#v", result)
    }

    got := <-observed
    if got.method != http.MethodPost || got.path != "/process" {
        test.Fatalf("request = %s %s", got.method, got.path)
    }
    if got.contentType != "application/json" ||
        got.accept != "application/json" {
        test.Fatalf(
            "content type = %q, accept = %q",
            got.contentType,
            got.accept,
        )
    }
    if got.key != input.IdempotencyKey {
        test.Fatalf("idempotency key = %q", got.key)
    }

    var payload struct {
        UserID       int64       `json:"user_id"`
        Amount       json.Number `json:"amount"`
        CurrencyCode string      `json:"currency"`
    }
    if err := json.Unmarshal(got.body, &payload); err != nil {
        test.Fatalf("decode request: %v", err)
    }
    if payload.UserID != input.UserID ||
        payload.Amount != input.Amount ||
        payload.CurrencyCode != input.CurrencyCode {
        test.Fatalf("payload = %#v", payload)
    }
}
```

`Fatal` нельзя вызывать из HTTP-handler: он работает в отдельной goroutine, а
`FailNow` должен быть вызван goroutine самого теста. Поэтому handler только
собирает факты, а проверка выполняется после `SendPayment`.

---

## Что проверить тестами

- Отправляются правильные метод, URL, точные сумма и валюта, а также три
  обязательных заголовка.
- `200` декодируется в `PaymentResponse`.
- `400`, `409`, `422`, `500` и `503` возвращаются как `*ServiceError`.
- Ошибка чтения и невалидный JSON не игнорируются.
- Отмена context и timeout завершают вызов.
- Retry-обёртка сохраняет один `Idempotency-Key` и один payload.

Для повторов удобно считать запросы и сохранять тела в handler. Чтобы такой тест
не ждал реальные `100ms + 200ms`, backoff лучше передавать в клиент как
зависимость и подменять функцией без ожидания.

---

## Interview-ready answer

### 1. Какие проблемы нужно назвать первыми?

- Компиляция — отсутствуют импорты и есть неиспользуемый `err`.
- Контракт — неверное имя `user_id`, нет валюты, `Accept` и `Idempotency-Key`.
- Ресурсы — нет timeout/context, body закрывается не на всех ветках.
- Ошибки — игнорируются `Marshal` и `ReadAll`, статусы теряют смысл.
- Деньги — `float64` лучше заменить на decimal либо на minor units вместе с
  валютой и её scale.

### 2. Зачем повторять запрос с тем же ключом?

- Неопределённость — при сетевом сбое клиент не знает, принят ли первый запрос.
- Дедупликация — тот же ключ позволяет сервису вернуть прежний результат, а не
  провести второй платёж.
- Ограничение — payload при повторе должен остаться тем же, иначе ожидаем `409`.

### 3. Какие ответы можно повторять?

- Не повторяем — `400`, `409` и `422`, потому что причина не временная.
- Повторяем ограниченно — `500`, `503` и некоторые сетевые ошибки.
- Условие — используем тот же ключ, тот же payload, общий deadline и backoff с
  jitter.

### 4. Почему недостаточно вернуть `string` и `payment service error`?

- Успех — вызывающему коду нужны типизированные `payment_id` и `status`.
- Ошибка — status, code и message определяют retry и бизнес-реакцию.
- Диагностика — `%w` сохраняет исходную ошибку для `errors.Is` и `errors.As`.

---

## Связанные материалы

- [Списание баланса и ledger](./03-balance-withdrawal-and-ledger.md)
- [HTTP client в Go](../../../08-networking-and-api/protocols/02-http/03-client-in-go.md)
- [Idempotency](../../../05-system-design/reliability-patterns/06-idempotency.md)
- [Retry с backoff](../system-primitives/02-retry-with-backoff.md)
- [Тестирование HTTP](../../../09-testing-and-quality/05-http-server-testing.md)
- [Payment System](../../../05-system-design/interview-cases/11-payment-system.md)
