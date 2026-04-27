# GCP паттерны

Практические паттерны для работы с Google Cloud Platform через Terraform. Охватывает ресурсы которые встречаются в большинстве backend проектов на GCP.

## Содержание

- [Аутентификация](#аутентификация)
- [Cloud Run](#cloud-run)
- [CloudSQL PostgreSQL](#cloudsql-postgresql)
- [IAM и Service Accounts](#iam-и-service-accounts)
- [Secret Manager](#secret-manager)
- [GCS Buckets](#gcs-buckets)
- [Pub/Sub](#pubsub)
- [Artifact Registry](#artifact-registry)
- [Управление API](#управление-api)

---

## Аутентификация

### Application Default Credentials (рекомендуется для локальной разработки)

```bash
# Аутентифицироваться через gcloud
gcloud auth application-default login

# Terraform автоматически использует ADC если credentials не указан в provider
```

```hcl
provider "google" {
  project = var.project_id
  region  = var.region
  # credentials не указан → использует ADC
}
```

### Service Account для CI/CD

```hcl
provider "google" {
  project     = var.project_id
  region      = var.region
  credentials = var.google_credentials_json  # JSON ключ из secret
}
```

```bash
# Или через переменную окружения
export GOOGLE_CREDENTIALS=$(cat service-account-key.json)
terraform apply
```

### Workload Identity Federation (лучший вариант для CI/CD)

Не нужны JSON ключи — GitHub Actions получает временный токен GCP:

```hcl
# Настроить WIF (делается один раз)
resource "google_iam_workload_identity_pool" "github" {
  workload_identity_pool_id = "github-pool"
  display_name              = "GitHub Actions pool"
}

resource "google_iam_workload_identity_pool_provider" "github" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-provider"

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
    "attribute.actor"      = "assertion.actor"
  }

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

# Разрешить GitHub repo использовать service account
resource "google_service_account_iam_member" "github_actions" {
  service_account_id = google_service_account.terraform_ci.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/my-org/my-repo"
}
```

---

## Cloud Run

### Базовый сервис

```hcl
resource "google_cloud_run_v2_service" "api" {
  name     = "my-api"
  location = var.region
  project  = var.project_id

  # INGRESS_TRAFFIC_ALL — доступен из интернета
  # INGRESS_TRAFFIC_INTERNAL_ONLY — только из VPC и Cloud Load Balancing
  # INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER — только через Load Balancer
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.api.email

    scaling {
      min_instance_count = var.environment == "prod" ? 1 : 0
      max_instance_count = var.environment == "prod" ? 20 : 5
    }

    # Таймаут запроса
    timeout = "300s"

    # Максимум одновременных запросов на инстанс
    max_instance_request_concurrency = 80

    containers {
      image = var.api_image

      # h2c — HTTP/2 cleartext (для gRPC)
      # Без port_name — HTTP/1.1
      ports {
        container_port = var.port
        name           = var.is_grpc ? "h2c" : null
      }

      resources {
        limits = {
          cpu    = var.cpu
          memory = var.memory
        }
        # cpu_idle = false — CPU всегда доступен (нужно для фоновых задач)
        # cpu_idle = true  — CPU только во время обработки запроса (дешевле)
        cpu_idle = var.environment != "prod"
      }

      # Обычные env vars
      dynamic "env" {
        for_each = var.env_vars
        content {
          name  = env.key
          value = env.value
        }
      }

      # Env vars из Secret Manager
      dynamic "env" {
        for_each = var.secret_env
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = env.value.secret
              version = try(env.value.version, "latest")
            }
          }
        }
      }

      # CloudSQL proxy через Unix socket
      dynamic "volume_mounts" {
        for_each = length(var.cloudsql_instances) > 0 ? [1] : []
        content {
          name       = "cloudsql"
          mount_path = "/cloudsql"
        }
      }
    }

    # CloudSQL volume
    dynamic "volumes" {
      for_each = length(var.cloudsql_instances) > 0 ? [1] : []
      content {
        name = "cloudsql"
        cloud_sql_instance {
          instances = var.cloudsql_instances
        }
      }
    }

    # VPC доступ для приватных ресурсов (Redis, внутренние сервисы)
    dynamic "vpc_access" {
      for_each = var.needs_vpc ? [1] : []
      content {
        egress = "PRIVATE_RANGES_ONLY"  # только приватные IP через VPC
        network_interfaces {
          network    = "default"
          subnetwork = "default"
        }
      }
    }
  }

  lifecycle {
    # CI/CD деплоит образы — Terraform не должен откатывать
    ignore_changes = ["template[0].containers[0].image"]
  }
}
```

### IAM для Cloud Run

```hcl
# Публичный доступ (unauthenticated)
resource "google_cloud_run_v2_service_iam_member" "public" {
  count    = var.public_access ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.api.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# Доступ для конкретного service account
resource "google_cloud_run_v2_service_iam_member" "internal_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.api.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.scheduler.email}"
}

# Доступ для Pub/Sub push subscription
resource "google_cloud_run_v2_service_iam_member" "pubsub_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.worker.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.pubsub_push.email}"
}
```

### Cloud Run Job (batch задачи)

```hcl
resource "google_cloud_run_v2_job" "migration" {
  name     = "db-migration"
  location = var.region
  project  = var.project_id

  template {
    template {
      service_account = google_service_account.migration.email

      containers {
        image = var.migration_image

        env {
          name  = "DB_CONNECTION"
          value = google_sql_database_instance.main.connection_name
        }

        volume_mounts {
          name       = "cloudsql"
          mount_path = "/cloudsql"
        }
      }

      volumes {
        name = "cloudsql"
        cloud_sql_instance {
          instances = [google_sql_database_instance.main.connection_name]
        }
      }

      # Retry при ошибке
      max_retries = 3
    }
  }
}
```

---

## CloudSQL PostgreSQL

```hcl
resource "google_sql_database_instance" "main" {
  name             = "${var.environment}-postgres"
  database_version = "POSTGRES_15"
  region           = var.region
  project          = var.project_id

  settings {
    tier = var.environment == "prod" ? "db-custom-2-7680" : "db-f1-micro"

    # Хранилище
    disk_autoresize       = true
    disk_autoresize_limit = var.environment == "prod" ? 200 : 50
    disk_size             = 20
    disk_type             = "PD_SSD"

    # IP конфигурация
    ip_configuration {
      # ipv4_enabled = false — только приватный IP
      # Нужен VPC peering или Cloud SQL Proxy
      ipv4_enabled    = true  # true = публичный IP (проще для начала)
      ssl_mode        = "ENCRYPTED_ONLY"

      # Разрешить только Cloud SQL Proxy (не прямые подключения)
      # Для Cloud Run — через Unix socket (/cloudsql/...), не через IP
    }

    backup_configuration {
      enabled                        = var.environment == "prod"
      start_time                     = "03:00"
      point_in_time_recovery_enabled = var.environment == "prod"
      transaction_log_retention_days = 7

      backup_retention_settings {
        retained_backups = 7
      }
    }

    maintenance_window {
      day          = 7  # воскресенье
      hour         = 3
      update_track = "stable"
    }

    # Флаги PostgreSQL
    database_flags {
      name  = "max_connections"
      value = "200"
    }
    database_flags {
      name  = "log_min_duration_statement"
      value = "1000"  # логировать запросы медленнее 1s
    }
  }

  deletion_protection = var.environment == "prod"

  lifecycle {
    prevent_destroy = true
  }
}

# База данных внутри инстанса
resource "google_sql_database" "app" {
  name     = "app"
  instance = google_sql_database_instance.main.name
  project  = var.project_id
}

# Пользователь
resource "google_sql_user" "app" {
  name     = "app"
  instance = google_sql_database_instance.main.name
  password = var.db_password  # брать из Secret Manager, не из переменной!
  project  = var.project_id
}

# Outputs
output "connection_name" {
  value = google_sql_database_instance.main.connection_name
  # Формат: "project:region:instance-name"
  # Используется в Cloud Run для CloudSQL proxy
}

output "public_ip" {
  value = google_sql_database_instance.main.public_ip_address
}
```

### CloudSQL read replica

```hcl
resource "google_sql_database_instance" "replica" {
  name                 = "${var.environment}-postgres-replica"
  database_version     = "POSTGRES_15"
  region               = var.region
  master_instance_name = google_sql_database_instance.main.name

  replica_configuration {
    failover_target = false
  }

  settings {
    tier              = "db-custom-1-3840"
    availability_type = "ZONAL"  # реплика не HA
    disk_autoresize   = true
  }
}
```

---

## IAM и Service Accounts

### Создание service account

```hcl
resource "google_service_account" "api" {
  account_id   = "${var.environment}-api"  # часть email до @
  display_name = "API Service Account (${var.environment})"
  project      = var.project_id
}

# Email: dev-api@my-project.iam.gserviceaccount.com
output "api_sa_email" {
  value = google_service_account.api.email
}
```

### Назначение ролей

```hcl
# Роль на проекте
resource "google_project_iam_member" "api_secret_accessor" {
  project = var.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.api.email}"
}

# Роль на конкретном ресурсе (предпочтительнее — принцип least privilege)
resource "google_secret_manager_secret_iam_member" "api_db_password" {
  secret_id = google_secret_manager_secret.db_password.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.api.email}"
}

resource "google_storage_bucket_iam_member" "api_uploads" {
  bucket = google_storage_bucket.uploads.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.api.email}"
}

# Создать несколько bindings одновременно
resource "google_project_iam_member" "api_roles" {
  for_each = toset([
    "roles/cloudsql.client",
    "roles/secretmanager.secretAccessor",
    "roles/run.invoker",
  ])

  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.api.email}"
}
```

### google_project_iam_member vs google_project_iam_binding

```hcl
# google_project_iam_member — добавляет ОДНУ binding
# Безопасен при параллельной работе нескольких Terraform конфигураций
resource "google_project_iam_member" "api" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.api.email}"
}

# google_project_iam_binding — ЗАМЕНЯЕТ все members для роли
# Опасен: если другой Terraform управляет той же ролью — они будут перезаписывать друг друга
resource "google_project_iam_binding" "cloudsql_clients" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  members = [
    "serviceAccount:${google_service_account.api.email}",
    "serviceAccount:${google_service_account.worker.email}",
  ]
}
```

**Правило:** используй `_member` если не уверен. `_binding` только когда нужно явно контролировать всех members.

---

## Secret Manager

```hcl
# Создать секрет (без значения — только метаданные)
resource "google_secret_manager_secret" "db_password" {
  secret_id = "${var.environment}-db-password"
  project   = var.project_id

  replication {
    auto {}  # автоматическая репликация
  }
}

# Значение секрета — обычно НЕ управляется Terraform
# Создаётся вручную или через CI/CD
# Причина: значение попадёт в state файл (plaintext)

# Если всё же нужно через Terraform:
resource "google_secret_manager_secret_version" "db_password" {
  secret      = google_secret_manager_secret.db_password.id
  secret_data = var.db_password  # sensitive variable

  lifecycle {
    ignore_changes = [secret_data]  # не менять при каждом apply
  }
}

# Дать сервису доступ к секрету
resource "google_secret_manager_secret_iam_member" "api_access" {
  secret_id = google_secret_manager_secret.db_password.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.api.email}"
  project   = var.project_id
}

# Использовать в Cloud Run
resource "google_cloud_run_v2_service" "api" {
  template {
    containers {
      env {
        name = "DB_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.db_password.secret_id
            version = "latest"  # или конкретная версия "3"
          }
        }
      }
    }
  }
}
```

---

## GCS Buckets

```hcl
resource "google_storage_bucket" "uploads" {
  name          = "${var.project_id}-${var.environment}-uploads"
  location      = var.region
  project       = var.project_id
  storage_class = "STANDARD"

  # Версионирование (полезно для важных данных)
  versioning {
    enabled = var.environment == "prod"
  }

  # Lifecycle: удалять старые версии и временные файлы
  lifecycle_rule {
    action {
      type = "Delete"
    }
    condition {
      age                   = 30   # дней
      num_newer_versions    = 3    # хранить 3 последних версии
      with_state            = "ARCHIVED"
    }
  }

  # Lifecycle: переместить в холодное хранилище
  lifecycle_rule {
    action {
      type          = "SetStorageClass"
      storage_class = "NEARLINE"
    }
    condition {
      age = 90  # через 90 дней
    }
  }

  # CORS для web uploads
  cors {
    origin          = ["https://my-app.com"]
    method          = ["GET", "PUT", "POST"]
    response_header = ["Content-Type", "x-goog-resumable"]
    max_age_seconds = 3600
  }

  # Запретить публичный доступ
  public_access_prevention = "enforced"

  force_destroy = var.environment != "prod"  # в prod нельзя удалить bucket с данными
}

# Публичное чтение (для статики/CDN)
resource "google_storage_bucket_iam_member" "public_read" {
  count  = var.public ? 1 : 0
  bucket = google_storage_bucket.uploads.name
  role   = "roles/storage.objectViewer"
  member = "allUsers"
}
```

---

## Pub/Sub

```hcl
# Topic
resource "google_pubsub_topic" "orders" {
  name    = "${var.environment}-orders"
  project = var.project_id

  message_retention_duration = "86400s"  # 24 часа

  labels = {
    environment = var.environment
  }
}

# Pull subscription (consumer сам читает)
resource "google_pubsub_subscription" "orders_worker" {
  name    = "${var.environment}-orders-worker"
  topic   = google_pubsub_topic.orders.id
  project = var.project_id

  ack_deadline_seconds       = 60
  message_retention_duration = "604800s"  # 7 дней
  retain_acked_messages      = false

  expiration_policy {
    ttl = ""  # не истекать
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.orders_dlq.id
    max_delivery_attempts = 5
  }
}

# Push subscription (Pub/Sub сам вызывает Cloud Run)
resource "google_pubsub_subscription" "orders_push" {
  name    = "${var.environment}-orders-push"
  topic   = google_pubsub_topic.orders.id
  project = var.project_id

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.worker.uri}/pubsub"

    oidc_token {
      service_account_email = google_service_account.pubsub_push.email
    }
  }

  ack_deadline_seconds = 30
}

# Dead letter topic
resource "google_pubsub_topic" "orders_dlq" {
  name    = "${var.environment}-orders-dlq"
  project = var.project_id
}

# IAM: разрешить сервису публиковать сообщения
resource "google_pubsub_topic_iam_member" "api_publisher" {
  topic   = google_pubsub_topic.orders.id
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.api.email}"
  project = var.project_id
}

# IAM: разрешить Pub/Sub создавать OIDC токены для push
resource "google_project_iam_member" "pubsub_token_creator" {
  project = var.project_id
  role    = "roles/iam.serviceAccountTokenCreator"
  member  = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}
```

---

## Artifact Registry

```hcl
resource "google_artifact_registry_repository" "docker" {
  repository_id = "my-services"
  format        = "DOCKER"
  location      = var.region
  project       = var.project_id
  description   = "Docker images for production services"

  cleanup_policies {
    id     = "keep-last-10"
    action = "KEEP"
    most_recent_versions {
      keep_count = 10
    }
  }

  cleanup_policies {
    id     = "delete-old"
    action = "DELETE"
    condition {
      older_than = "2592000s"  # 30 дней
      tag_state  = "UNTAGGED"
    }
  }
}

# Разрешить CI/CD пушить образы
resource "google_artifact_registry_repository_iam_member" "ci_writer" {
  repository = google_artifact_registry_repository.docker.name
  location   = var.region
  project    = var.project_id
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${google_service_account.ci_cd.email}"
}

# Разрешить Cloud Run читать образы
resource "google_artifact_registry_repository_iam_member" "run_reader" {
  repository = google_artifact_registry_repository.docker.name
  location   = var.region
  project    = var.project_id
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${google_service_account.api.email}"
}
```

---

## Управление API

```hcl
# Включить нужные GCP API (сервисы)
# Без этого ресурсы не создадутся
resource "google_project_service" "apis" {
  for_each = toset([
    "run.googleapis.com",              # Cloud Run
    "sqladmin.googleapis.com",          # CloudSQL
    "secretmanager.googleapis.com",     # Secret Manager
    "pubsub.googleapis.com",            # Pub/Sub
    "artifactregistry.googleapis.com",  # Artifact Registry
    "iam.googleapis.com",              # IAM
    "cloudresourcemanager.googleapis.com",
  ])

  project = var.project_id
  service = each.value

  # Не отключать API при terraform destroy
  disable_on_destroy = false
}

# Остальные ресурсы зависят от включённых API
resource "google_cloud_run_v2_service" "api" {
  depends_on = [google_project_service.apis]
  # ...
}
```
