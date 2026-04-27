# OAuth 2.0 и OpenID Connect

OAuth 2.0 — протокол авторизации: пользователь разрешает вашему приложению доступ к своим данным у провайдера (Google, GitHub). OpenID Connect (OIDC) — слой поверх OAuth 2.0, добавляет идентификацию: кто этот пользователь.

## Содержание

- [Authorization Code + PKCE flow](#authorization-code--pkce-flow)
- [State — защита от CSRF](#state--защита-от-csrf)
- [PKCE — защита code interception](#pkce--защита-code-interception)
- [Nonce — защита от replay](#nonce--защита-от-replay)
- [Верификация ID Token](#верификация-id-token)
- [Account linking](#account-linking)
- [Реализация в Go](#реализация-в-go)

---

## Authorization Code + PKCE flow

```
Пользователь    Ваш сервер          Google
     │                │                 │
     │  Нажал "Sign   │                 │
     │  in with Google│                 │
     │────────────────▶                 │
     │                │                 │
     │                │ Сгенерировать:  │
     │                │ - state         │
     │                │ - code_verifier │
     │                │ - nonce         │
     │                │ Сохранить в БД  │
     │                │                 │
     │  Redirect →    │                 │
     │  accounts.google.com?            │
     │  client_id=...&                  │
     │  redirect_uri=...&               │
     │  code_challenge=hash(verifier)&  │
     │  state=...&                      │
     │  nonce=...                       │
     │◀───────────────                  │
     │                                  │
     │  Логин и согласие у Google       │
     │──────────────────────────────────▶
     │                                  │
     │  Redirect → /callback?code=...&state=...
     │◀──────────────────────────────────
     │────────────────▶                 │
     │                │ Проверить state │
     │                │ Exchange code   │
     │                │ + code_verifier │──────────────────▶
     │                │                 │   access_token
     │                │                 │   id_token (JWT)
     │                │◀──────────────────────────────────
     │                │                 │
     │                │ Верифицировать id_token
     │                │ Найти/создать аккаунт
     │                │ Выдать сессию   │
     │◀───────────────                  │
```

---

## State — защита от CSRF

`state` — случайная строка, которую сервер генерирует перед редиректом к провайдеру и проверяет в callback. Без него атакующий может подделать callback и привязать свой Google-аккаунт к аккаунту жертвы.

```go
func (h *OAuthHandler) Initiate(w http.ResponseWriter, r *http.Request) {
    state, err := generateRandomToken(32)
    if err != nil {
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }

    // Хранить state server-side с TTL — НЕ в cookie
    if err := h.store.SaveOAuthState(r.Context(), OAuthState{
        Token:     state,
        ExpiresAt: time.Now().Add(10 * time.Minute),
        // Можно добавить: IP, redirect_to
    }); err != nil {
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }

    url := h.googleConfig.AuthCodeURL(state,
        oauth2.AccessTypeOnline,
        // ... другие параметры
    )
    http.Redirect(w, r, url, http.StatusFound)
}

func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
    state := r.URL.Query().Get("state")
    code := r.URL.Query().Get("code")

    // Атомарно: найти и удалить — single-use, защита от replay
    saved, err := h.store.PopOAuthState(r.Context(), state)
    if err != nil || saved == nil {
        http.Error(w, "invalid state", http.StatusBadRequest)
        return
    }
    if time.Now().After(saved.ExpiresAt) {
        http.Error(w, "state expired", http.StatusBadRequest)
        return
    }
    // ... продолжить
}
```

**Почему state хранится в БД, а не в cookie:**  
Cookie с state можно подменить (cookie injection). Server-side store гарантирует что state был выпущен именно этим сервером.

---

## PKCE — защита code interception

PKCE (Proof Key for Code Exchange) защищает от атаки где authorization code перехватывается (например, через другое приложение на мобильном, через open redirect).

```go
import "crypto/sha256"

func generatePKCE() (verifier, challenge string, err error) {
    b := make([]byte, 32)
    if _, err = rand.Read(b); err != nil {
        return "", "", err
    }
    verifier = base64.RawURLEncoding.EncodeToString(b)

    // challenge = base64url(sha256(verifier))
    h := sha256.Sum256([]byte(verifier))
    challenge = base64.RawURLEncoding.EncodeToString(h[:])
    return verifier, challenge, nil
}

// При инициации: отправить challenge провайдеру, сохранить verifier server-side
url := config.AuthCodeURL(state,
    oauth2.SetAuthURLParam("code_challenge", challenge),
    oauth2.SetAuthURLParam("code_challenge_method", "S256"),
)

// При обмене кода: отправить verifier провайдеру для проверки
token, err := config.Exchange(ctx, code,
    oauth2.SetAuthURLParam("code_verifier", verifier),
)
```

---

## Nonce — защита от replay

`nonce` защищает от replay атаки на ID Token: атакующий перехватывает чужой ID Token и пытается использовать его для входа.

```go
// При инициации: сгенерировать nonce, включить в запрос к провайдеру
nonce, _ := generateRandomToken(32)
h.store.SaveOAuthState(ctx, OAuthState{
    Token:  state,
    Nonce:  nonce,
    // ...
})

url := config.AuthCodeURL(state,
    oauth2.SetAuthURLParam("nonce", nonce),
    // ...
)

// В callback: после получения ID Token проверить что nonce совпадает
func verifyIDToken(ctx context.Context, rawIDToken string, expectedNonce string) (*oidc.IDToken, error) {
    verifier := provider.Verifier(&oidc.Config{ClientID: clientID})

    idToken, err := verifier.Verify(ctx, rawIDToken)
    if err != nil {
        return nil, fmt.Errorf("verify id token: %w", err)
    }

    var claims struct {
        Nonce string `json:"nonce"`
    }
    if err := idToken.Claims(&claims); err != nil {
        return nil, fmt.Errorf("extract claims: %w", err)
    }
    if claims.Nonce != expectedNonce {
        return nil, errors.New("nonce mismatch")
    }
    return idToken, nil
}
```

---

## Верификация ID Token

ID Token — JWT выданный провайдером. Нельзя доверять без верификации.

```go
import "github.com/coreos/go-oidc/v3/oidc"

type GoogleClaims struct {
    Subject       string `json:"sub"`            // стабильный уникальный ID пользователя
    Email         string `json:"email"`
    EmailVerified bool   `json:"email_verified"`
    Name          string `json:"name"`
    Nonce         string `json:"nonce"`
}

func (h *OAuthHandler) verifyGoogleToken(ctx context.Context, rawToken, expectedNonce string) (*GoogleClaims, error) {
    provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
    if err != nil {
        return nil, err
    }

    verifier := provider.Verifier(&oidc.Config{
        ClientID: h.config.ClientID,
    })

    // Библиотека проверяет: подпись, iss, aud, exp
    idToken, err := verifier.Verify(ctx, rawToken)
    if err != nil {
        return nil, fmt.Errorf("token verification: %w", err)
    }

    var claims GoogleClaims
    if err := idToken.Claims(&claims); err != nil {
        return nil, fmt.Errorf("extract claims: %w", err)
    }

    // Проверить nonce вручную — библиотека не делает это автоматически
    if claims.Nonce != expectedNonce {
        return nil, errors.New("nonce mismatch")
    }

    // Subject обязателен — это дурабельный идентификатор пользователя у провайдера
    if claims.Subject == "" {
        return nil, errors.New("missing sub claim")
    }

    return &claims, nil
}
```

**Что обязательно проверять вручную:**

| Claim | Что проверять |
|---|---|
| `iss` | совпадает с ожидаемым провайдером (библиотека делает) |
| `aud` | совпадает с вашим `client_id` (библиотека делает) |
| `exp` | токен не истёк (библиотека делает) |
| `nonce` | совпадает с сохранённым (вручную) |
| `sub` | не пустой (вручную) |
| `email_verified` | проверить перед account linking (вручную) |

---

## Account linking

Правило: привязать OAuth-аккаунт к существующему только если `email_verified=true` и email совпадает. Иначе — создать новый аккаунт.

```go
func (s *OAuthService) HandleCallback(ctx context.Context, claims *GoogleClaims) (*Account, error) {
    // Шаг 1: поискать по provider subject (sub) — дурабельный ID
    identity, err := s.repo.GetProviderIdentity(ctx, "google", claims.Subject)
    if err == nil {
        // Известный Google пользователь — просто логин
        account, err := s.repo.GetAccount(ctx, identity.AccountID)
        if err != nil {
            return nil, err
        }
        return account, nil
    }
    if !errors.Is(err, ErrNotFound) {
        return nil, err
    }

    // Шаг 2: новый Google пользователь — попробовать привязать к существующему аккаунту
    if claims.EmailVerified && claims.Email != "" {
        existing, err := s.repo.GetAccountByEmail(ctx, claims.Email)
        if err == nil {
            // Привязать Google к существующему аккаунту
            if err := s.repo.CreateProviderIdentity(ctx, ProviderIdentity{
                AccountID:      existing.ID,
                Provider:       "google",
                ProviderSub:    claims.Subject,
                ProviderEmail:  claims.Email,
            }); err != nil {
                return nil, err
            }
            return existing, nil
        }
        if !errors.Is(err, ErrNotFound) {
            return nil, err
        }
    }
    // email_verified=false или аккаунта нет → создать новый аккаунт
    return s.createAccountFromOAuth(ctx, claims)
}
```

**Почему `sub`, а не `email` как primary key провайдера:**  
Email у пользователя может измениться. `sub` (subject) — постоянный идентификатор конкретного аккаунта у провайдера, не меняется никогда.

---

## Реализация в Go

**Пакеты:**
- `golang.org/x/oauth2` — OAuth 2.0 клиент, exchange, token refresh
- `github.com/coreos/go-oidc/v3/oidc` — OIDC верификация, discovery endpoint, ID Token parsing
- `golang.org/x/oauth2/google` — Google-специфичные endpoints

```go
import (
    "golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
    "github.com/coreos/go-oidc/v3/oidc"
)

func NewGoogleOAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
    return &oauth2.Config{
        ClientID:     clientID,
        ClientSecret: clientSecret,
        RedirectURL:  redirectURL,
        Scopes: []string{
            oidc.ScopeOpenID,
            "email",
            "profile",
        },
        Endpoint: google.Endpoint,
    }
}
```

**Что не хранить в логах и БД:**
- authorization code
- access token провайдера
- refresh token
- raw ID token
- code_verifier
