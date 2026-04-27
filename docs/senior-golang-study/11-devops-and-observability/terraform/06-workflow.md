# Workflow

Стандартный цикл работы с Terraform: init → validate → plan → apply. В CI/CD: plan на PR, apply на merge.

## Содержание

- [Основные команды](#основные-команды)
- [terraform plan в деталях](#terraform-plan-в-деталях)
- [terraform apply в деталях](#terraform-apply-в-деталях)
- [Полезные флаги](#полезные-флаги)
- [terraform fmt и validate](#terraform-fmt-и-validate)
- [CI/CD паттерн](#cicd-паттерн)
- [Практические советы](#практические-советы)

---

## Основные команды

```bash
# 1. Инициализация — первый запуск или при смене backend/провайдеров
terraform init

# 2. Проверка синтаксиса
terraform validate

# 3. Форматирование кода
terraform fmt -recursive

# 4. План — показать что изменится (ничего не трогает!)
terraform plan

# 5. Применить изменения
terraform apply

# 6. Удалить всё что создано Terraform
terraform destroy

# Полная последовательность для нового окружения:
terraform init
terraform plan
terraform apply
```

---

## terraform plan в деталях

Plan — самая важная команда. Показывает точно что произойдёт, прежде чем что-то изменить.

```bash
terraform plan

# Вывод с объяснением символов:
#
# Terraform will perform the following actions:
#
#   # google_storage_bucket.uploads will be created
#   + resource "google_storage_bucket" "uploads" {
#       + name     = "my-project-dev-uploads"    # + = новый атрибут
#       + location = "EUROPE-WEST4"
#     }
#
#   # google_cloud_run_v2_service.api will be updated in-place
#   ~ resource "google_cloud_run_v2_service" "api" {
#       ~ template {
#           ~ containers {
#               ~ image = "api:v1.0" -> "api:v1.1"   # ~ = изменение
#             }
#         }
#     }
#
#   # google_sql_database_instance.old will be destroyed
#   - resource "google_sql_database_instance" "old" {
#       - name = "old-db"    # - = удаление
#     }
#
#   # google_sql_database_instance.main must be replaced
#   -/+ resource "google_sql_database_instance" "main" {
#       ~ name = "old-name" -> "new-name" # forces replacement
#     }                                   # -/+ = пересоздание!

# Plan: 1 to add, 1 to change, 1 to destroy.
```

**Символы plan:**
- `+` создать
- `-` удалить
- `~` изменить на месте
- `-/+` пересоздать (удалить + создать) — ОПАСНО для stateful ресурсов!
- `<=` read (data source — только чтение)

`-/+` (forces replacement) нужно проверять особенно внимательно. Некоторые атрибуты нельзя изменить без пересоздания ресурса. Например, изменение имени CloudSQL инстанса → его удаление и создание нового (потеря данных!).

### Сохранить план в файл

```bash
# Сохранить план — потом apply применит именно этот план
terraform plan -out=tfplan

# Применить сохранённый план (не спрашивает подтверждения)
terraform apply tfplan

# Просмотреть сохранённый план в читаемом виде
terraform show tfplan

# Просмотреть план в JSON
terraform show -json tfplan | jq .
```

Сохранённый план гарантирует что apply применит именно то что ты видел в plan. В CI/CD: сохранить plan на шаге `plan`, применить на шаге `apply`.

---

## terraform apply в деталях

```bash
# Apply с подтверждением
terraform apply

# Apply без подтверждения (для CI/CD)
terraform apply -auto-approve

# Apply из сохранённого плана
terraform apply tfplan  # не спрашивает подтверждения

# Apply только конкретного ресурса (точечное применение)
terraform apply -target=google_storage_bucket.uploads
terraform apply -target=module.api_service

# Apply с передачей переменных
terraform apply -var="environment=prod" -var="project_id=my-project"
terraform apply -var-file="prod.tfvars"
```

**`-target` — использовать только при отладке**, не в обычном workflow. Это нарушает целостность state: другие ресурсы не узнают о изменениях в целевом ресурсе.

---

## Полезные флаги

```bash
# Переменные
terraform plan -var="environment=dev"
terraform plan -var-file="environments/dev.tfvars"

# Количество параллельных операций (default: 10)
terraform apply -parallelism=20  # больше параллелизма
terraform apply -parallelism=1   # последовательно, для отладки

# Не обновлять state из API перед plan (быстрее, но может быть неточно)
terraform plan -refresh=false

# Заменить конкретный ресурс (пересоздать принудительно)
terraform apply -replace=google_cloud_run_v2_service.api

# Показать подробный вывод
TF_LOG=INFO terraform apply
TF_LOG=DEBUG terraform apply  # очень подробно
```

---

## terraform fmt и validate

```bash
# Форматирование — делать всегда перед коммитом
terraform fmt              # текущая директория
terraform fmt -recursive   # рекурсивно все .tf файлы

# Проверить форматирование без изменений (для CI)
terraform fmt -check -recursive
# Выход 0 = всё отформатировано
# Выход 1 = есть нарушения

# Проверка синтаксиса и корректности конфигурации
terraform validate
# Проверяет: правильный HCL, корректные типы переменных,
# ссылки на существующие ресурсы и outputs

# Lint — дополнительные проверки (не часть стандартного Terraform)
# tflint — популярный линтер
tflint --recursive
```

---

## CI/CD паттерн

Стандартный подход для командной работы:

```
PR открыт:
  1. terraform fmt -check    # проверить форматирование
  2. terraform validate      # проверить синтаксис
  3. terraform plan          # показать изменения в PR comment
  4. (ручная проверка плана)

PR одобрен и смержен:
  5. terraform apply -auto-approve  # применить изменения
```

### GitHub Actions пример

```yaml
# .github/workflows/terraform.yml
name: Terraform

on:
  pull_request:
    paths: ['infrastructure/**']
  push:
    branches: [main]
    paths: ['infrastructure/**']

env:
  TF_VERSION: "1.7.0"

jobs:
  plan:
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: hashicorp/setup-terraform@v3
        with:
          terraform_version: ${{ env.TF_VERSION }}

      - name: Authenticate to GCP
        uses: google-github-actions/auth@v2
        with:
          workload_identity_provider: ${{ secrets.WIF_PROVIDER }}
          service_account: ${{ secrets.TF_SERVICE_ACCOUNT }}

      - name: Terraform Init
        run: terraform init
        working-directory: infrastructure/envs/prod

      - name: Terraform Format Check
        run: terraform fmt -check -recursive

      - name: Terraform Validate
        run: terraform validate
        working-directory: infrastructure/envs/prod

      - name: Terraform Plan
        id: plan
        run: terraform plan -out=tfplan -no-color
        working-directory: infrastructure/envs/prod

      - name: Comment Plan on PR
        uses: actions/github-script@v7
        with:
          script: |
            const output = `#### Terraform Plan 📖
            \`\`\`\n${{ steps.plan.outputs.stdout }}\n\`\`\``;
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: output
            })

  apply:
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    environment: production  # требует manual approval
    steps:
      - uses: actions/checkout@v4

      - uses: hashicorp/setup-terraform@v3
        with:
          terraform_version: ${{ env.TF_VERSION }}

      - name: Authenticate to GCP
        uses: google-github-actions/auth@v2
        with:
          workload_identity_provider: ${{ secrets.WIF_PROVIDER }}
          service_account: ${{ secrets.TF_SERVICE_ACCOUNT }}

      - name: Terraform Init
        run: terraform init
        working-directory: infrastructure/envs/prod

      - name: Terraform Apply
        run: terraform apply -auto-approve
        working-directory: infrastructure/envs/prod
```

### Workload Identity Federation вместо ключей

Вместо JSON ключей service account для GCP:

```yaml
# Более безопасно: WIF (Workload Identity Federation)
# GitHub Actions получает временный токен GCP без хранения ключей
- uses: google-github-actions/auth@v2
  with:
    workload_identity_provider: 'projects/123456/locations/global/workloadIdentityPools/github/providers/github'
    service_account: 'terraform-ci@my-project.iam.gserviceaccount.com'
```

---

## Практические советы

### Что делать если terraform plan показывает неожиданные изменения

```bash
# 1. Обновить state из реального API
terraform refresh

# 2. Посмотреть что в state vs что в конфиге
terraform state show google_cloud_run_v2_service.api

# 3. Если ресурс изменён вне Terraform — drift
#    Варианты: принять изменения (import новые атрибуты),
#              или применить Terraform (вернуть к желаемому состоянию)
```

### Как проверить что apply не сломает прод

```bash
# 1. Всегда запускать plan перед apply
# 2. Искать -/+ (forces replacement) — пересоздание stateful ресурсов
# 3. Проверять что удаляется только то что должно удаляться
# 4. Для prod — запускать plan в отдельной среде сначала

# Считать изменения в плане (для ревью)
terraform plan 2>&1 | grep -E "^Plan:"
# Plan: 3 to add, 1 to change, 0 to destroy.
```

### Откат изменений

Terraform не имеет встроенного rollback. Откат = применить предыдущую версию конфига:

```bash
# Откатить к предыдущему коммиту
git revert HEAD  # или git checkout предыдущий-коммит
terraform apply  # применить откат

# Если state повреждён — восстановить из GCS versioning
gsutil cp gs://my-terraform-state/prod/terraform.tfstate#<version_id> ./terraform.tfstate
terraform state push terraform.tfstate
```

### Debugging

```bash
# Подробные логи
TF_LOG=DEBUG terraform apply 2>&1 | tee terraform.log

# Только логи провайдера
TF_LOG_PROVIDER=DEBUG terraform plan

# Посмотреть граф зависимостей
terraform graph | dot -Tpng > graph.png  # требует graphviz
```
