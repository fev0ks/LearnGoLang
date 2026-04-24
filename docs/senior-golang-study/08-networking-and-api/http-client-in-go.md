# HTTP Client in Go

`http.DefaultClient` — ловушка для production кода. Правильный HTTP client в Go требует явной настройки Transport, таймаутов и retry стратегии.

---

## `http.DefaultClient` — почему опасен

```go
// Плохо: DefaultClient без таймаутов
resp, err := http.Get("https://api.example.com/users")

// DefaultClient = &http.Client{} — нет таймаутов!
// Если сервер зависнет — твоя горутина висит вечно → goroutine leak
```

---

## Правильный `http.Client`

```go
client := &http.Client{
    Timeout: 30 * time.Second, // полный timeout от начала до конца

    Transport: &http.Transport{
        // Connection pool
        MaxIdleConns:        100,              // max idle connections всего
        MaxIdleConnsPerHost: 10,               // max idle на один хост
        MaxConnsPerHost:     0,               // 0 = без ограничения (active)
        IdleConnTimeout:     90 * time.Second, // когда убивать idle соединение

        // Timeouts на уровне TCP
        DialContext: (&net.Dialer{
            Timeout:   5 * time.Second,  // timeout на TCP connect
            KeepAlive: 30 * time.Second, // TCP keepalive
        }).DialContext,
        TLSHandshakeTimeout:   10 * time.Second,
        ResponseHeaderTimeout: 10 * time.Second, // ждать первый байт ответа
        ExpectContinueTimeout: 1 * time.Second,
        
        // Compression
        DisableCompression: false, // автоматическое gzip
        
        // HTTP/2
        ForceAttemptHTTP2: true,
    },
}
```

### Timeout на разных уровнях

```
http.Client.Timeout:          весь lifecycle (dial + TLS + write + read)
DialContext.Timeout:          только TCP connect
TLSHandshakeTimeout:          только TLS handshake
ResponseHeaderTimeout:        ожидание первого байта response headers
```

```go
// Дополнительный контроль через context
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
resp, err := client.Do(req)
// context timeout переопределяет client.Timeout если короче
```

---

## Connection pooling — как работает Transport

`http.Transport` поддерживает **connection pool** (idle connections). Это ключевая оптимизация: не создавать TCP+TLS соединение на каждый запрос.

```
Request 1: dial TCP → TLS handshake → HTTP request → HTTP response → [keep-alive → pool]
Request 2:                                                             [get from pool] → HTTP request → ...
```

**Правила для production:**
1. Создавай `http.Client` **один раз** — при старте приложения (и переиспользуй)
2. `http.Transport` не копируй — thread-safe, разделяй между запросами
3. Всегда закрывай `resp.Body` — иначе соединение не вернётся в pool

```go
// КРИТИЧНО: всегда читать и закрывать body
resp, err := client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()

// Читать полностью перед закрытием — иначе соединение не переиспользуется
body, err := io.ReadAll(resp.Body)
```

---

## Retry стратегия

### Базовый retry с exponential backoff

```go
type RetryClient struct {
    client     *http.Client
    maxRetries int
    baseDelay  time.Duration
}

func (rc *RetryClient) Do(req *http.Request) (*http.Response, error) {
    var lastErr error
    
    for attempt := range rc.maxRetries + 1 {
        // Копируем request для повторной отправки (body читается только раз)
        reqCopy, err := cloneRequest(req)
        if err != nil {
            return nil, err
        }
        
        resp, err := rc.client.Do(reqCopy)
        if err == nil && !isRetryable(resp.StatusCode) {
            return resp, nil
        }
        
        if err == nil {
            // Читаем и закрываем body неуспешного ответа
            io.Copy(io.Discard, resp.Body)
            resp.Body.Close()
            lastErr = fmt.Errorf("status %d", resp.StatusCode)
        } else {
            lastErr = err
            // Не retry при context cancel
            if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
                return nil, err
            }
        }
        
        if attempt < rc.maxRetries {
            delay := rc.baseDelay * time.Duration(1<<attempt) // exponential
            jitter := time.Duration(rand.Int63n(int64(delay / 4))) // jitter
            
            select {
            case <-time.After(delay + jitter):
            case <-req.Context().Done():
                return nil, req.Context().Err()
            }
        }
    }
    return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func isRetryable(code int) bool {
    switch code {
    case http.StatusTooManyRequests,
        http.StatusBadGateway,
        http.StatusServiceUnavailable,
        http.StatusGatewayTimeout:
        return true
    }
    return false
}

// Клонирование request с новым body
func cloneRequest(req *http.Request) (*http.Request, error) {
    clone := req.Clone(req.Context())
    if req.Body != nil && req.Body != http.NoBody {
        // Для retry нужно сохранить body
        // Лучше использовать GetBody
        if req.GetBody != nil {
            body, err := req.GetBody()
            if err != nil {
                return nil, err
            }
            clone.Body = body
        }
    }
    return clone, nil
}
```

### Retry с `GetBody` для body requests

