# Секреты в Docker Compose и контейнерах

## Содержание

- [Чего не делать](#чего-не-делать)
- [env_file и подстановка из shell](#env_file-и-подстановка-из-shell)
- [Mounted files для TLS и ключей](#mounted-files-для-tls-и-ключей)
- [Compose secrets](#compose-secrets)
- [Образ не должен содержать секреты](#образ-не-должен-содержать-секреты)

---

## Чего не делать

```yaml
# НЕЛЬЗЯ — секрет прямо в compose файле, попадёт в git
services:
  api:
    environment:
      DATABASE_PASSWORD: super-secret-prod-password
      JWT_SECRET: prod-jwt-secret-12345
```

```dockerfile
# НЕЛЬЗЯ — секрет в образе, виден в docker history
ENV JWT_SECRET=prod-secret
RUN echo "DB_PASS=secret" >> /app/.env
```

---

## env_file и подстановка из shell

**env_file** — compose читает переменные из файла, который не коммитится:

```yaml
services:
  api:
    build: .
    env_file:
      - .env.local     # для local dev, в .gitignore
    ports:
      - "8080:8080"
```

**Подстановка из shell** — значения берутся из окружения где запускается compose:

```yaml
services:
  api:
    environment:
      DATABASE_URL: ${DATABASE_URL}
      JWT_SECRET: ${JWT_SECRET}
      REDIS_ADDR: ${REDIS_ADDR:-localhost:6379}   # default если не задан
```

```bash
# В shell установить перед запуском
export DATABASE_URL=postgres://app:app@db:5432/app
docker compose up
```

Хорошо работает с direnv или CI/CD переменными — compose файл чистый, значения снаружи.

---

## Mounted files для TLS и ключей

Для TLS-сертификатов, private keys, JSON credentials — файлы лучше чем env:

```yaml
services:
  api:
    volumes:
      - ./secrets/dev-tls.crt:/run/secrets/tls.crt:ro
      - ./secrets/dev-tls.key:/run/secrets/tls.key:ro
      - ./secrets/google-credentials.json:/run/secrets/gcp.json:ro
    environment:
      TLS_CERT_FILE: /run/secrets/tls.crt
      TLS_KEY_FILE: /run/secrets/tls.key
      GOOGLE_APPLICATION_CREDENTIALS: /run/secrets/gcp.json
```

```bash
# secrets/ в .gitignore, только dev-сертификаты
secrets/
```

Go читает путь из env:
```go
certFile := os.Getenv("TLS_CERT_FILE")
keyFile  := os.Getenv("TLS_KEY_FILE")
cert, err := tls.LoadX509KeyPair(certFile, keyFile)
```

---

## Compose secrets

Docker Compose имеет встроенный механизм `secrets` — монтирует содержимое в `/run/secrets/<name>`:

```yaml
services:
  api:
    secrets:
      - db_password
      - jwt_secret

secrets:
  db_password:
    file: ./secrets/db_password.txt   # local dev: читать из файла
  jwt_secret:
    file: ./secrets/jwt_secret.txt
```

```go
// Приложение читает из фиксированного пути
dbPass, err := os.ReadFile("/run/secrets/db_password")
```

На практике в local dev команды чаще используют `env_file` или `${VAR}` подстановку. `secrets:` полезнее в Docker Swarm или как явная документация что именно является секретом.

---

## Образ не должен содержать секреты

Один образ должен работать во всех средах с разными секретами:

```dockerfile
# Правильный Dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /app/server ./cmd/server

FROM gcr.io/distroless/base-debian12
COPY --from=builder /app/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
# Никаких ENV с секретами, никаких COPY .env
```

```bash
# Проверить что образ не содержит секреты
docker history myapp:latest
docker run --rm myapp:latest env | grep -i secret  # должно быть пусто
```

**Разные среды — разный способ инжекта, один образ:**
```bash
# local
docker run -e DATABASE_URL=postgres://local... myapp:latest

# production (из secret manager через CI/CD)
docker run -e DATABASE_URL=$PROD_DB_URL myapp:latest
```
