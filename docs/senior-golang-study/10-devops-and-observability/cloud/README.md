# Cloud: AWS и Google Cloud

Раздел объясняет облачную инфраструктуру со стороны backend-разработчика: какой
сервис решает задачу, как приложение получает доступ и какие operational
trade-offs возникают. AWS и Google Cloud разобраны отдельно, чтобы похожие роли
сервисов не скрывали различия в IAM, networking и deployment model.

---

## Материалы

- [01. AWS: практический обзор](./01-aws-core-services.md) —
  выбор compute, storage и messaging, IAM, VPC, Go, delivery и observability
- [02. Стоимость AWS](./02-cloud-cost-and-architecture.md) — Cost Explorer,
  CUR 2.0, right-sizing, Savings Plans/RI, Spot, Budgets и проверяемые расчёты
- [03. Google Cloud: практический обзор](./03-gcp-core-services.md) — Cloud Run,
  GKE, Compute Engine, data services, Pub/Sub, IAM, сеть и observability

---

## Что должен знать senior

**Архитектурно:**

- разница между zone, region и edge location;
- как scope VPC и subnet отличается между AWS и GCP;
- как выбрать compute по workload, а не по привычному названию продукта;
- как IAM связывает identity, role и resource;
- managed vs self-hosted trade-offs.

**Cost-aware:**

- различать стоимость выделенной capacity и pay-per-use;
- считать egress, cross-zone и cross-region traffic;
- находить idle resources и ограничивать autoscaling;
- понимать, когда commitment и Spot действительно подходят;
- знать, что budget alert не является жёстким лимитом расходов.

**Практически:**

- не хранить cloud credentials в коде и repository;
- использовать workload identity: IAM role в AWS, service account и ADC в GCP;
- включать billing alerts до production traffic;
- задавать tags/labels и lifecycle policies для storage;
- проверять backups, restore path, dashboards и alerts до запуска.

---

## Связанные разделы

- [Terraform](../terraform/) — IaC для AWS/GCP
- [Kubernetes](../kubernetes/) — основа EKS и GKE
- [CI/CD](../ci-cd/) — deployment в AWS и GCP
- [Secrets management](../../11-security/secrets-management/) — AWS Secrets
  Manager и Google Secret Manager
- [Hardware и OS](../hardware-and-os/) — что скрывают VM и container runtimes
