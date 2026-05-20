# Cloud (AWS)

Облачная инфраструктура — стандарт для backend в 2026. AWS остаётся доминирующим провайдером, но концепции переносятся на GCP/Azure.

## Материалы

- [01. AWS: основные сервисы](./01-aws-core-services.md) — EC2, VPC, IAM, S3, RDS, DynamoDB, SQS/SNS, Lambda, EKS/ECS, CloudWatch, Route 53, CloudFront, ELB/ALB/NLB, Secrets Manager. Managed vs self-hosted, AWS SDK в Go
- [02. Cloud cost и архитектурные решения](./02-cloud-cost-and-architecture.md) — pricing models, EC2 types, RI/Savings Plans, Spot, storage cost, network cost (NAT GW, egress), скрытые расходы, монитоинг, архитектурные решения для экономии, истории горьких уроков, чек-лист

## Что должен знать senior

**Архитектурно:**
- разница между AZ, region, edge location
- VPC топология (public/private subnets, NAT, IGW)
- Security Groups vs NACL
- IAM (users, roles, policies, least privilege)
- Managed vs self-hosted trade-offs

**Cost-aware:**
- порядок цен (EC2, RDS, S3, egress)
- Reserved Instances и Savings Plans
- Network cost — самое коварное (NAT GW, egress, cross-AZ)
- Как находить idle resources
- Когда Spot имеет смысл

**Sin't dunce:**
- никогда не embed AWS credentials в код (IAM roles!)
- enable billing alerts заранее
- tag everything
- lifecycle policies для storage

## Связанные разделы

- [Terraform](../terraform/) — IaC для AWS/GCP
- [Kubernetes](../kubernetes/) — на EKS работает тут
- [CI/CD](../ci-cd/) — deployment на AWS
- [Secrets management](../../12-security/secrets-management/) — AWS Secrets Manager
- [Hardware и OS](../hardware-and-os/) — что внутри EC2 instance
