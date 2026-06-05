# State и бэкенды

## Содержание

- [Что такое state](#что-такое-state)
- [Remote backends](#remote-backends)
- [State locking](#state-locking)
- [Работа с state вручную](#работа-с-state-вручную)
- [terraform import](#terraform-import)
- [moved блок](#moved-блок)
- [Безопасность state](#безопасность-state)

---

## Что такое state

**State** — файл `terraform.tfstate`, в котором Terraform хранит слепок реальной инфраструктуры. Это источник правды о том, что уже создано.

```json
// terraform.tfstate (упрощённо)
{
  "version": 4,
  "resources": [
    {
      "type": "google_storage_bucket",
      "name": "uploads",
      "instances": [
        {
          "attributes": {
            "id": "my-uploads-bucket",
            "name": "my-uploads-bucket",
            "location": "EUROPE-WEST4",
            "self_link": "https://www.googleapis.com/storage/v1/b/my-uploads-bucket"
          }
        }
      ]
    }
  ]
}
```

Зачем нужен state:

**1. Сопоставление конфиг → реальный ресурс.** Terraform знает что `google_storage_bucket.uploads` в конфиге — это бакет `my-uploads-bucket` в GCP. Без этого невозможно понять что изменилось.

**2. Определение изменений.** При `terraform plan` Terraform сравнивает три вещи: желаемое состояние (конфиг), реальное состояние (API GCP), записанное состояние (state). Разница → план изменений.

**3. Хранение outputs и зависимостей.** Значения outputs из одного модуля, которые нужны другому, хранятся в state.

```
Terraform plan = конфиг .tf  vs  terraform.tfstate  vs  реальный GCP API
                                 (что было)            (что есть)
```

### Почему нельзя хранить state в git

1. **Sensitive данные.** State хранит все атрибуты ресурсов, включая пароли БД, приватные ключи, токены — в открытом виде.

2. **Конфликты.** Два инженера изменили state одновременно → конфликт merge → испорченный state → Terraform думает что ресурсы не существуют → пересоздаёт их!

3. **Нет блокировки.** Git не поддерживает лок файла при работе.

---

## Remote backends

Remote backend — хранение state файла в облаке, а не локально.

```hcl
# backend.tf

# GCS (Google Cloud Storage) — для GCP проектов
terraform {
  backend "gcs" {
    bucket = "my-terraform-state"  # bucket должен существовать заранее
    prefix = "prod/eu/cloud-run"   # путь внутри bucket
  }
}

# S3 (AWS) — для AWS проектов
terraform {
  backend "s3" {
    bucket         = "my-terraform-state"
    key            = "prod/eu/cloud-run/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "terraform-state-lock"  # таблица для locking
    encrypt        = true
  }
}

# Terraform Cloud / HCP Terraform
terraform {
  cloud {
    organization = "my-org"
    workspaces {
      name = "prod-eu-cloud-run"
    }
  }
}
```

### Создание GCS bucket для state

State bucket нужно создать вручную один раз (он сам не может управлять собой):

```bash
# Создать bucket
gsutil mb -p my-project -l europe-west4 gs://my-terraform-state

# Включить versioning (можно откатиться к предыдущему state)
gsutil versioning set on gs://my-terraform-state

# Запретить публичный доступ
gsutil pap set enforced gs://my-terraform-state
```

### Инициализация с remote backend

```bash
# При первом использовании или смене backend
terraform init

# Если локальный state уже есть — Terraform предложит мигрировать его в remote
#
# Initializing the backend...
# Do you want to copy existing state to the new backend?
#   Yes → скопировать в GCS
#   No  → начать с чистого state в GCS
```

### Backends и Terragrunt

В реальных проектах с Terragrunt backend конфигурируется автоматически:

```hcl
# root.hcl (Terragrunt генерирует backend.tf в каждой директории)
remote_state {
  backend = "gcs"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    bucket = "my-terraform-state"
    prefix = "${path_relative_to_include()}"  # автоматический путь
  }
}
```

---

## State locking

Когда два человека запускают `terraform apply` одновременно — катастрофа: оба читают одинаковый state, оба вычисляют план, оба применяют → state разъезжается с реальностью.

State locking решает эту проблему: первый `apply` захватывает лок, второй ждёт или получает ошибку.

**GCS** поддерживает locking нативно (через object metadata).

**S3** требует отдельной DynamoDB таблицы:

```hcl
# Создать DynamoDB таблицу для лока (terraform или вручную)
resource "aws_dynamodb_table" "terraform_lock" {
  name         = "terraform-state-lock"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }
}
```

### Если лок завис

```bash
# Если apply упал в середине — лок может остаться
# Список залоченных state
terraform force-unlock LOCK_ID

# LOCK_ID показывается в сообщении об ошибке:
# Error: Error locking state: Error acquiring the state lock:
# Lock Info:
#   ID: 12345678-1234-1234-1234-123456789012
#   ...
```

Никогда не снимай лок если не уверен что другой apply уже не работает.

---

## Работа с state вручную

```bash
# Показать все ресурсы в state
terraform state list

# Пример вывода:
# google_cloud_run_v2_service.api
# google_sql_database_instance.main
# google_storage_bucket.uploads
# google_project_service.apis["run.googleapis.com"]

# Показать детали конкретного ресурса
terraform state show google_sql_database_instance.main

# Переименовать ресурс в state (без пересоздания)
# Нужно при рефакторинге когда меняется имя ресурса в конфиге
terraform state mv \
  google_storage_bucket.old_name \
  google_storage_bucket.new_name

# Удалить ресурс из state (без удаления в GCP)
# Нужно когда хочешь убрать ресурс из управления Terraform
terraform state rm google_storage_bucket.uploads

# Скачать state (для просмотра или резервной копии)
terraform state pull > backup.tfstate

# Загрузить state (опасно! только если точно знаешь что делаешь)
terraform state push backup.tfstate

# Обновить state из реального API (refresh)
terraform refresh
```

---

## terraform import

Позволяет взять существующий ресурс GCP под управление Terraform без его пересоздания.

Сценарий: база данных создана вручную давно, хочешь управлять ею через Terraform.

### Современный способ (Terraform 1.5+): import блок

```hcl
# Добавить в main.tf
import {
  # id ресурса в GCP (формат зависит от типа ресурса)
  id = "projects/my-project/locations/europe-west4/services/my-existing-api"
  to = google_cloud_run_v2_service.api
}

# Описать ресурс в конфиге как обычно
resource "google_cloud_run_v2_service" "api" {
  name     = "my-existing-api"
  location = "europe-west4"
  # ...
}
```

```bash
# Запустить plan — покажет что ресурс будет импортирован
terraform plan

# Применить импорт
terraform apply
# После apply блок import можно удалить из main.tf
```

### Старый способ: terraform import CLI

```bash
# terraform import <адрес в конфиге> <id в облаке>

# Cloud Run сервис
terraform import \
  google_cloud_run_v2_service.api \
  "projects/my-project/locations/europe-west4/services/my-existing-api"

# GCS bucket
terraform import \
  google_storage_bucket.uploads \
  "my-existing-bucket"

# CloudSQL instance
terraform import \
  google_sql_database_instance.main \
  "projects/my-project/instances/my-existing-db"
```

**Важно после импорта:** ресурс добавлен в state, но конфиг `.tf` ещё не содержит его описания. Нужно написать соответствующий resource блок. Запусти `terraform plan` — он покажет расхождение между state и конфигом.

### Генерация конфига при импорте

Terraform 1.5+ умеет генерировать конфиг при импорте:

```bash
# Генерировать .tf файл для импортируемого ресурса
terraform plan -generate-config-out=generated.tf
```

Генерированный конфиг нужно почистить — убрать computed атрибуты, заменить хардкод на переменные.

---

## moved блок

При рефакторинге иногда нужно переименовать ресурс в конфиге. Без `moved` Terraform удалит старый ресурс и создаст новый. `moved` говорит Terraform что это переименование:

```hcl
# Было: google_storage_bucket.data
# Стало: google_storage_bucket.uploads

moved {
  from = google_storage_bucket.data
  to   = google_storage_bucket.uploads
}

resource "google_storage_bucket" "uploads" {
  name = "my-uploads"
  # ...
}
```

```hcl
# При переносе ресурса в модуль
moved {
  from = google_cloud_run_v2_service.api
  to   = module.cloud_run.google_cloud_run_v2_service.services["api"]
}
```

Блок `moved` можно оставить в конфиге навсегда или удалить после применения (если уверен что все уже применили).

---

## Безопасность state

State содержит **все атрибуты ресурсов в открытом виде**, включая sensitive данные.

Пример что есть в state:
```json
{
  "type": "google_sql_user",
  "attributes": {
    "password": "super-secret-password-123"  // открытый текст!
  }
}
```

**Меры защиты:**

```hcl
# 1. Шифрование at rest в GCS bucket (включено по умолчанию в GCS)
# 2. IAM на bucket — доступ только для CI/CD service account и DevOps

# 3. Для S3 — включить шифрование явно
terraform {
  backend "s3" {
    encrypt = true  # обязательно!
    # ...
  }
}

# 4. Логировать доступ к state bucket — кто и когда читал
```

**Принцип наименьших привилегий для state bucket:**
```
CI/CD service account:    roles/storage.objectAdmin на state bucket
Разработчики:             roles/storage.objectViewer (только чтение)
Terraform apply вручную: через временное повышение прав
```

**Не хранить sensitive значения в outputs если возможно:**
```hcl
output "db_password" {
  value     = random_password.db.result
  sensitive = true  # скрыть из логов, но в state всё равно есть
}

# Лучше: записать пароль в Secret Manager, выдать его имя
output "db_password_secret_name" {
  value = google_secret_manager_secret.db_password.name
}
```
