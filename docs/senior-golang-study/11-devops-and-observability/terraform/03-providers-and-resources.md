# Провайдеры и ресурсы

## Содержание

- [Провайдеры](#провайдеры)
- [Ресурсы (resource)](#ресурсы-resource)
- [Data sources](#data-sources)
- [Мета-аргументы](#мета-аргументы)
- [lifecycle](#lifecycle)
- [depends_on](#depends_on)

---

## Провайдеры

**Провайдер** — плагин, который умеет разговаривать с конкретным API. Terraform сам по себе не знает ничего о GCP или AWS — это знают провайдеры.

При `terraform init` Terraform скачивает нужные провайдеры из реестра (registry.terraform.io).

### Объявление провайдера

```hcl
# versions.tf — фиксировать версии провайдеров обязательно
terraform {
  required_version = ">= 1.5.0"

  required_providers {
    google = {
      source  = "hashicorp/google"  # registry.terraform.io/hashicorp/google
      version = ">= 5.0, < 7.0"    # разрешённый диапазон версий
    }
    google-beta = {
      source  = "hashicorp/google-beta"
      version = ">= 5.0, < 7.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"  # ~> 3.0 означает >= 3.0, < 4.0
    }
  }
}
```

Синтаксис версий:
- `>= 1.5.0` — версия 1.5.0 и выше
- `< 7.0` — строго меньше 7.0
- `~> 3.0` — >= 3.0, < 4.0 (patch updates allowed)
- `~> 3.1` — >= 3.1, < 3.2 (только patch)
- `= 3.1.0` — точная версия

### Конфигурация провайдера

```hcl
# Провайдер Google Cloud
provider "google" {
  project = var.project_id
  region  = var.region

  # Аутентификация — несколько вариантов:
  # 1. Файл credentials (не для продакшна, лучше ADC)
  credentials = file("credentials.json")

  # 2. Application Default Credentials — gcloud auth application-default login
  # ADC используется автоматически если credentials не указан

  # 3. Переменная окружения GOOGLE_CREDENTIALS
}

# Несколько провайдеров одного типа — aliases
provider "google" {
  alias   = "europe"
  project = var.project_id
  region  = "europe-west4"
}

provider "google" {
  alias   = "us"
  project = var.project_id
  region  = "us-central1"
}

# Использование alias
resource "google_storage_bucket" "eu_bucket" {
  provider = google.europe
  name     = "eu-bucket"
  location = "europe-west4"
}
```

### Что делает terraform init

```bash
terraform init

# 1. Читает required_providers из versions.tf
# 2. Скачивает провайдеры в .terraform/ директорию
# 3. Создаёт .terraform.lock.hcl с точными версиями

# .terraform.lock.hcl нужно коммитить в git!
# Это обеспечивает воспроизводимость для всей команды
```

---

## Ресурсы (resource)

**Ресурс** — любой объект инфраструктуры, которым управляет Terraform: Cloud Run сервис, база данных, bucket, DNS запись, правило IAM и т.д.

```hcl
# Синтаксис: resource "<тип>" "<имя>" { ... }
# Тип определяет провайдер и тип ресурса: google_cloud_run_v2_service
# Имя — локальный идентификатор в Terraform (не в облаке!)

resource "google_cloud_run_v2_service" "api" {
  name     = "my-api"          # имя в GCP
  location = var.region
  project  = var.project_id

  template {
    containers {
      image = "gcr.io/my-project/api:latest"

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }
    }
  }
}

# Обращение к атрибутам ресурса: <тип>.<имя>.<атрибут>
output "service_url" {
  value = google_cloud_run_v2_service.api.uri  # URI создаётся GCP, Terraform читает его
}
```

### Атрибуты: аргументы vs атрибуты

У ресурса два вида полей:

**Arguments** — то что ты задаёшь (входные параметры):
```hcl
resource "google_sql_database_instance" "main" {
  name             = "my-db"          # argument
  database_version = "POSTGRES_15"    # argument
  region           = "europe-west4"   # argument
}
```

**Attributes** — то что GCP возвращает после создания (читать только):
```hcl
# После создания можно прочитать:
google_sql_database_instance.main.connection_name  # "project:region:instance-name"
google_sql_database_instance.main.public_ip_address
google_sql_database_instance.main.self_link
```

Разница важна: аргументы ты указываешь, атрибуты Terraform читает из API после создания.

---

## Data sources

**Data source** — способ прочитать данные о существующем ресурсе без его создания. Используется когда ресурс создан вне Terraform или в другой конфигурации.

```hcl
# data "<тип>" "<имя>" { ... }

# Найти существующую сеть
data "google_compute_network" "default" {
  name    = "default"
  project = var.project_id
}

# Найти существующий секрет в Secret Manager
data "google_secret_manager_secret_version" "db_password" {
  secret  = "my-db-password"
  project = var.project_id
  # version = "latest"  по умолчанию
}

# Получить информацию о проекте GCP
data "google_project" "current" {
  project_id = var.project_id
}

# Использование: data.<тип>.<имя>.<атрибут>
resource "google_cloud_run_v2_service" "api" {
  template {
    containers {
      env {
        name = "DB_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = data.google_secret_manager_secret_version.db_password.secret
            version = "latest"
          }
        }
      }
    }

    vpc_access {
      network_interfaces {
        network    = data.google_compute_network.default.id
        subnetwork = "default"
      }
    }
  }
}
```

Data sources vs resources:
```
resource — Terraform управляет жизненным циклом (создаёт, изменяет, удаляет)
data     — Terraform только читает (ничего не создаёт и не удаляет)
```

---

## Мета-аргументы

Мета-аргументы — специальные аргументы которые работают для любого ресурса.

### provider

Указать нестандартный провайдер (alias):

```hcl
resource "google_storage_bucket" "us_bucket" {
  provider = google.us  # использовать провайдер с alias = "us"
  name     = "us-bucket"
  location = "us-central1"
}
```

### count

Создать несколько экземпляров (см. подробнее в [02-hcl-language.md](./02-hcl-language.md)):

```hcl
resource "google_monitoring_alert_policy" "latency" {
  count        = var.environment == "prod" ? 1 : 0
  display_name = "API latency alert"
  # ...
}
```

### for_each

Создать ресурсы из коллекции (предпочтительнее count для коллекций):

```hcl
resource "google_project_service" "apis" {
  for_each = toset([
    "run.googleapis.com",
    "sqladmin.googleapis.com",
    "secretmanager.googleapis.com",
  ])

  project = var.project_id
  service = each.value
}
```

---

## lifecycle

Lifecycle управляет поведением Terraform при изменении ресурса.

### ignore_changes

Игнорировать изменения конкретных атрибутов — Terraform не будет пытаться вернуть их к значению в конфиге:

```hcl
resource "google_cloud_run_v2_service" "api" {
  name  = "my-api"
  # ...

  template {
    containers {
      image = "gcr.io/my-project/api:bootstrap"  # начальный образ
    }
  }

  lifecycle {
    # CI/CD деплоит новые версии через gcloud, не через terraform.
    # Без ignore_changes terraform apply откатит образ к bootstrap.
    ignore_changes = [
      "template[0].containers[0].image",
    ]
  }
}
```

```hcl
resource "google_sql_database_instance" "main" {
  name = "my-db"

  lifecycle {
    # Игнорировать несколько атрибутов
    ignore_changes = [
      settings[0].maintenance_window,
      settings[0].backup_configuration,
    ]
  }
}
```

### prevent_destroy

Защита от случайного удаления важных ресурсов:

```hcl
resource "google_sql_database_instance" "main" {
  name = "production-db"

  lifecycle {
    prevent_destroy = true
    # Попытка terraform destroy или удаление ресурса из конфига
    # вызовет ошибку, а не удаление БД
  }
}
```

### create_before_destroy

По умолчанию Terraform удаляет старый ресурс, потом создаёт новый. Для ресурсов где нельзя иметь downtime — создать новый сначала:

```hcl
resource "google_compute_ssl_certificate" "main" {
  name        = "my-cert-${substr(sha256(var.cert_pem), 0, 8)}"
  private_key = var.private_key
  certificate = var.cert_pem

  lifecycle {
    create_before_destroy = true
    # Новый сертификат создаётся, переключается, старый удаляется
  }
}
```

### replace_triggered_by

Пересоздать ресурс когда меняется другой ресурс:

```hcl
resource "google_cloud_run_v2_service" "api" {
  lifecycle {
    # Пересоздать сервис при изменении конфига (например, нового секрета)
    replace_triggered_by = [
      google_secret_manager_secret_version.config,
    ]
  }
}
```

---

## depends_on

Terraform строит граф зависимостей автоматически по ссылкам между ресурсами. `depends_on` нужен только для неявных зависимостей (когда Terraform не видит связи через атрибуты):

```hcl
resource "google_project_service" "run_api" {
  service = "run.googleapis.com"
}

resource "google_cloud_run_v2_service" "api" {
  name     = "my-api"
  location = var.region

  # Terraform не видит зависимость через атрибуты,
  # но Cloud Run не создастся пока API не включён
  depends_on = [
    google_project_service.run_api,
  ]
}
```

```hcl
# Другой пример: IAM политика должна применяться после создания ресурса
resource "google_storage_bucket" "uploads" {
  name = "my-uploads"
}

resource "google_storage_bucket_iam_member" "public_read" {
  bucket = google_storage_bucket.uploads.name  # явная ссылка — depends_on не нужен
  role   = "roles/storage.objectViewer"
  member = "allUsers"
}

# Неявная зависимость — нужен depends_on
resource "null_resource" "bucket_setup" {
  provisioner "local-exec" {
    # Скрипт использует bucket, но Terraform не видит связь через атрибуты
    command = "gsutil config ${var.bucket_name}"
  }

  depends_on = [google_storage_bucket.uploads]
}
```

**Правило:** если можно выразить зависимость через атрибут (`resource_b.field = resource_a.output`) — делай это. `depends_on` — последний аргумент.

---

## Полный пример: сервис с базой данных

```hcl
# versions.tf
terraform {
  required_version = ">= 1.5.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0, < 7.0"
    }
  }
}

# variables.tf
variable "project_id" { type = string }
variable "region"     { type = string; default = "europe-west4" }
variable "environment" { type = string }
variable "api_image"  { type = string }

# main.tf
locals {
  name_prefix = "${var.project_id}-${var.environment}"
}

# Включить нужные API
resource "google_project_service" "apis" {
  for_each = toset([
    "run.googleapis.com",
    "sqladmin.googleapis.com",
  ])
  project = var.project_id
  service = each.value
}

# База данных
resource "google_sql_database_instance" "main" {
  name             = "${local.name_prefix}-db"
  database_version = "POSTGRES_15"
  region           = var.region
  project          = var.project_id

  settings {
    tier = var.environment == "prod" ? "db-g1-small" : "db-f1-micro"
  }

  lifecycle {
    prevent_destroy = true
  }

  depends_on = [google_project_service.apis["sqladmin.googleapis.com"]]
}

# Cloud Run сервис
resource "google_cloud_run_v2_service" "api" {
  name     = "${local.name_prefix}-api"
  location = var.region
  project  = var.project_id

  template {
    containers {
      image = var.api_image

      env {
        name  = "DB_CONNECTION"
        value = google_sql_database_instance.main.connection_name
      }
    }
  }

  lifecycle {
    ignore_changes = ["template[0].containers[0].image"]
  }

  depends_on = [google_project_service.apis["run.googleapis.com"]]
}

# outputs.tf
output "api_url" {
  value = google_cloud_run_v2_service.api.uri
}

output "db_connection_name" {
  value = google_sql_database_instance.main.connection_name
}
```
