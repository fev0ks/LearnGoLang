# kubectl: практическая шпаргалка

## Содержание

- [Безопасный контекст](#безопасный-контекст)
- [Просмотр ресурсов](#просмотр-ресурсов)
- [Диагностика Pod](#диагностика-pod)
- [Logs, exec и debug](#logs-exec-и-debug)
- [Deployment и rollout](#deployment-и-rollout)
- [Service и EndpointSlice](#service-и-endpointslice)
- [Resources и events](#resources-и-events)
- [Применение изменений](#применение-изменений)
- [Маршруты диагностики](#маршруты-диагностики)
- [Interview-ready answer](#interview-ready-answer)

`kubectl` — клиент Kubernetes API. Полезнее запомнить не сотни команд, а маршрут диагностики: проверить контекст, найти объект, изучить его состояние и события, затем перейти к логам или к сетевому пути.

Команды ниже предполагают доступ к кластеру. Учебный стенд, на котором их можно выполнить, описан в [приложении про локальный кластер](./00-local-cluster.md).

---

## Безопасный контекст

Контекст (`context`) — сохранённая в `kubeconfig` связка «кластер + пользователь + namespace по умолчанию». Большинство опасных ошибок `kubectl` — это правильная команда, выполненная в неправильном кластере или пространстве имён.

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

Явное указание `-n` надёжнее для инструкций и скриптов: результат не зависит от того, какое пространство имён выбрано по умолчанию на конкретной машине.

<details>
<summary>Пример вывода списка контекстов</summary>

```text
CURRENT   NAME       CLUSTER        AUTHINFO       NAMESPACE
          staging    staging        developer      payments
*         prod       prod-eu        developer      payments
          local      kind-local     kind-local     default
```

Звёздочка показывает активный контекст. Перед изменением рабочего окружения полезно ещё раз вывести `current-context` прямо в том же окне терминала.

</details>

---

## Просмотр ресурсов

Начать стоит с краткого состояния:

```bash
kubectl -n payments get deployment,replicaset,pod,service
kubectl -n payments get pods -o wide
kubectl -n payments get pods --show-labels
kubectl -n payments get pods -w
```

`kubectl get all` не означает буквально все типы ресурсов: `Secret`, `ConfigMap`, `Ingress` и большинство пользовательских типов туда не входят.

Получить полное представление объекта или отдельное поле:

```bash
kubectl -n payments get pod api-abc -o yaml
kubectl -n payments get pod api-abc -o jsonpath='{.status.containerStatuses[*].restartCount}'
kubectl explain deployment.spec.strategy.rollingUpdate
```

YAML живого объекта содержит его текущее состояние (`status`) и значения, подставленные сервером по умолчанию. Поэтому копировать такой вывод обратно в репозиторий как исходный манифест без очистки не следует.

---

## Диагностика Pod

Базовый цикл:

```bash
kubectl -n payments get pod api-abc -o wide
kubectl -n payments describe pod api-abc
kubectl -n payments logs api-abc -c api --tail=200
```

`describe` показывает:

- узел и условия готовности Pod (`conditions`);
- текущее и предыдущее состояние каждого контейнера;
- код завершения, причину и счётчик перезапусков;
- проверки состояния, requests/limits и подключённые тома;
- связанные события внизу вывода.

Колонка `STATUS` — удобная сводка, но она не всегда совпадает с фазой Pod. `CrashLoopBackOff` и `ImagePullBackOff` описывают состояние конкретного контейнера, а не всего Pod.

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

Здесь важнее не слово `CrashLoopBackOff`, а предыдущий `Exit Code`, логи завершившегося процесса и событие, подтверждающее нарастающую задержку между перезапусками.

</details>

| Наблюдение | Частая причина | Следующая проверка |
| --- | --- | --- |
| `Pending` | не хватает ресурсов узла, мешают правила размещения или PVC не привязан | `describe pod` → `FailedScheduling` |
| `ImagePullBackOff` | неверные имя или тег образа либо нет доступа к реестру | события и `imagePullSecrets` |
| `CrashLoopBackOff` | процесс завершается вскоре после запуска | `logs --previous`, предыдущее состояние, код завершения |
| `OOMKilled` | превышен memory limit контейнера или на узле закончилась память | предыдущее состояние, график памяти, limit и фактическое потребление |
| `0/1 Ready` | не проходит проверка готовности | сообщение probe, конечная точка, timeout |
| долго `Terminating` | не завершились процесс, `preStop`, finalizer или отключение тома | отметка времени удаления, события, владелец объекта и его finalizers |

Finalizers не следует снимать автоматически. Наличие finalizer означает, что контроллер ещё должен завершить обязательную очистку; ручное снятие способно оставить внешний ресурс или том в несогласованном состоянии.

---

## Logs, exec и debug

### Logs

```bash
kubectl -n payments logs api-abc -c api --tail=200
kubectl -n payments logs api-abc -c api --since=30m
kubectl -n payments logs api-abc -c api -f
kubectl -n payments logs api-abc -c api --previous
kubectl -n payments logs -l app=api --all-containers --prefix --tail=50
```

`--previous` особенно важен при цикле перезапусков: он показывает `stdout` и `stderr` предыдущего экземпляра контейнера, то есть того запуска, который упал. `kubectl logs` не заменяет централизованное хранение логов: после удаления Pod или ротации локального файла часть истории исчезает.

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

`exec` работает только если нужная утилита есть в самом образе. Отладочный контейнер (`ephemeral container`) выручает с образами без оболочки и утилит (`distroless`), но требует соответствующих прав и поддержки со стороны среды выполнения. Флаг `--target` не на каждой платформе гарантирует видимость процессов другого контейнера.

Временный сетевой клиент:

```bash
kubectl -n payments run curl-debug \
  --image=curlimages/curl:8.12.1 \
  --restart=Never --rm -it -- \
  curl -sv http://api:8080/readyz
```

В рабочем окружении для отладочного образа используют разрешённую политикой и зафиксированную ссылку: неизменяемый тег или digest.

### Port-forward

```bash
kubectl -n payments port-forward service/api 8080:80
kubectl -n payments port-forward pod/api-abc 8081:8080
```

По умолчанию туннель слушает только локальный адрес. `--address=0.0.0.0` открывает его другим машинам и позволяет обойти обычный путь через Ingress и аутентификацию, поэтому без явной защиты этот флаг опасен.

---

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

При GitOps-доставке такие изменения лучше делать в источнике истины, то есть в репозитории: иначе следующее согласование вернёт состояние из Git, а между описанием и кластером появится труднообъяснимое расхождение.

Если обновление зависло, проверяют новый `ReplicaSet` и его Pod, а не только `Deployment`:

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

При истечении времени ожидания команда возвращает ненулевой код завершения. Дальше полезно сравнить `ReplicaSet`:

```bash
kubectl -n payments get rs -l app=api
kubectl -n payments get pods -l app=api \
  -o custom-columns='NAME:.metadata.name,READY:.status.containerStatuses[*].ready,IMAGE:.spec.containers[*].image,NODE:.spec.nodeName'
```

</details>

---

## Service и EndpointSlice

Проверка маршрута от Service к Pod:

```bash
kubectl -n payments describe service api
kubectl -n payments get pods -l app=api --show-labels
kubectl -n payments get endpointslice \
  -l kubernetes.io/service-name=api -o wide
```

Объект `Endpoints` объявлен устаревшим начиная с Kubernetes 1.33, поэтому новые инструкции по диагностике пишут на `EndpointSlice`.

Если готовых конечных точек нет:

1. selector `Service` должен совпадать с метками Pod;
2. `targetPort` должен указывать на правильный номер или имя порта;
3. Pod должен быть `Ready`;
4. сетевые политики (`NetworkPolicy`) и само приложение должны разрешать трафик.

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

Обычный трафик `Service` направляется только в готовые конечные точки. У завершающейся точки `ready: false`, но поля `serving` и `terminating` позволяют клиентам, которые их учитывают, реализовать более аккуратный вывод Pod из-под нагрузки.

</details>

Чтобы разделить возможные причины, проверку ведут слоями:

- `port-forward pod/...` проверяет само приложение, минуя выбор Pod через `Service`;
- запрос к `Service` из отладочного Pod дополнительно проверяет DNS, `Service` и сеть кластера;
- внешний запрос добавляет к этому Ingress, Gateway и внешний балансировщик.

Слой, на котором запрос перестаёт проходить, и указывает на источник проблемы.

---

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

События объясняют решения платформы: размещение, загрузку образа, проверки состояния и операции с томами.

```bash
kubectl -n payments get events --sort-by='.metadata.creationTimestamp'
kubectl -n payments get events --field-selector type=Warning
kubectl -n payments get events \
  --field-selector involvedObject.name=api-abc
```

События хранятся ограниченное время и могут объединяться в одну запись со счётчиком. Для расследования, растянутого во времени, нужны централизованные логи, метрики и журнал аудита.

---

## Применение изменений

Безопасная последовательность при декларативном применении манифестов:

```bash
kubectl diff -f ./k8s/
kubectl apply --dry-run=server -f ./k8s/
kubectl apply -f ./k8s/
kubectl -n payments rollout status deployment/api --timeout=5m
```

- `--dry-run=client` проверяет только локальную генерацию манифеста;
- `--dry-run=server` дополнительно проходит проверки приёма и серверную валидацию, ничего не сохраняя;
- `diff` показывает ожидаемое изменение, но может требовать права на изменение объекта;
- `apply` завершается успешно до окончания обновления, поэтому его статус проверяют отдельной командой.

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

Такой diff показывает изменение шаблона Pod и предстоящее обновление, но не доказывает, что новый образ пройдёт проверки запуска и готовности или что Pod поместится на узел.

</details>

Команды `delete`, `scale`, `edit`, `patch` и `rollout undo` изменяют кластер. Перед ними проверяют контекст, пространство имён, объект-владелец и то, не описано ли это состояние в репозитории-источнике истины.

---

## Маршруты диагностики

### Pod перезапускается

```text
get pod
  → describe pod: состояние / предыдущее состояние / код завершения / события
  → logs --previous
  → метрики и логи зависимостей
```

### Обновление не завершается

```text
rollout status
  → новый ReplicaSet
  → его Pod: Pending, загрузка образа, запуск, готовность
  → есть ли ресурсы под maxSurge и не истёк ли progressDeadline
  → исправить манифест или откатить
```

### Service не отвечает

```text
selector и targetPort Service
  → метки и готовность Pod
  → состояние конечных точек EndpointSlice
  → запрос из отладочного Pod
  → NetworkPolicy / Gateway / внешний балансировщик
```

---

## Interview-ready answer

**1. Как диагностировать Pod в `CrashLoopBackOff`?**

Смотрю `describe pod`, чтобы увидеть предыдущее состояние контейнера, код завершения и события, затем `logs --previous`. После этого проверяю конфигурацию, проверки состояния, memory limit и зависимости. `CrashLoopBackOff` — не отдельная фаза Pod, а нарастающая задержка между перезапусками контейнера.

**2. Чем отличаются `get`, `describe`, `logs` и события?**

`get` даёт краткое или структурированное состояние объекта, `describe` объединяет ключевые поля и связанные события, `logs` показывает `stdout` и `stderr` контейнера, а события объясняют решения платформы: размещение, загрузку образа, проверку состояния или монтирование тома.

**3. Как диагностировать пустой Service?**

Сверяю selector `Service` с метками Pod, затем проверяю готовность Pod и содержимое `EndpointSlice`. Если конечные точки есть, проверяю `targetPort`, запрос изнутри кластера и сетевые политики. Прямой `port-forward` к Pod помогает отделить проблему приложения от проблемы на пути через `Service`.

---

## Официальные источники

- [kubectl quick reference](https://kubernetes.io/docs/reference/kubectl/quick-reference/)
- [Debug Pods](https://kubernetes.io/docs/tasks/debug/debug-application/debug-pods/)
- [Debug Services](https://kubernetes.io/docs/tasks/debug/debug-application/debug-service/)
- [EndpointSlices](https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/)
