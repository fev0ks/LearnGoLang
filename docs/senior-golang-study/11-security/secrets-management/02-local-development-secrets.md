# Секреты в local development

Цель: не коммитить реальные секреты, быстрый старт для нового разработчика, изоляция от production credentials.

## Содержание

- [Базовый паттерн: .env.example](#базовый-паттерн-envexample)
- [direnv — автозагрузка в shell](#direnv--автозагрузка-в-shell)
- [Загрузка .env в Go без внешних библиотек](#загрузка-env-в-go-без-внешних-библиотек)
- [godotenv](#godotenv)
- [Чего не делать](#чего-не-делать)

---

## Базовый паттерн: .env.example

В репозитории — только `env.example` с именами переменных и dev-заглушками. Реальные значения разработчик кладёт локально.

```bash
# .env.example — коммитится в git
APP_ENV=local
DATABASE_URL=postgres://app:app@localhost:5432/app?sslmode=disable
JWT_SECRET=dev-only-change-me-in-production
REDIS_ADDR=localhost:6379
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=
```

```bash
# .gitignore
.env
.env.local
.env.*.local
```

Разработчик копирует и заполняет:
```bash
cp .env.example .env.local
# Заполнить GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET для своего dev OAuth приложения
```

**Правило:** в `.env.example` никогда не должно быть реальных значений — только `change-me-local-only`, пустые строки, или сгенерированные безопасные dev-дефолты.

---

## direnv — автозагрузка в shell

[direnv](https://direnv.net/) автоматически загружает `.envrc` при переходе в директорию и выгружает при выходе.

```bash
# .envrc
dotenv .env.local
```

```bash
direnv allow .  # разрешить один раз
# Теперь при cd в проект — переменные автоматически в shell
```

Удобно когда несколько проектов с разными конфигами — не нужно вручную `source .env`.

---

## Загрузка .env в Go без внешних библиотек

```go
// Простой loader для local dev — в production .env файла нет
func loadDotEnv(path string) error {
    f, err := os.Open(path)
    if errors.Is(err, os.ErrNotExist) {
        return nil  // в production файла нет — это нормально
    }
    if err != nil {
        return err
    }
    defer f.Close()

    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        k, v, ok := strings.Cut(line, "=")
        if !ok {
            continue
        }
        // Не перезаписывать уже установленные переменные (env > .env файл)
        if os.Getenv(strings.TrimSpace(k)) == "" {
            os.Setenv(strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"`))
        }
    }
    return scanner.Err()
}

func main() {
    // Загрузить только в dev, проигнорировать если файла нет
    if err := loadDotEnv(".env.local"); err != nil {
        log.Fatalf("load .env: %v", err)
    }
    cfg, err := LoadConfig()
    // ...
}
```

---

## godotenv

Если не хочется писать самому:

```go
import "github.com/joho/godotenv"

func main() {
    // Загрузить .env.local если существует, не падать если нет
    _ = godotenv.Load(".env.local")

    cfg, err := LoadConfig()
    // ...
}
```

```go
// Для тестов: установить переменные из файла
func TestMain(m *testing.M) {
    _ = godotenv.Load("../../.env.test")
    os.Exit(m.Run())
}
```

---

## Чего не делать

```bash
# Коммитить .env с реальными значениями
git add .env  # НИКОГДА

# Хранить production credentials в local .env
DATABASE_URL=postgres://prod-user:real-prod-pass@prod-db:5432/prod

# Копировать production secrets в personal tools
# (Slack, заметки, почта) — они там хранятся бессрочно

# Один shared long-lived token для всей команды
# — невозможно ротировать, невозможно отозвать для одного разработчика
```

**Практика:** dev credentials должны быть отдельными от production. Для OAuth — создать отдельное dev-приложение у провайдера. Для DB — отдельная локальная или shared dev база, не staging/production.
