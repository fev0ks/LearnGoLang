# TLS termination, Re-encryption и mTLS

## Содержание

- [Три модели](#три-модели)
- [TLS в Go: http.Server](#tls-в-go-httpserver)
- [mTLS: взаимная аутентификация](#mtls-взаимная-аутентификация)
- [mTLS клиент в Go](#mtls-клиент-в-го)
- [Откуда брать внутренние сертификаты](#откуда-брать-внутренние-сертификаты)
- [Когда какую модель выбрать](#когда-какую-модель-выбрать)

---

## Три модели

**TLS termination at edge:**
```
client ──TLS──▶ edge/proxy ──HTTP──▶ backend
```
Внешний HTTPS заканчивается на прокси, внутри — незашифрованный HTTP. Упрощает certificate management — сертификат только на proxy.

**Re-encryption:**
```
client ──TLS──▶ edge/proxy ──TLS──▶ backend
```
Внешний TLS завершается, но к backend открывается новый TLS соединение. Backend должен иметь серверный сертификат. Нужен при compliance-требованиях или zero-trust сети.

**mTLS (mutual TLS):**
```
service A ──TLS + client cert──▶ service B
```
Обе стороны предъявляют сертификаты. Сервер не только шифрует трафик, но и аутентифицирует клиента. Это workload identity — сервис A доказывает что он действительно A.

---

## TLS в Go: http.Server

```go
import "crypto/tls"

func newTLSServer(certFile, keyFile string) *http.Server {
    tlsCfg := &tls.Config{
        MinVersion: tls.VersionTLS12,
        // TLS 1.3 предпочтительнее — только современные шифры, нет negotiation
        // MinVersion: tls.VersionTLS13,

        // Безопасные cipher suites (Go выбирает автоматически для TLS 1.3)
        // Для TLS 1.2 явно ограничить при необходимости:
        CipherSuites: []uint16{
            tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
            tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
            tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
        },

        // ALPN — объявить поддержку HTTP/2
        NextProtos: []string{"h2", "http/1.1"},
    }

    return &http.Server{
        Addr:      ":8443",
        TLSConfig: tlsCfg,
    }
}

// Запуск
srv := newTLSServer("cert.pem", "key.pem")
log.Fatal(srv.ListenAndServeTLS("cert.pem", "key.pem"))
```

### Автоматический Let's Encrypt (golang.org/x/crypto/acme/autocert)

```go
import "golang.org/x/crypto/acme/autocert"

m := &autocert.Manager{
    Cache:      autocert.DirCache("/var/cache/autocert"),
    Prompt:     autocert.AcceptTOS,
    HostPolicy: autocert.HostWhitelist("example.com", "www.example.com"),
}

srv := &http.Server{
    Addr:      ":443",
    TLSConfig: m.TLSConfig(),
}
// Отдельный сервер для HTTP→HTTPS редиректа
go http.ListenAndServe(":80", m.HTTPHandler(nil))
log.Fatal(srv.ListenAndServeTLS("", ""))
```

---

## mTLS: взаимная аутентификация

```go
// Сервер: требовать клиентский сертификат
func newMTLSServer(certFile, keyFile, caCertFile string) (*http.Server, error) {
    // Загрузить CA cert для верификации клиентов
    caCert, err := os.ReadFile(caCertFile)
    if err != nil {
        return nil, fmt.Errorf("read CA cert: %w", err)
    }
    caCertPool := x509.NewCertPool()
    if !caCertPool.AppendCertsFromPEM(caCert) {
        return nil, errors.New("failed to parse CA cert")
    }

    tlsCfg := &tls.Config{
        MinVersion: tls.VersionTLS12,
        ClientAuth: tls.RequireAndVerifyClientCert,  // обязательный клиентский cert
        ClientCAs:  caCertPool,                       // кому доверяем
    }

    srv := &http.Server{
        Addr:      ":8443",
        TLSConfig: tlsCfg,
    }
    return srv, nil
}

// Получить identity клиента из запроса
func clientIdentityFromTLS(r *http.Request) string {
    if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
        return ""
    }
    return r.TLS.PeerCertificates[0].Subject.CommonName
}

// Middleware: проверить что клиент — разрешённый сервис
func requireServiceCert(allowedCNs []string) func(http.Handler) http.Handler {
    allowed := make(map[string]struct{})
    for _, cn := range allowedCNs {
        allowed[cn] = struct{}{}
    }

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            cn := clientIdentityFromTLS(r)
            if _, ok := allowed[cn]; !ok {
                http.Error(w, "forbidden", http.StatusForbidden)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

---

## mTLS клиент в Go

```go
func newMTLSClient(certFile, keyFile, caCertFile string) (*http.Client, error) {
    // Клиентский сертификат — наша identity
    clientCert, err := tls.LoadX509KeyPair(certFile, keyFile)
    if err != nil {
        return nil, fmt.Errorf("load client cert: %w", err)
    }

    // CA cert для верификации сервера
    caCert, err := os.ReadFile(caCertFile)
    if err != nil {
        return nil, fmt.Errorf("read CA: %w", err)
    }
    caCertPool := x509.NewCertPool()
    caCertPool.AppendCertsFromPEM(caCert)

    tlsCfg := &tls.Config{
        Certificates: []tls.Certificate{clientCert},  // наш cert
        RootCAs:      caCertPool,                      // доверенные CA
        MinVersion:   tls.VersionTLS12,
    }

    return &http.Client{
        Transport: &http.Transport{TLSClientConfig: tlsCfg},
        Timeout:   10 * time.Second,
    }, nil
}
```

### Тестирование mTLS с самоподписными сертификатами

```bash
# Сгенерировать CA
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 365 -key ca.key -out ca.crt -subj "/CN=test-ca"

# Сгенерировать серверный cert
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr -subj "/CN=localhost"
openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt

# Сгенерировать клиентский cert
openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr -subj "/CN=service-a"
openssl x509 -req -days 365 -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client.crt
```

---

## Откуда брать внутренние сертификаты

В продакшне сертификаты не генерируются вручную — это platform concern:

| Способ | Где используется |
|---|---|
| `cert-manager` (Kubernetes) | автовыпуск через Let's Encrypt или internal CA |
| Cloud PKI (GCP Certificate Authority Service, AWS ACM PCA) | управляемый internal CA |
| HashiCorp Vault PKI | динамические short-lived certs |
| SPIRE / SPIFFE | workload identity в zero-trust средах |
| Service mesh (Istio/Linkerd) | автоматический mTLS между всеми сервисами |

Service mesh — самый распространённый подход в Kubernetes: mTLS включается на уровне sidecar, Go-код ничего не меняет.

---

## Когда какую модель выбрать

```
TLS termination at edge
  → небольшой сервис, operational simplicity важнее
  → edge/CDN уже делает TLS termination

Re-encryption
  → compliance требует шифрование на всех hop-ах
  → внутренняя сеть не считается доверенной

mTLS
  → zero-trust, нужна workload identity
  → много сервисов, критичная инфраструктура
  → service mesh берёт это на себя прозрачно
```
