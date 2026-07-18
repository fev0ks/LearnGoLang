# kubectl: практическая шпаргалка

`kubectl` — клиент Kubernetes API. Полезнее запомнить не сотни команд, а маршрут диагностики: проверить контекст, найти объект, изучить его состояние и events, затем перейти к logs или сетевому пути.

## Содержание

- [Безопасный контекст](#безопасный-контекст)
- [Просмотр ресурсов](#просмотр-ресурсов)
- [Диагностика Pod](#диагностика-pod)
- [Logs, exec и debug](#logs-exec-и-debug)
- [Deployment и rollout](#deployment-и-rollout)
- [Service и EndpointSlice](#service-и-endpointslice)
- [Resources и events](#resources-и-events)
- [Применение изменений](#применение-изменений)
- [Troubleshooting workflows](#troubleshooting-workflows)
- [Interview-ready answer](#interview-ready-answer)

## Безопасный контекст

Большинство опасных ошибок `kubectl` — правильная команда в неправильном кластере или namespace.

```bash
kubectl config current-context
kubectl config get-contexts
kubectl config view --minify
```

Для production-команд полезно указывать контекст и namespace явно:

```bash
kubectl --context=prod -n payments get pods
kubectl --context=prod -n payments auth can-i delete pods
```

Переключить сохранённый контекст можно так:

```bash
kubectl config use-context staging
kubectl config set-context --current --namespace=payments
```

Явный `-n` лучше для runbook и скриптов: результат не зависит от локального default namespace.

<details>
<summary>Пример вывода списка контекстов</summary>

```text
CURRENT   NAME       CLUSTER        AUTHINFO       NAMESPACE
          staging    staging        developer      payments
*         prod       prod-eu        developer      payments
          local      kind-local     kind-local     default
```

Звёздочка показывает активный context. Перед изменением production полезно ещё раз вывести `current-context` прямо в том же terminal session.

</details>

## Просмотр ресурсов

Начать стоит с краткого состояния:

```bash
kubectl -n payments get deployment,replicaset,pod,service
kubectl -n payments get pods -o wide
kubectl -n payments get pods --show-labels
kubectl -n payments get pods -w
```

`kubectl get all` не означает буквально все типы ресурсов: Secret, ConfigMap, Ingress и многие CRD туда не входят.

Получить полное представление объекта или отдельное поле:

```bash
kubectl -n payments get pod api-abc -o yaml
kubectl -n payments get pod api-abc -o jsonpath='{.status.containerStatuses[*].restartCount}'
kubectl explain deployment.spec.strategy.rollingUpdate
```

YAML живого объекта содержит status и server defaults, поэтому его не следует без очистки копировать обратно как исходный manifest.

## Диагностика Pod

Базовый цикл:

```bash
kubectl -n payments get pod api-abc -o wide
kubectl -n payments describe pod api-abc
kubectl -n payments logs api-abc -c api --tail=200
```

`describe` показывает:

- Node и Pod conditions;
- state и last state каждого container;
- exit code, reason и restart count;
- probes, requests/limits и mounts;
- связанные events внизу вывода.

Колонка `STATUS` — удобная сводка, но не всегда Pod phase. `CrashLoopBackOff` и `ImagePullBackOff` обычно описывают состояние container.

<details>
<summary>Пример фрагмента kubectl describe pod</summary>

```text
Containers:
  api:
    Image:          registry.example/api:v1.2.3
    State:          Waiting
      Reason:       CrashLoopBackOff
    Last State:     Terminated
      Reason:       Error
      Exit Code:    1
    Restart Count:  7
    Readiness:      http-get http://:8080/readyz
Events:
  Type     Reason   Message
  Warning  BackOff  Back-off restarting failed container api
```

Здесь важнее не слово `CrashLoopBackOff`, а предыдущий `Exit Code`, logs завершившегося процесса и событие, подтверждающее restart backoff.

</details>

| Наблюдение | Частая причина | Следующая проверка |
| --- | --- | --- |
| `Pending` | нет capacity, affinity/taint, unbound PVC | `describe pod` → `FailedScheduling` |
| `ImagePullBackOff` | неверный image/tag или registry credentials | events и `imagePullSecrets` |
| `CrashLoopBackOff` | процесс завершается после запуска | `logs --previous`, last state, exit code |
| `OOMKilled` | cgroup memory limit или node OOM | last state, memory graph, limit и working set |
| `0/1 Ready` | readiness не проходит | probe message, endpoint, timeout |
| долго `Terminating` | process/preStop/finalizer/volume не завершились | deletion timestamp, events, owner и finalizers |

Не удаляйте finalizers автоматически. Finalizer означает, что контроллер ещё должен завершить cleanup; ручное снятие может оставить внешний ресурс или storage в неконсистентном состоянии.

## Logs, exec и debug

### Logs

```bash
kubectl -n payments logs api-abc -c api --tail=200
kubectl -n payments logs api-abc -c api --since=30m
kubectl -n payments logs api-abc -c api -f
kubectl -n payments logs api-abc -c api --previous
kubectl -n payments logs -l app=api --all-containers --prefix --tail=50
```

`--previous` особенно важен при restart loop: он показывает stdout/stderr предыдущего экземпляра container. `kubectl logs` не заменяет централизованное хранение — после удаления Pod или ротации локального файла часть истории может исчезнуть.

<details>
<summary>Пример поиска причины предыдущего restart</summary>

```bash
pod=api-7d8d6fcb9b-2px8m

kubectl -n payments logs "$pod" -c api --previous --tail=100
kubectl -n payments get pod "$pod" \
  -o jsonpath='{.status.containerStatuses[?(@.name=="api")].lastState.terminated.reason}{"\n"}{.status.containerStatuses[?(@.name=="api")].lastState.terminated.exitCode}{"\n"}'
```

```text
level=error msg="configuration invalid" field=DATABASE_URL
Error
1
```

</details>

### Exec и ephemeral container

```bash
kubectl -n payments exec api-abc -c api -- printenv APP_ENV
kubectl -n payments exec -it api-abc -c api -- /bin/sh

kubectl -n payments debug -it api-abc \
  --image=busybox:1.36.1 \
  --target=api
```

`exec` работает только если нужная утилита есть в image. Ephemeral container полезен для distroless image, но требует RBAC и поддержки runtime; `--target` не на каждой платформе гарантирует видимость процессов другого container.

Временный сетевой клиент:

```bash
kubectl -n payments run curl-debug \
  --image=curlimages/curl:8.12.1 \
  --restart=Never --rm -it -- \
  curl -sv http://api:8080/readyz
```

В production используйте разрешённый и закреплённый digest/tag debug image.

### Port-forward

```bash
kubectl -n payments port-forward service/api 8080:80
kubectl -n payments port-forward pod/api-abc 8081:8080
```

По умолчанию listener доступен только локально. `--address=0.0.0.0` открывает туннель другим машинам и может обойти обычный ingress/auth path — использовать его без явной защиты опасно.

## Deployment и rollout

```bash
kubectl -n payments get deployment api
kubectl -n payments describe deployment api
kubectl -n payments get replicaset -l app=api
kubectl -n payments rollout status deployment/api --timeout=5m
kubectl -n payments rollout history deployment/api
```

Откатить Pod template:

```bash
kubectl -n payments rollout undo deployment/api
kubectl -n payments rollout undo deployment/api --to-revision=3
```

Для разовой операции доступны:

```bash
kubectl -n payments set image deployment/api api=registry.example/api:v1.2.4
kubectl -n payments rollout restart deployment/api
```

В GitOps/CD такие изменения лучше делать в source of truth: иначе следующий reconciliation вернёт состояние из Git, а live cluster получит труднообъяснимый drift.

Если rollout завис, проверяют новый ReplicaSet и его Pod, а не только Deployment:

```bash
kubectl -n payments get replicaset
kubectl -n payments get pods -l app=api
kubectl -n payments describe deployment api
```

<details>
<summary>Пример нормального и зависшего rollout</summary>

```text
$ kubectl -n payments rollout status deployment/api --timeout=5m
Waiting for deployment "api" rollout to finish: 2 of 3 updated replicas are available...
deployment "api" successfully rolled out
```

При timeout команда возвращает ненулевой exit code. Дальше полезно сравнить ReplicaSet:

```bash
kubectl -n payments get rs -l app=api
kubectl -n payments get pods -l app=api \
  -o custom-columns='NAME:.metadata.name,READY:.status.containerStatuses[*].ready,IMAGE:.spec.containers[*].image,NODE:.spec.nodeName'
```

</details>

## Service и EndpointSlice

Проверка маршрута от Service к Pod:

```bash
kubectl -n payments describe service api
kubectl -n payments get pods -l app=api --show-labels
kubectl -n payments get endpointslice \
  -l kubernetes.io/service-name=api -o wide
```

`Endpoints` API deprecated начиная с Kubernetes 1.33; для новых runbook следует использовать `EndpointSlice`.

Если ready endpoints отсутствуют:

1. selector Service должен совпадать с labels Pod;
2. `targetPort` должен указывать на правильный port/name;
3. Pod должен быть Ready;
4. NetworkPolicy и приложение должны разрешать трафик.

<details>
<summary>Пример EndpointSlice с ready и terminating endpoints</summary>

```yaml
apiVersion: discovery.k8s.io/v1
kind: EndpointSlice
metadata:
  labels:
    kubernetes.io/service-name: api
addressType: IPv4
ports:
  - name: http
    port: 8080
endpoints:
  - addresses: ["10.42.1.17"]
    conditions:
      ready: true
      serving: true
      terminating: false
  - addresses: ["10.42.2.21"]
    conditions:
      ready: false
      serving: true
      terminating: true
```

Обычный Service traffic использует ready endpoint. У terminating endpoint `ready: false`; поля `serving` и `terminating` позволяют aware-клиентам реализовать более сложный draining.

</details>

Для разделения проблем:

- `port-forward pod/...` проверяет приложение, обходя Service selection;
- запрос к Service из debug Pod проверяет DNS, Service и cluster networking;
- внешний запрос дополнительно включает Ingress/Gateway/load balancer.

## Resources и events

`kubectl top` требует Metrics API, обычно предоставляемый Metrics Server:

```bash
kubectl -n payments top pods --containers
kubectl top nodes
```

Requests и limits:

```bash
kubectl -n payments get pod api-abc \
  -o custom-columns='NAME:.metadata.name,CPU_REQ:.spec.containers[*].resources.requests.cpu,CPU_LIMIT:.spec.containers[*].resources.limits.cpu,MEM_REQ:.spec.containers[*].resources.requests.memory,MEM_LIMIT:.spec.containers[*].resources.limits.memory'
```

Events полезны для scheduling, image pull, probes и volume operations:

```bash
kubectl -n payments get events --sort-by='.metadata.creationTimestamp'
kubectl -n payments get events --field-selector type=Warning
kubectl -n payments get events \
  --field-selector involvedObject.name=api-abc
```

Events имеют ограниченный срок хранения и могут агрегироваться. Для долгого расследования нужны централизованные logs, metrics и audit trail.

## Применение изменений

Безопасная последовательность для declarative manifests:

```bash
kubectl diff -f ./k8s/
kubectl apply --dry-run=server -f ./k8s/
kubectl apply -f ./k8s/
kubectl -n payments rollout status deployment/api --timeout=5m
```

- `--dry-run=client` проверяет локальную генерацию;
- `--dry-run=server` проходит admission и server-side validation без сохранения;
- `diff` показывает ожидаемое изменение, но может требовать права patch;
- `apply` успешен до завершения rollout, поэтому статус проверяют отдельно.

<details>
<summary>Пример diff перед изменением image и resources</summary>

```diff
 spec:
   template:
     spec:
       containers:
         - name: api
-          image: registry.example/api:v1.2.3
+          image: registry.example/api:v1.2.4
           resources:
             requests:
-              memory: 128Mi
+              memory: 256Mi
```

Такой diff показывает изменение Pod template и ожидаемый rollout, но не доказывает, что новый image пройдёт startup/readiness или поместится на Node.

</details>

Команды `delete`, `scale`, `edit`, `patch` и `rollout undo` изменяют кластер. Перед ними проверяют context, namespace, owner resource и source of truth.

## Troubleshooting workflows

### Pod перезапускается

```text
get pod
  → describe pod: state / last state / exit code / events
  → logs --previous
  → metrics и dependency logs
```

### Rollout не завершается

```text
rollout status
  → новый ReplicaSet
  → его Pod: Pending, image pull, startup, readiness
  → capacity для maxSurge и progressDeadline
  → исправить manifest или rollback
```

### Service не отвечает

```text
Service selector и targetPort
  → Pod labels и readiness
  → EndpointSlice conditions
  → запрос из debug Pod
  → NetworkPolicy / Gateway / external LB
```

## Interview-ready answer

**1. Как диагностировать Pod в `CrashLoopBackOff`?**

Смотрю `describe pod`, чтобы увидеть container last state, exit code и events, затем `logs --previous`. После этого проверяю config, probes, memory limit и зависимости. `CrashLoopBackOff` — не отдельная фаза Pod, а backoff между рестартами container.

**2. Чем отличаются `get`, `describe`, `logs` и `events`?**

`get` даёт краткое или структурированное состояние объекта, `describe` объединяет ключевые поля и связанные events, `logs` показывает stdout/stderr container, а events объясняют решения платформы: scheduling, pull, probe или mount.

**3. Как диагностировать пустой Service?**

Сверяю selector Service с labels Pod, затем readiness и EndpointSlice. Если endpoint есть, проверяю `targetPort`, запрос изнутри кластера и NetworkPolicy. Прямой port-forward к Pod помогает отделить приложение от Service path.

## Официальные источники

- [kubectl quick reference](https://kubernetes.io/docs/reference/kubectl/quick-reference/)
- [Debug Pods](https://kubernetes.io/docs/tasks/debug/debug-application/debug-pods/)
- [Debug Services](https://kubernetes.io/docs/tasks/debug/debug-application/debug-service/)
- [EndpointSlices](https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/)
