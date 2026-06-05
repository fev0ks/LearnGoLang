# Сессии и безопасность сессий

Opaque session — случайный непредсказуемый токен, который сервер хранит в БД. В отличие от JWT — сессия ревокабельна мгновенно, сервер полностью контролирует её состояние.

## Содержание

- [Cookie атрибуты](#cookie-атрибуты)
- [Генерация токена и хранение хеша](#генерация-токена-и-хранение-хеша)
- [Session fixation и ротация](#session-fixation-и-ротация)
- [Жизненный цикл: absolute vs idle timeout](#жизненный-цикл-absolute-vs-idle-timeout)
- [Ревокация и logout](#ревокация-и-logout)
- [Схема таблицы сессий](#схема-таблицы-сессий)
- [Обработка ошибок без раскрытия деталей](#обработка-ошибок-без-раскрытия-деталей)

---

## Cookie атрибуты

Каждый атрибут закрывает конкретный вектор атаки:

```go
func setSessionCookie(w http.ResponseWriter, token string) {
    http.SetCookie(w, &http.Cookie{
        Name:     "session_id",
        Value:    token,
        Path:     "/",
        HttpOnly: true,             // недоступен через document.cookie → XSS не украдёт токен
        Secure:   true,             // только по HTTPS → защита от перехвата в сети
        SameSite: http.SameSiteLaxMode, // не отправляется в cross-site POST → защита от CSRF
        MaxAge:   30 * 24 * 3600,  // 30 дней absolute TTL в браузере
    })
}

func clearSessionCookie(w http.ResponseWriter) {
    http.SetCookie(w, &http.Cookie{
        Name:     "session_id",
        Value:    "",
        Path:     "/",
        HttpOnly: true,
        Secure:   true,
        SameSite: http.SameSiteLaxMode,
        MaxAge:   -1,  // удалить немедленно
    })
}
```

| Атрибут | Без него | С ним |
|---|---|---|
| `HttpOnly` | JS читает cookie → XSS крадёт сессию | JS не видит cookie |
| `Secure` | cookie едет по HTTP → MITM перехватывает | только HTTPS |
| `SameSite=Lax` | cookie едет в cross-site POST → CSRF | не едет в фоновых запросах с других сайтов |
| `Path=/` | cookie едет только к конкретному пути | едет ко всем путям сервиса |

---

## Генерация токена и хранение хеша

**Токен** выдаётся клиенту и хранится только в cookie. В БД хранится только его SHA-256 хеш — если БД утечёт, атакующий не сможет подставить токен.

```go
import (
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
)

func generateSessionToken() (token, tokenHash string, err error) {
    b := make([]byte, 32)  // 256 бит энтропии
    if _, err = rand.Read(b); err != nil {
        return "", "", fmt.Errorf("generate token: %w", err)
    }
    token = hex.EncodeToString(b)  // 64 символа, отправляется клиенту

    h := sha256.Sum256([]byte(token))
    tokenHash = hex.EncodeToString(h[:])  // хранится в БД
    return token, tokenHash, nil
}

// При логине
func (s *SessionService) Create(ctx context.Context, accountID string) (string, error) {
    token, tokenHash, err := generateSessionToken()
    if err != nil {
        return "", err
    }

    session := &Session{
        TokenHash: tokenHash,
        AccountID: accountID,
        CreatedAt: time.Now(),
        ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
        LastSeenAt: time.Now(),
    }
    if err := s.repo.Create(ctx, session); err != nil {
        return "", err
    }
    return token, nil  // вернуть только клиенту
}

// При каждом запросе
func (s *SessionService) Validate(ctx context.Context, token string) (*Session, error) {
    h := sha256.Sum256([]byte(token))
    tokenHash := hex.EncodeToString(h[:])

    session, err := s.repo.GetByTokenHash(ctx, tokenHash)
    if err != nil {
        return nil, ErrSessionNotFound  // не раскрывать детали
    }

    if time.Now().After(session.ExpiresAt) {
        return nil, ErrSessionExpired
    }

    // Обновить last_seen (idle timeout) — можно делать асинхронно
    _ = s.repo.UpdateLastSeen(ctx, session.ID, time.Now())

    return session, nil
}
```

---

## Session fixation и ротация

**Session fixation атака:**
1. Атакующий получает session ID (например, до логина, через URL-параметр)
2. Жертва логинится с тем же session ID
3. Атакующий теперь использует тот же ID как аутентифицированный

**Защита: ротация** — при каждом успешном логине выпускать новый session token и инвалидировать старый.

```go
func (s *AuthService) Login(ctx context.Context, email, password string) (string, error) {
    account, err := s.verifyCredentials(ctx, email, password)
    if err != nil {
        return "", ErrInvalidCredentials
    }

    // Инвалидировать ВСЕ существующие сессии этого аккаунта (опционально)
    // или только текущую (если есть cookie в запросе)
    _ = s.sessions.RevokeForAccount(ctx, account.ID)

    // Выпустить новый токен
    token, err := s.sessions.Create(ctx, account.ID)
    if err != nil {
        return "", fmt.Errorf("create session: %w", err)
    }
    return token, nil
}
```

**Ротация при каждом запросе (sliding session):** иногда применяют — каждый запрос выдаёт новый токен и аннулирует старый. Усложняет реализацию и создаёт проблемы при параллельных запросах (race condition). Обычно достаточно ротировать только при логине.

---

## Жизненный цикл: absolute vs idle timeout

```
absolute timeout: 30 дней с момента создания — максимальное время жизни сессии
idle timeout:      7 дней без активности — если пользователь не приходил 7 дней, сессия протухает
```

```go
type Session struct {
    ID         string
    TokenHash  string
    AccountID  string
    CreatedAt  time.Time
    ExpiresAt  time.Time    // absolute: CreatedAt + 30 days
    LastSeenAt time.Time    // обновляется при каждом запросе
}

const (
    absoluteTTL = 30 * 24 * time.Hour
    idleTTL     = 7 * 24 * time.Hour
)

func (s *Session) IsExpired() bool {
    now := time.Now()
    if now.After(s.ExpiresAt) {
        return true  // absolute timeout
    }
    if now.After(s.LastSeenAt.Add(idleTTL)) {
        return true  // idle timeout
    }
    return false
}

// Renewal: продлить ExpiresAt при активности, но не дальше absolute cap
func (s *Session) Renew() {
    now := time.Now()
    absoluteCap := s.CreatedAt.Add(absoluteTTL)
    newExpiry := now.Add(idleTTL)
    if newExpiry.After(absoluteCap) {
        newExpiry = absoluteCap
    }
    s.ExpiresAt = newExpiry
    s.LastSeenAt = now
}
```

---

## Ревокация и logout

Opaque сессии ревокабельны мгновенно — это главное преимущество перед JWT.

```go
// Logout: инвалидировать конкретную сессию
func (s *AuthService) Logout(ctx context.Context, token string) error {
    h := sha256.Sum256([]byte(token))
    tokenHash := hex.EncodeToString(h[:])

    // Идемпотентно — повторный logout безопасен
    if err := s.sessions.DeleteByTokenHash(ctx, tokenHash); err != nil {
        if errors.Is(err, ErrSessionNotFound) {
            return nil  // уже инвалидирована — ОК
        }
        return err
    }
    return nil
}

// Ревокация всех сессий — при смене пароля, подозрительной активности
func (s *AuthService) RevokeAllSessions(ctx context.Context, accountID string) error {
    return s.sessions.DeleteAllForAccount(ctx, accountID)
}
```

**Очистка устаревших сессий** — фоновая задача, не ждёт обращения пользователя:

```go
func (s *SessionCleaner) Run(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            deleted, err := s.repo.DeleteExpired(ctx, time.Now())
            if err != nil {
                s.log.Error("cleanup sessions", "err", err)
            } else {
                s.log.Info("cleaned expired sessions", "count", deleted)
            }
        case <-ctx.Done():
            return
        }
    }
}
```

---

## Схема таблицы сессий

```sql
CREATE TABLE sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash   CHAR(64) NOT NULL UNIQUE,   -- SHA-256 hex, не сам токен
    account_id   UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_agent   TEXT,                        -- опционально, для UI "активные сессии"
    ip_address   INET                         -- опционально
);

CREATE INDEX idx_sessions_account_id ON sessions (account_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);  -- для очистки
```

---

## Обработка ошибок без раскрытия деталей

```go
var (
    ErrSessionNotFound  = errors.New("session not found")
    ErrSessionExpired   = errors.New("session expired")
    ErrSessionRevoked   = errors.New("session revoked")
    ErrInvalidToken     = errors.New("invalid token format")
)

// В HTTP хэндлере — все ошибки аутентификации → 401, без деталей клиенту
func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (*Session, bool) {
    cookie, err := r.Cookie("session_id")
    if err != nil {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return nil, false
    }

    session, err := h.sessions.Validate(r.Context(), cookie.Value)
    if err != nil {
        // Логировать детали для отладки, клиенту — одно сообщение
        h.log.Debug("session validation failed", "err", err, "request_id", requestID(r))
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return nil, false
    }
    return session, true
}
```
