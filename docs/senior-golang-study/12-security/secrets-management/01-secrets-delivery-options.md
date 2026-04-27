# Способы доставки секретов

Секрет — любое значение которое не должно утечь: DB password, API key, JWT signing key, TLS private key. Главный вопрос не "где лежит", а как попадает в runtime и где может случайно утечь.

## Содержание

- [Способы доставки](#способы-доставки)
- [Загрузка конфига в Go](#загрузка-конфига-в-go)
- [Blast radius при утечке](#blast-radius-при-утечке)
- [Чего никогда не делать](#чего-никогда-не-делать)

---

## Способы доставки

### Environment variables

Самый распространённый способ для Cloud/12-factor приложений.

```go
db, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
```

**Плюсы:** простота, поддерживается везде, хорошо с 12-factor config.  
**Минусы:** значения видны в `/proc/<pid>/environ`, могут попасть в crash dumps, debug-вывод, `docker inspect`. Не подходят для multiline (TLS private key).

### Файлы (mounted secrets)

Предпочтительны для TLS-ключей, JSON credentials, multiline-секретов.

```go
certPEM, err := os.ReadFile(os.Getenv("TLS_CERT_FILE"))
keyPEM, err := os.ReadFile(os.Getenv("TLS_KEY_FILE"))
```

Kubernetes монтирует Secrets как файлы в `/run/secrets/` — приложение читает файл, путь берёт из env.

**Плюсы:** подходят для крупных секретов, не светятся в env.  
**Минусы:** нужно управлять путями и правами на файлы.

### External secret manager

Приложение само вызывает хранилище при старте или на лету:

```go
import secretmanager "cloud.google.com/go/secretmanager/apiv1"

func loadSecret(ctx context.Context, name string) (string, error) {
    client, err := secretmanager.NewClient(ctx)
    if err != nil {
        return "", err
    }
    defer client.Close()

    req := &secretmanagerpb.AccessSecretVersionRequest{
        Name: name,  // "projects/my-project/secrets/db-password/versions/latest"
    }
    result, err := client.AccessSecretVersion(ctx, req)
    if err != nil {
        return "", fmt.Errorf("access secret %q: %w", name, err)
    }
    return string(result.Payload.Data), nil
}
```

**Плюсы:** централизованный аудит доступа, ротация, версионирование, fine-grained permissions.  
**Минусы:** зависимость от внешней системы при старте, сложнее local dev.

---

## Загрузка конфига в Go

**Паттерн: Config struct + envconfig:**

```go
import "github.com/kelseyhightower/envconfig"

type Config struct {
    DatabaseURL  string `envconfig:"DATABASE_URL" required:"true"`
    JWTSecret    string `envconfig:"JWT_SECRET" required:"true"`
    RedisAddr    string `envconfig:"REDIS_ADDR" default:"localhost:6379"`
    TLSCertFile  string `envconfig:"TLS_CERT_FILE"`
    TLSKeyFile   string `envconfig:"TLS_KEY_FILE"`
}

func LoadConfig() (Config, error) {
    var cfg Config
    if err := envconfig.Process("", &cfg); err != nil {
        return Config{}, fmt.Errorf("load config: %w", err)
    }
    return cfg, nil
}
```

**Паттерн: явная проверка при старте:**

```go
func main() {
    cfg, err := LoadConfig()
    if err != nil {
        log.Fatalf("config: %v", err)  // упасть сразу, не в runtime
    }

    // Не логировать сами секреты — только факт что загрузились
    slog.Info("config loaded",
        "db_url_set", cfg.DatabaseURL != "",
        "jwt_secret_set", cfg.JWTSecret != "",
    )
}
```

**Паттерн: секрет из файла с fallback на env:**

```go
func secretFromFileOrEnv(fileEnv, valueEnv string) (string, error) {
    if path := os.Getenv(fileEnv); path != "" {
        data, err := os.ReadFile(path)
        if err != nil {
            return "", fmt.Errorf("read secret file %q: %w", path, err)
        }
        return strings.TrimSpace(string(data)), nil
    }
    val := os.Getenv(valueEnv)
    if val == "" {
        return "", fmt.Errorf("neither %s nor %s is set", fileEnv, valueEnv)
    }
    return val, nil
}

// Использование:
// JWT_SECRET_FILE=/run/secrets/jwt-key  → читает файл
// JWT_SECRET=...                        → читает env
jwtSecret, err := secretFromFileOrEnv("JWT_SECRET_FILE", "JWT_SECRET")
```

---

## Blast radius при утечке

Минимизировать ущерб при компрометации:

**Short-lived credentials** — Database password на 1 час (Vault dynamic secrets) вместо статического пароля на годы.

**Разные секреты на среду** — production и staging никогда не шарят один ключ. Если staging скомпрометирован — production не затронут.

**Scope минимален** — DB user для сервиса A имеет доступ только к схеме сервиса A, не ко всей базе.

**Ротация** — секреты меняются по расписанию, не только при инциденте.

---

## Чего никогда не делать

```go
// Хардкодить секрет в коде
const dbPassword = "super-secret-123"  // уходит в git, в бинарь

// Логировать секрет
slog.Info("connecting", "password", cfg.DatabaseURL)  // URL содержит пароль

// Хранить в git
// .env с реальными значениями закоммиченный в repo

// Один секрет на все среды
// DATABASE_URL одинаковый для prod, staging, local

// Секреты в образе
// RUN echo "JWT_SECRET=..." > /app/.env  ← видно в docker history
```

```bash
# Проверить git history на секреты
git log --all --full-history -- "*.env"
# Инструменты: trufflesecurity/trufflehog, Yelp/detect-secrets, gitleaks
```

**Build artifact должен быть отделён от секретов:** один и тот же Docker image едет в staging и production с разными секретами, подставленными при запуске.
