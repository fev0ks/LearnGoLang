# GitHub Actions: Secrets и OIDC

## Содержание

- [Secrets vs Variables](#secrets-vs-variables)
- [Уровни хранения secrets](#уровни-хранения-secrets)
- [Environments: изоляция secrets по среде](#environments-изоляция-secrets-по-среде)
- [OIDC: аутентификация без long-lived keys](#oidc-аутентификация-без-long-lived-keys)
- [OIDC с GCP](#oidc-с-gcp)
- [OIDC с AWS](#oidc-с-aws)
- [Передача secrets в reusable workflows](#передача-secrets-в-reusable-workflows)
- [Антипаттерны](#антипаттерны)

---

## Secrets vs Variables

| | `secrets.*` | `vars.*` |
|---|---|---|
| Видимость в логах | Маскируется `***` | Видна |
| Использование | Токены, пароли, ключи | URL, имена проектов, флаги |
| Доступ | Только в workflow | В workflow и UI |
| Вывод через echo | Заблокирован | Разрешён |

```yaml
env:
  DATABASE_URL: ${{ secrets.DATABASE_URL }}      # маскируется
  GCP_REGION: ${{ vars.GCP_REGION }}             # видна: europe-west4
  APP_NAME: ${{ vars.APP_NAME }}
```

Если попытаться вывести secret через `echo`:
```bash
echo "${{ secrets.MY_SECRET }}"
# вывод: ***
```

---

## Уровни хранения secrets

```
Organization secrets   → доступны во всех репо организации
  └── Repository secrets  → доступны в конкретном репо
        └── Environment secrets  → доступны только когда job использует этот environment
```

Приоритет: Environment > Repository > Organization.

Когда что использовать:
- **Organization**: общие для всех команд (Slack webhook, Sentry DSN, общий registry token).
- **Repository**: специфичные для проекта (DB credentials, deploy keys).
- **Environment**: prod secrets — только deploy jobs с `environment: prod`.

---

## Environments: изоляция secrets по среде

Environments позволяют:
- Хранить secrets отдельно для dev/staging/prod.
- Настроить required reviewers (manual approve).
- Ограничить деплой только из определённых веток.
- Видеть историю деплоев в GitHub UI.

Настройка: Settings → Environments → New environment.

```yaml
jobs:
  deploy-prod:
    runs-on: ubuntu-latest
    needs: deploy-staging
    environment:
      name: prod
      url: https://myapp.com    # отображается в GitHub UI

    steps:
      - name: Deploy
        run: ./deploy.sh
        env:
          # этот secret доступен только когда environment: prod
          PROD_DB_PASSWORD: ${{ secrets.DB_PASSWORD }}
          PROD_API_KEY: ${{ secrets.API_KEY }}
```

Protection rules для `prod` environment:
- **Required reviewers**: кто должен approve деплой.
- **Wait timer**: задержка перед деплоем (например, 5 минут).
- **Deployment branches**: только `main` или `release/*`.

---

## OIDC: аутентификация без long-lived keys

**Проблема long-lived keys**: service account key живёт бесконечно. Утечка = компрометация. Нужно ротировать вручную.

**OIDC решение**: GitHub Actions получает краткоживущий JWT (10 минут), обменивает его у cloud provider на access token с нужными правами.

```
[GitHub runner]
    │
    │  1. Запрашивает JWT у GitHub OIDC endpoint
    ▼
[GitHub OIDC]
    │  JWT содержит: repo, workflow, ref, actor, environment
    │
    │  2. Отправляет JWT cloud provider
    ▼
[GCP/AWS/Azure OIDC endpoint]
    │  3. Проверяет подпись JWT и claims
    │  4. Возвращает short-lived access token
    ▼
[GitHub runner]
    │  5. Использует token для API вызовов
```

Требования:
```yaml
permissions:
  id-token: write    # разрешить получать OIDC JWT
  contents: read
```

---

## OIDC с GCP

**Шаг 1: Настроить Workload Identity Federation в GCP**

```bash
# Создать Workload Identity Pool
gcloud iam workload-identity-pools create "github-pool" \
  --project="${GCP_PROJECT_ID}" \
  --location="global" \
  --display-name="GitHub Actions Pool"

# Создать Provider в пуле
gcloud iam workload-identity-pools providers create-oidc "github-provider" \
  --project="${GCP_PROJECT_ID}" \
  --location="global" \
  --workload-identity-pool="github-pool" \
  --display-name="GitHub provider" \
  --attribute-mapping="google.subject=assertion.sub,attribute.actor=assertion.actor,attribute.repository=assertion.repository" \
  --issuer-uri="https://token.actions.githubusercontent.com"

# Дать Service Account права на Workload Identity
gcloud iam service-accounts add-iam-policy-binding "ci-sa@${GCP_PROJECT_ID}.iam.gserviceaccount.com" \
  --project="${GCP_PROJECT_ID}" \
  --role="roles/iam.workloadIdentityUser" \
  --member="principalSet://iam.googleapis.com/projects/${GCP_PROJECT_NUMBER}/locations/global/workloadIdentityPools/github-pool/attribute.repository/myorg/myrepo"
```

**Шаг 2: Использовать в workflow**

```yaml
permissions:
  id-token: write
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      # Аутентификация через OIDC — никаких секретов в репо!
      - name: Authenticate to GCP
        id: auth
        uses: google-github-actions/auth@v2
        with:
          workload_identity_provider: ${{ vars.GCP_WORKLOAD_IDENTITY_PROVIDER }}
          service_account: ${{ vars.GCP_SERVICE_ACCOUNT }}
          # workload_identity_provider format:
          # projects/PROJECT_NUMBER/locations/global/workloadIdentityPools/POOL/providers/PROVIDER

      - name: Setup gcloud
        uses: google-github-actions/setup-gcloud@v2

      - name: Push to Artifact Registry
        run: |
          gcloud auth configure-docker europe-west4-docker.pkg.dev
          docker push europe-west4-docker.pkg.dev/myproject/myrepo/app:${{ github.sha }}

      - name: Deploy to Cloud Run
        run: |
          gcloud run services update my-service \
            --region=europe-west4 \
            --image=europe-west4-docker.pkg.dev/myproject/myrepo/app@${{ steps.build.outputs.digest }}
```

`GCP_WORKLOAD_IDENTITY_PROVIDER` и `GCP_SERVICE_ACCOUNT` — не секретные (не содержат credentials), хранить в `vars.*`.

---

## OIDC с AWS

```bash
# Создать OIDC provider в AWS IAM
aws iam create-open-id-connect-provider \
  --url "https://token.actions.githubusercontent.com" \
  --client-id-list "sts.amazonaws.com" \
  --thumbprint-list "6938fd4d98bab03faadb97b34396831e3780aea1"

# Создать IAM Role с trust policy
cat > trust-policy.json << 'EOF'
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "Federated": "arn:aws:iam::ACCOUNT_ID:oidc-provider/token.actions.githubusercontent.com" },
    "Action": "sts:AssumeRoleWithWebIdentity",
    "Condition": {
      "StringEquals": {
        "token.actions.githubusercontent.com:aud": "sts.amazonaws.com",
        "token.actions.githubusercontent.com:sub": "repo:myorg/myrepo:environment:prod"
      }
    }
  }]
}
EOF
```

```yaml
permissions:
  id-token: write
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: prod    # sub claim включает environment

    steps:
      - name: Configure AWS credentials (OIDC)
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: ${{ vars.AWS_ROLE_ARN }}
          aws-region: ${{ vars.AWS_REGION }}
          # Никакого AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY

      - name: Push to ECR
        run: |
          aws ecr get-login-password | docker login \
            --username AWS \
            --password-stdin \
            ${{ vars.AWS_ACCOUNT_ID }}.dkr.ecr.${{ vars.AWS_REGION }}.amazonaws.com
```

---

## Передача secrets в reusable workflows

Secrets не наследуются автоматически — нужно явно передавать:

```yaml
# вызывающий workflow
jobs:
  deploy:
    uses: ./.github/workflows/deploy.yml
    with:
      environment: prod
    secrets:
      DEPLOY_TOKEN: ${{ secrets.PROD_DEPLOY_TOKEN }}
      # или передать все secrets автоматически:
      # secrets: inherit
```

```yaml
# reusable workflow
on:
  workflow_call:
    secrets:
      DEPLOY_TOKEN:
        required: true

jobs:
  run:
    steps:
      - run: deploy.sh
        env:
          TOKEN: ${{ secrets.DEPLOY_TOKEN }}
```

`secrets: inherit` — передаёт все secrets вызывающего workflow. Удобно, но нарушает принцип минимальных привилегий.

---

## Антипаттерны

**Long-lived service account keys** — хранить AWS_SECRET_ACCESS_KEY или GCP JSON key в secrets. Решение: OIDC.

**Выводить secrets в логи через `echo`** — GHA маскирует известные secrets, но только если они >= 3 символов и не разбиты пробелами.

**`secrets: inherit` повсеместно** — все reusable workflows получают prod secrets. Использовать явную передачу нужных secrets.

**Один набор secrets для всех сред** — `DB_PASSWORD` используется и для dev и для prod. Решение: environments с отдельными secrets.

**Secrets в артефактах** — загружать файлы с credentials через `upload-artifact`. Артефакты могут быть скачаны любым участником репо.

**Не ротировать secrets** — даже не OIDC secrets нужно периодически ротировать. Настроить напоминание или автоматическую ротацию.
