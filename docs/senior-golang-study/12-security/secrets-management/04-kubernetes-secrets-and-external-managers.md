# Kubernetes Secrets и External Managers

## Содержание

- [Kubernetes Secret: базовый минимум](#kubernetes-secret-базовый-минимум)
- [Ограничения нативного Secret](#ограничения-нативного-secret)
- [External Secrets Operator](#external-secrets-operator)
- [Sealed Secrets и SOPS для GitOps](#sealed-secrets-и-sops-для-gitops)
- [Vault](#vault)
- [Практическая стратегия](#практическая-стратегия)

---

## Kubernetes Secret: базовый минимум

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: app-secrets
  namespace: production
type: Opaque
stringData:                          # base64 кодирует автоматически
  DATABASE_URL: "postgres://user:pass@db:5432/app"
  JWT_SECRET: "my-signing-key"
```

**Инжект как env vars:**

```yaml
containers:
  - name: app
    envFrom:
      - secretRef:
          name: app-secrets          # все ключи → env vars
    # или отдельные ключи:
    env:
      - name: DATABASE_URL
        valueFrom:
          secretKeyRef:
            name: app-secrets
            key: DATABASE_URL
```

**Инжект как файлы (предпочтительно для TLS, крупных секретов):**

```yaml
containers:
  - name: app
    volumeMounts:
      - name: secrets-vol
        mountPath: /run/secrets
        readOnly: true
    env:
      - name: TLS_CERT_FILE
        value: /run/secrets/tls.crt
volumes:
  - name: secrets-vol
    secret:
      secretName: app-tls-secrets
      items:
        - key: tls.crt
          path: tls.crt
        - key: tls.key
          path: tls.key
```

---

## Ограничения нативного Secret

- **Plaintext в etcd** — без дополнительной настройки (`EncryptionConfiguration`) секреты в etcd не зашифрованы
- **Нет аудита доступа** — стандартный Kubernetes audit log не отслеживает кто читал Secret
- **Нет ротации** — изменение Secret не перезапускает Pod автоматически (нужен `reloader`)
- **Нет GitOps** — хранить plaintext YAML с секретами в git нельзя

---

## External Secrets Operator

[external-secrets.io](https://external-secrets.io) — CRD, который синхронизирует секреты из внешнего хранилища в Kubernetes Secret:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: app-secrets
spec:
  refreshInterval: 1h              # как часто синхронизировать
  secretStoreRef:
    name: gcp-secret-manager       # источник
    kind: ClusterSecretStore
  target:
    name: app-secrets              # создать/обновить этот Secret
  data:
    - secretKey: DATABASE_URL
      remoteRef:
        key: prod-database-url     # имя в Google Secret Manager
    - secretKey: JWT_SECRET
      remoteRef:
        key: prod-jwt-secret
```

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ClusterSecretStore
metadata:
  name: gcp-secret-manager
spec:
  provider:
    gcpsm:
      projectID: my-gcp-project
      auth:
        workloadIdentity:
          clusterLocation: europe-west1
          clusterName: my-cluster
          serviceAccountRef:
            name: external-secrets-sa
            namespace: external-secrets
```

**Плюсы:** централизованное хранение, аудит в Secret Manager, ротация через обновление секрета в SM.

---

## Sealed Secrets и SOPS для GitOps

Когда нужно хранить секреты в git:

**Sealed Secrets** ([bitnami-labs/sealed-secrets](https://github.com/bitnami-labs/sealed-secrets)):

```bash
# Зашифровать
kubectl create secret generic app-secrets \
  --from-literal=JWT_SECRET=my-secret \
  --dry-run=client -o yaml | \
  kubeseal --controller-name=sealed-secrets > sealed-secret.yaml
# sealed-secret.yaml коммитится в git — расшифровать может только кластер
```

```yaml
# sealed-secret.yaml (безопасно коммитить)
apiVersion: bitnami.com/v1alpha1
kind: SealedSecret
metadata:
  name: app-secrets
spec:
  encryptedData:
    JWT_SECRET: AgBy3i4OJSWK+PiTySYZZA9rO...
```

**SOPS** ([mozilla/sops](https://github.com/mozilla/sops)) — шифрует YAML/JSON через KMS/PGP/age:

```bash
sops --encrypt --gcp-kms projects/my-project/locations/global/keyRings/my-ring/cryptoKeys/my-key \
  secrets.yaml > secrets.enc.yaml
# secrets.enc.yaml → в git
# secrets.yaml → в .gitignore
```

---

## Vault

[HashiCorp Vault](https://www.vaultproject.io/) — полноценный secret management platform:

**Dynamic secrets** — Vault генерирует временные credentials специально для конкретного Pod:

```go
import vault "github.com/hashicorp/vault/api"

func loadFromVault(addr, token, path string) (map[string]string, error) {
    cfg := vault.DefaultConfig()
    cfg.Address = addr

    client, err := vault.NewClient(cfg)
    if err != nil {
        return nil, err
    }
    client.SetToken(token)

    secret, err := client.Logical().Read(path)
    if err != nil {
        return nil, fmt.Errorf("vault read %q: %w", path, err)
    }
    if secret == nil {
        return nil, fmt.Errorf("vault: no secret at %q", path)
    }

    result := make(map[string]string)
    for k, v := range secret.Data {
        if s, ok := v.(string); ok {
            result[k] = s
        }
    }
    return result, nil
}
```

В Kubernetes — Vault Agent Sidecar или Vault Secrets Operator инжектят секреты как файлы без изменений в Go-коде.

---

## Практическая стратегия

```
Маленький кластер, нет GitOps:
  Kubernetes Secret + RBAC + etcd encryption at rest

GitOps (ArgoCD/Flux):
  Sealed Secrets или SOPS — секреты шифруются и хранятся в git

Несколько кластеров или облако:
  External Secrets Operator → Google Secret Manager / AWS Secrets Manager

Высокие требования (финтех, HIPAA):
  HashiCorp Vault — dynamic secrets, audit, short-lived credentials
```

**Независимо от варианта:**
- etcd encryption at rest включить (`EncryptionConfiguration`)
- RBAC: ограничить `get secret` только нужным ServiceAccount
- `reloader` ([stakater/Reloader](https://github.com/stakater/Reloader)) — автоперезапуск Pod при изменении Secret