```go
// При создании request с body — устанавливай GetBody для возможности retry
bodyBytes := []byte(`{"name":"alice"}`)
req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
req.GetBody = func() (io.ReadCloser, error) {
    return io.NopCloser(bytes.NewReader(bodyBytes)), nil
}
```

---

## Читать response правильно

```go
func readResponse[T any](resp *http.Response) (T, error) {
    var result T
    defer resp.Body.Close()
    
    // Ограничить размер ответа
    body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10 MB
    if err != nil {
        return result, fmt.Errorf("read body: %w", err)
    }
    
    if resp.StatusCode >= 400 {
        return result, fmt.Errorf("status %d: %s", resp.StatusCode, body)
    }
    
    if err := json.Unmarshal(body, &result); err != nil {
        return result, fmt.Errorf("unmarshal: %w", err)
    }
    return result, nil
}
```

---

## HTTP/2

Go `net/http` поддерживает HTTP/2 автоматически при HTTPS. Ключевые улучшения:
- **Multiplexing**: несколько запросов на одном TCP соединении параллельно
- **Header compression** (HPACK): снижает overhead повторяющихся headers
- **Server push**: сервер может отправить ресурсы до запроса (редко используется)

```go
// Явное HTTP/2 без TLS (для internal services, h2c)
import "golang.org/x/net/http2"

client := &http.Client{
    Transport: &http2.Transport{
        AllowHTTP: true, // h2c — HTTP/2 без TLS
        DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
            return (&net.Dialer{}).DialContext(ctx, network, addr)
        },
    },
}
```

---

## Типичные паттерны

### API Client как сервис

```go
type UserAPIClient struct {
    base   string
    client *http.Client
}

func NewUserAPIClient(baseURL string) *UserAPIClient {
    return &UserAPIClient{
        base: strings.TrimRight(baseURL, "/"),
        client: &http.Client{
            Timeout: 10 * time.Second,
            Transport: &http.Transport{
                MaxIdleConnsPerHost: 10,
                IdleConnTimeout:     90 * time.Second,
            },
        },
    }
}

func (c *UserAPIClient) GetUser(ctx context.Context, id string) (*User, error) {
    req, err := http.NewRequestWithContext(ctx, "GET",
        fmt.Sprintf("%s/users/%s", c.base, id), nil)
    if err != nil {
        return nil, fmt.Errorf("build request: %w", err)
    }
    req.Header.Set("Accept", "application/json")
    
    resp, err := c.client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("do request: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode == http.StatusNotFound {
        return nil, ErrUserNotFound
    }
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
        return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
    }
    
    var user User
    if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
        return nil, fmt.Errorf("decode response: %w", err)
    }
    return &user, nil
}
```

### Circuit Breaker (базовый паттерн)

```go
// Используй готовые библиотеки: sony/gobreaker, mercari/go-circuitbreaker
import "github.com/sony/gobreaker"

cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
    Name:        "user-service",
    MaxRequests: 3,   // в half-open: пропустить 3 запроса
    Interval:    10 * time.Second,
    Timeout:     30 * time.Second, // open → half-open
    ReadyToTrip: func(counts gobreaker.Counts) bool {
        return counts.ConsecutiveFailures >= 5
    },
})

result, err := cb.Execute(func() (any, error) {
    return client.GetUser(ctx, id)
})
```

---

## Диагностика

### Трассировка запроса

```go
import "net/http/httptrace"

trace := &httptrace.ClientTrace{
    DNSDone:           func(info httptrace.DNSDoneInfo) { log.Printf("DNS: %v", info) },
    ConnectDone:       func(net, addr string, err error) { log.Printf("Connect: %s %v", addr, err) },
    TLSHandshakeDone: func(state tls.ConnectionState, err error) { log.Printf("TLS done: %v", err) },
    GotFirstResponseByte: func() { log.Printf("first byte received") },
}
ctx = httptrace.WithClientTrace(ctx, trace)
req = req.WithContext(ctx)
```

---

## Interview-ready answer

**Q: Почему нельзя использовать `http.DefaultClient` в production?**

DefaultClient создаётся без таймаутов. Если внешний сервис зависнет или будет очень медленным — горутина заблокируется навсегда. Под нагрузкой это быстро исчерпает пул горутин. Также DefaultClient не настроен под конкретный use case: MaxIdleConnsPerHost по умолчанию = 2, что создаёт очередь соединений при параллельных запросах.

**Q: Что такое connection pooling в HTTP клиенте и почему важно закрывать resp.Body?**

`http.Transport` держит пул idle keep-alive соединений. После ответа соединение возвращается в пул — и следующий запрос использует уже установленное TCP+TLS соединение без overhead. Если не закрыть resp.Body (и не прочитать до конца) — соединение не может вернуться в пул и будет создано новое. Под нагрузкой это обнулит пользу от connection pooling.

**Q: Как реализовать retry безопасно?**

Retry только на идемпотентные коды (429, 502, 503, 504). Никогда не retry при context.Canceled. Обязательно exponential backoff + jitter чтобы не thundering herd. Для запросов с body — `GetBody` функция для пересоздания body. Retry POST только если явно идемпотентно (через Idempotency-Key).
