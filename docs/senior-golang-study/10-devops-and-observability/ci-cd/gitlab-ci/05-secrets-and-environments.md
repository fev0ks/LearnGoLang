# GitLab CI: Secrets и Environments

## Содержание

- [CI/CD Variables: типы и уровни](#cicd-variables-типы-и-уровни)
- [Protected и Masked](#protected-и-masked)
- [Environments: деплой и история](#environments-деплой-и-история)
- [OIDC с GitLab CI](#oidc-с-gitlab-ci)
- [HashiCorp Vault интеграция](#hashicorp-vault-интеграция)
- [Антипаттерны](#антипаттерны)

---

## CI/CD Variables: типы и уровни

### Уровни хранения

```
GitLab Instance variables   → доступны всем группам/проектам
  └── Group variables       → доступны всем проектам в группе
        └── Project variables → доступны только в одном проекте
```

Приоритет при конфликте имён: Project > Group > Instance.

Настройка: Settings → CI/CD → Variables.

### Использование в pipeline

```yaml
variables:
  # Переопределить или добавить к существующим
  MY_VAR: "value"

job:
  script:
    - echo "${MY_VAR}"
    - echo "${DB_PASSWORD}"    # из GitLab Variables
    - echo "${CI_COMMIT_SHA}"  # встроенная переменная GitLab
```

### Передача переменных при ручном запуске

```yaml
variables:
  DEPLOY_ENV:
    value: "staging"
    options:
      - "dev"
      - "staging"
      - "prod"
    description: "Target deployment environment"
```

При запуске через UI или API можно передать значение:
```bash
curl --request POST \
  --header "PRIVATE-TOKEN: ${GITLAB_TOKEN}" \
  "https://gitlab.com/api/v4/projects/${PROJECT_ID}/pipeline" \
  --form "ref=main" \
  --form "variables[][key]=DEPLOY_ENV" \
  --form "variables[][value]=prod"
```

---

## Protected и Masked

### Protected variables

Protected variable доступна только в jobs на **protected branches** (main, release/*) или **protected tags**.

```
Settings → CI/CD → Variables → [variable] → Protected: ✓
```

Использование: prod credentials, API keys для production деплоя. Jobs в feature branches не получат эти переменные.

### Masked variables

Masked variable маскируется в логах: `***`. Требования для masking:
- Значение не содержит newline.
- Длина >= 8 символов.
- Нет специальных символов кроме `@`, `:`, `.`, `/`, `+`, `-`, `_`, `=`.

```yaml
# CI job log:
$ echo "${DB_PASSWORD}"
[MASKED]
```

### File type variables

Variable со значением-файлом. Создаёт временный файл с этим содержимым.

```yaml
# переменная типа File: KUBECONFIG = содержимое kubeconfig
deploy:
  script:
    - kubectl --kubeconfig="${KUBECONFIG}" apply -f k8s/
    # ${KUBECONFIG} — путь к временному файлу
```

Полезно для: kubeconfig, service account JSON, SSH private key.

---

## Environments: деплой и история

Environment в GitLab — именованная цель деплоя. Позволяет:
- Видеть историю деплоев (что и когда задеплоено).
- Открывать URL среды прямо из UI.
- Настроить Protected Environments (ограничить кто может деплоить).
- Привязать переменные к конкретной среде.

```yaml
deploy-staging:
  stage: deploy
  environment:
    name: staging
    url: https://staging.myapp.com
    # on_stop — job для "остановки" среды
    on_stop: stop-staging
    # auto_stop_in — автоматически остановить через N времени
    auto_stop_in: 1 week
  script:
    - ./deploy.sh staging

stop-staging:
  stage: deploy
  environment:
    name: staging
    action: stop    # действие: остановить среду
  when: manual
  script:
    - ./teardown.sh staging
```

### Dynamic environments (review apps)

Для каждого MR создать отдельную среду:

```yaml
deploy-review:
  stage: deploy
  environment:
    name: review/$CI_COMMIT_REF_SLUG    # review/feature-auth, review/fix-bug
    url: https://${CI_COMMIT_REF_SLUG}.review.myapp.com
    on_stop: stop-review
    auto_stop_in: 3 days
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  script:
    - ./deploy.sh review "${CI_COMMIT_REF_SLUG}"

stop-review:
  stage: deploy
  environment:
    name: review/$CI_COMMIT_REF_SLUG
    action: stop
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
      when: manual
  script:
    - ./teardown.sh review "${CI_COMMIT_REF_SLUG}"
```

### Protected Environments

Settings → Environments → [env name] → Protected: ✓

Настройки:
- **Allowed to deploy**: только определённые роли/пользователи могут триггерить deploy jobs.
- **Required approvals**: N человек должны одобрить перед выполнением.

Protected environments также изолируют variables: secret привязанный к `production` environment доступен только deploy jobs с `environment: production`.

---

## OIDC с GitLab CI

GitLab CI поддерживает OIDC — runner получает JWT и обменивает его на cloud credentials.

```yaml
job:
  id_tokens:
    AWS_TOKEN:
      aud: "https://sts.amazonaws.com"    # audience для AWS
    GCP_TOKEN:
      aud: "https://iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/gitlab-pool/providers/gitlab-provider"
```

### OIDC с AWS

```bash
# Настройка AWS OIDC Provider
aws iam create-open-id-connect-provider \
  --url "https://gitlab.com" \
  --client-id-list "https://sts.amazonaws.com" \
  --thumbprint-list "$(openssl s_client -connect gitlab.com:443 -showcerts < /dev/null 2>/dev/null | openssl x509 -fingerprint -noout | cut -d= -f2 | tr -d ':')"
```

```yaml
deploy-aws:
  id_tokens:
    AWS_TOKEN:
      aud: "https://sts.amazonaws.com"
  environment: production
  script:
    - |
      # Получить credentials через OIDC
      ROLE_ARN="arn:aws:iam::ACCOUNT:role/GitLabCIRole"
      export $(aws sts assume-role-with-web-identity \
        --role-arn "${ROLE_ARN}" \
        --role-session-name "gitlab-ci-${CI_JOB_ID}" \
        --web-identity-token "${AWS_TOKEN}" \
        --query 'Credentials.[AccessKeyId,SecretAccessKey,SessionToken]' \
        --output text | \
        awk '{print "AWS_ACCESS_KEY_ID="$1"\nAWS_SECRET_ACCESS_KEY="$2"\nAWS_SESSION_TOKEN="$3}')

      # Теперь можно использовать aws cli
      aws ecr get-login-password | docker login ...
```

### OIDC с GCP

```yaml
deploy-gcp:
  id_tokens:
    GCP_TOKEN:
      aud: "https://iam.googleapis.com/projects/${GCP_PROJECT_NUMBER}/locations/global/workloadIdentityPools/gitlab-pool/providers/gitlab-provider"
  script:
    - |
      # Обменять JWT на GCP access token
      gcloud auth login --brief --cred-file <(cat <<EOF
      {
        "type": "external_account",
        "audience": "//iam.googleapis.com/projects/${GCP_PROJECT_NUMBER}/locations/global/workloadIdentityPools/gitlab-pool/providers/gitlab-provider",
        "subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
        "token_url": "https://sts.googleapis.com/v1/token",
        "service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/${GCP_SA}:generateAccessToken",
        "credential_source": {
          "file": "${GCP_TOKEN}"
        }
      }
      EOF
      )
      gcloud run services update ...
```

---

## HashiCorp Vault интеграция

GitLab CI имеет нативную интеграцию с HashiCorp Vault через JWT.

Настройка Vault:
```bash
# Включить JWT auth
vault auth enable jwt
vault write auth/jwt/config \
  jwks_url="https://gitlab.com/-/jwks" \
  bound_issuer="https://gitlab.com"

# Создать политику
vault policy write ci-policy - <<EOF
path "secret/data/myapp/*" {
  capabilities = ["read"]
}
EOF

# Создать роль
vault write auth/jwt/role/my-role \
  role_type="jwt" \
  policies="ci-policy" \
  token_ttl="1h" \
  bound_claims='{"project_path": "mygroup/myrepo", "ref": "main"}'
```

Использование в pipeline:

```yaml
deploy:
  secrets:
    DB_PASSWORD:
      vault: myapp/production/db-password@secret    # path@mount
      file: false    # как переменную, не файл
    TLS_CERT:
      vault: myapp/production/tls-cert@secret
      file: true     # как файл
  script:
    - echo "${DB_PASSWORD}"  # доступна как переменная
    - cat "${TLS_CERT}"      # TLS_CERT — путь к временному файлу
```

GitLab автоматически получает JWT, обменивает у Vault на токен, читает secret и передаёт в job как переменную.

---

## Антипаттерны

**Незащищённые prod secrets** — DB_PASSWORD не помечена как Protected и доступна в feature branches. Всегда помечать prod credentials как Protected.

**Не masked secrets в логах** — `echo "${API_KEY}"` в script выводит ключ если переменная не Masked.

**Хардкод credentials в `.gitlab-ci.yml`** — даже в private репо это плохая практика.

```yaml
# плохо
script:
  - docker login -u myuser -p hardcodedpassword registry.example.com

# хорошо
script:
  - docker login -u "${REGISTRY_USER}" -p "${REGISTRY_PASSWORD}" "${REGISTRY}"
```

**Variables в artifacts** — генерировать `.env` файл с секретами и сохранять как artifact. Артефакты доступны для скачивания членам проекта.

**Один набор credentials для всех сред** — `DB_PASSWORD` одна и та же для dev, staging, prod. При компрометации dev credentials — prod под угрозой. Разделять по environment.

**Не использовать OIDC при наличии возможности** — хранить AWS_SECRET_ACCESS_KEY в GitLab Variables вместо настройки OIDC. Long-lived keys = постоянный риск.
