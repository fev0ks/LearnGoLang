# Аудит-логирование аутентификации

Аудит-лог фиксирует кто, когда и что сделал. Для аутентификации — это обязательный инструмент безопасности: обнаружение атак, расследование инцидентов, compliance.

## Содержание

- [Какие события логировать](#какие-события-логировать)
- [Stable fields — структура события](#stable-fields--структура-события)
- [Что никогда не логировать](#что-никогда-не-логировать)
- [Реализация в Go](#реализация-в-го)
- [Email hashing](#email-hashing)
- [Использование аудит-лога](#использование-аудит-лога)

---

## Какие события логировать

| Событие | Когда |
|---|---|
| `auth.signup` | успешная регистрация |
| `auth.login.success` | успешный вход (пароль или OAuth) |
| `auth.login.failure` | неверный пароль, неизвестный email |
| `auth.logout` | явный logout |
| `auth.session.revoked` | принудительная ревокация (смена пароля, admin action) |
| `auth.oauth.callback.success` | успешный OAuth login |
| `auth.oauth.callback.failure` | ошибка OAuth (invalid state, claim mismatch) |
| `auth.password.changed` | смена пароля |

---

## Stable fields — структура события

Поля должны быть стабильными — future abuse-control layer будет потреблять их без изменений.

```go
type AuthEvent struct {
    // Обязательные поля
    EventType  string    `json:"event_type"`   // "auth.login.failure"
    OccurredAt time.Time `json:"occurred_at"`
    RequestID  string    `json:"request_id"`   // для корреляции с HTTP логами

    // Идентификация — что знаем на момент события
    AccountID     string `json:"account_id,omitempty"`   // если известен
    EmailHash     string `json:"email_hash,omitempty"`   // SHA-256(normalize(email))
    Provider      string `json:"provider,omitempty"`     // "password", "google", "apple"
    ProviderSub   string `json:"provider_sub,omitempty"` // subject у провайдера

    // Контекст запроса
    IPAddress string `json:"ip_address,omitempty"`
    UserAgent string `json:"user_agent,omitempty"`

    // Детали события
    ReasonCode string `json:"reason_code,omitempty"` // "bad_password", "unknown_email", "state_mismatch"
    SessionID  string `json:"session_id,omitempty"`  // ID сессии (не токен!)
}
```

**Почему `email_hash`, а не `email`:**  
Аудит-лог часто попадает в системы где email не должен быть в открытом виде (GDPR, внутренние политики). Хеш позволяет коррелировать события по одному email без его раскрытия.

**Почему `session_id` (не токен):**  
Токен — секрет. ID сессии — просто идентификатор строки в БД, не позволяет авторизоваться.

---

## Что никогда не логировать

```go
// НЕЛЬЗЯ
log.Info("login attempt",
    "email", email,          // открытый email
    "password", password,    // пароль в любом виде
    "token", sessionToken,   // session token
    "access_token", token,   // OAuth access token
    "id_token", idToken,     // JWT от провайдера
    "code", authCode,        // authorization code
    "code_verifier", verifier, // PKCE verifier
)

// МОЖНО
log.Info("login attempt",
    "email_hash", hashEmail(email),  // хеш
    "account_id", accountID,         // если известен
    "reason_code", "bad_password",
    "request_id", requestID,
)
```

---

## Реализация в Go

```go
type AuditLogger struct {
    log *slog.Logger
}

func NewAuditLogger(log *slog.Logger) *AuditLogger {
    return &AuditLogger{log: log.With("component", "audit")}
}

func (a *AuditLogger) LoginSuccess(ctx context.Context, accountID, email, provider, sessionID string, r *http.Request) {
    a.log.InfoContext(ctx, "auth.login.success",
        "event_type",  "auth.login.success",
        "occurred_at", time.Now().UTC(),
        "request_id",  requestIDFromContext(ctx),
        "account_id",  accountID,
        "email_hash",  hashEmail(email),
        "provider",    provider,
        "session_id",  sessionID,
        "ip_address",  clientIP(r),
        "user_agent",  r.UserAgent(),
    )
}

func (a *AuditLogger) LoginFailure(ctx context.Context, email, provider, reasonCode string, r *http.Request) {
    // AccountID может быть неизвестен (неверный email) — не включать
    a.log.WarnContext(ctx, "auth.login.failure",
        "event_type",  "auth.login.failure",
        "occurred_at", time.Now().UTC(),
        "request_id",  requestIDFromContext(ctx),
        "email_hash",  hashEmail(email),  // хеш, не открытый email
        "provider",    provider,
        "reason_code", reasonCode,        // "bad_password", "unknown_email", "account_locked"
        "ip_address",  clientIP(r),
        "user_agent",  r.UserAgent(),
    )
}

func (a *AuditLogger) SessionRevoked(ctx context.Context, accountID, sessionID, reason string) {
    a.log.InfoContext(ctx, "auth.session.revoked",
        "event_type",  "auth.session.revoked",
        "occurred_at", time.Now().UTC(),
        "request_id",  requestIDFromContext(ctx),
        "account_id",  accountID,
        "session_id",  sessionID,
        "reason",      reason,  // "logout", "password_changed", "admin_revoke"
    )
}

func (a *AuditLogger) OAuthFailure(ctx context.Context, provider, reasonCode string, r *http.Request) {
    a.log.WarnContext(ctx, "auth.oauth.callback.failure",
        "event_type",  "auth.oauth.callback.failure",
        "occurred_at", time.Now().UTC(),
        "request_id",  requestIDFromContext(ctx),
        "provider",    provider,
        "reason_code", reasonCode,  // "invalid_state", "nonce_mismatch", "claim_missing"
        "ip_address",  clientIP(r),
    )
}
```

### Интеграция в сервисный слой

```go
func (s *AuthService) Login(ctx context.Context, email, password string, r *http.Request) (*Session, error) {
    account, err := s.repo.GetByEmail(ctx, normalizeEmail(email))
    if err != nil {
        // Timing: одинаковое время ответа для "нет пользователя" и "неверный пароль"
        bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
        s.audit.LoginFailure(ctx, email, "password", "unknown_email", r)
        return nil, ErrInvalidCredentials
    }

    if err := checkPassword(account.PasswordHash, password); err != nil {
        s.audit.LoginFailure(ctx, email, "password", "bad_password", r)
        return nil, ErrInvalidCredentials
    }

    session, err := s.sessions.Create(ctx, account.ID)
    if err != nil {
        return nil, err
    }

    s.audit.LoginSuccess(ctx, account.ID, email, "password", session.ID, r)
    return session, nil
}
```

---

## Email hashing

Хеш email должен быть стабильным и воспроизводимым — для корреляции событий по одному email.

```go
import (
    "crypto/sha256"
    "encoding/hex"
    "strings"
)

// Нормализовать перед хешированием — иначе "Alice@Example.com" и "alice@example.com"
// дадут разные хеши
func hashEmail(email string) string {
    normalized := strings.ToLower(strings.TrimSpace(email))
    h := sha256.Sum256([]byte(normalized))
    return hex.EncodeToString(h[:])
}
```

**Добавить pepper** (соль на уровне приложения) если нужна дополнительная защита от rainbow tables по email:

```go
func hashEmail(email string, pepper []byte) string {
    normalized := strings.ToLower(strings.TrimSpace(email))
    mac := hmac.New(sha256.New, pepper)
    mac.Write([]byte(normalized))
    return hex.EncodeToString(mac.Sum(nil))
}
// pepper хранится в секретах приложения, не в БД
```

---

## Использование аудит-лога

**Обнаружение brute force** — много `auth.login.failure` с одного IP или по одному `email_hash`:

```sql
SELECT ip_address, COUNT(*) as failures
FROM audit_log
WHERE event_type = 'auth.login.failure'
  AND occurred_at > NOW() - INTERVAL '10 minutes'
GROUP BY ip_address
HAVING COUNT(*) > 10
ORDER BY failures DESC;
```

**Подозрительная активность** — логин из новой геолокации сразу после логина из другой:

```sql
SELECT account_id, ip_address, occurred_at
FROM audit_log
WHERE event_type = 'auth.login.success'
  AND account_id = $1
ORDER BY occurred_at DESC
LIMIT 10;
```

**Расследование инцидента** — все события по аккаунту за период:

```sql
SELECT event_type, occurred_at, ip_address, reason_code, session_id
FROM audit_log
WHERE account_id = $1
  AND occurred_at BETWEEN $2 AND $3
ORDER BY occurred_at;
```
