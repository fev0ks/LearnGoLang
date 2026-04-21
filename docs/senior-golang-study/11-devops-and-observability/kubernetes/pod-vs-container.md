# Pod vs Container

Это вопрос, на котором часто сыпятся даже люди, которые "работали с Kubernetes".

## Содержание

- [Container: единица запуска](#container-единица-запуска)
- [Pod: единица оркестрации](#pod-единица-оркестрации)
- [Что контейнеры внутри Pod делят](#что-контейнеры-внутри-pod-делят)
- [Multi-container Pod: когда нужен](#multi-container-pod-когда-нужен)
- [Init containers](#init-containers)
- [Жизненный цикл Pod](#жизненный-цикл-pod)
- [Interview-ready answer](#interview-ready-answer)

## Container: единица запуска

Контейнер — runtime unit: изолированный процесс с собственным filesystem view, env vars и resource limits.

В Docker ты работаешь напрямую с контейнерами. В Kubernetes контейнер никогда не создается напрямую — только Pod, который их оборачивает.

## Pod: единица оркестрации

`Pod` — минимальная deployable единица в Kubernetes. Именно Pod, а не контейнер:

- получает IP-адрес в кластере;
- является единицей планирования (scheduler размещает Pod на Node);
- перезапускается и заменяется Kubernetes;
- умирает целиком — нельзя перезапустить один контейнер внутри Pod независимо.

Упрощенная spec Pod'а для Go-сервиса:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: api-server
spec:
  containers:
    - name: api
      image: myrepo/api-server:v1.2.3
      ports:
        - containerPort: 8080
      env:
        - name: DB_DSN
          valueFrom:
            secretKeyRef:
              name: api-secrets
              key: db_dsn
```

На практике Pod не создают напрямую — используют Deployment, который управляет Pod'ами через ReplicaSet.

## Что контейнеры внутри Pod делят

Контейнеры в одном Pod разделяют:

- **network namespace**: один IP-адрес, одни порты; контейнеры общаются по `localhost`;
- **UTS namespace**: одно hostname;
- **volumes**: Pod-level volume монтируется во все контейнеры, которым он нужен.

Они НЕ делят:
- filesystem (у каждого контейнера свой);
- process namespace (по умолчанию).

Именно из-за общего network namespace sidecar-прокси (Envoy, Linkerd) работает прозрачно: основной контейнер слушает порт, прокси перехватывает трафик через localhost без изменений в коде приложения.

## Multi-container Pod: когда нужен

Один Pod — один основной контейнер. Второй контейнер добавляется только если логика тесно связана с жизненным циклом основного.

**Sidecar pattern** — наиболее частый случай:

```yaml
spec:
  containers:
    - name: api
      image: myrepo/api-server:v1.2.3
    - name: log-forwarder
      image: fluent/fluent-bit:latest
      volumeMounts:
        - name: log-volume
          mountPath: /var/log/app
  volumes:
    - name: log-volume
      emptyDir: {}
```

Sidecar запускается и останавливается вместе с основным контейнером, имеет доступ к тем же volumes и сети.

**Ambassador / proxy**: Envoy-sidecar в service mesh (Istio, Linkerd) — перехватывает сетевой трафик к основному контейнеру и от него.

## Init containers

Init container — специальный контейнер, который запускается и завершается до запуска основных контейнеров. Следующий init container стартует только после успешного завершения предыдущего.

Типичное применение:
- проверить доступность зависимостей (DB, другой сервис);
- накатить database migration;
- сгенерировать или скопировать конфиг-файлы.

```yaml
spec:
  initContainers:
    - name: wait-for-db
      image: busybox
      command: ['sh', '-c', 'until nc -z postgres:5432; do sleep 2; done']
  containers:
    - name: api
      image: myrepo/api-server:v1.2.3
```

Если init container завершается с ошибкой, Pod не стартует, и Kubernetes перезапускает init container согласно `restartPolicy`.

## Жизненный цикл Pod

```text
Pending -> Running -> Succeeded / Failed
                  \-> (при ошибке) -> CrashLoopBackOff
```

- `Pending`: Pod принят, ждет размещения на Node или загрузки image.
- `Running`: хотя бы один контейнер запущен.
- `Succeeded`: все контейнеры завершились с кодом 0 (для Job/CronJob).
- `Failed`: контейнер завершился с ненулевым кодом.
- `CrashLoopBackOff`: контейнер многократно падает, Kubernetes увеличивает backoff между перезапусками.

`CrashLoopBackOff` — первое, что смотреть при `kubectl get pods`, если что-то не стартует.

## Interview-ready answer

Container — это runtime unit: изолированный процесс. Pod — это orchestration unit Kubernetes: он получает IP, является единицей планирования и заменяется целиком. Контейнеры в одном Pod делят network namespace, поэтому могут общаться через localhost — на этом работают sidecar-паттерны. Pod напрямую не создают: используют Deployment, который управляет Pod'ами через ReplicaSet. Если Pod умирает — Kubernetes пересоздает его по новому IP, и именно поэтому нужен Service как стабильная точка доступа.
