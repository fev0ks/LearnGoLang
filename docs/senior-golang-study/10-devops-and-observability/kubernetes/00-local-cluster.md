# Локальный кластер для практики

## Содержание

- [Зачем нужен локальный кластер](#зачем-нужен-локальный-кластер)
- [Что установить](#что-установить)
- [Создать кластер одной командой](#создать-кластер-одной-командой)
- [Первая проверка](#первая-проверка)
- [Кластер из нескольких узлов](#кластер-из-нескольких-узлов)
- [Запустить первое приложение](#запустить-первое-приложение)
- [Добавить Ingress controller](#добавить-ingress-controller)
- [Что можно потрогать по каждому материалу раздела](#что-можно-потрогать-по-каждому-материалу-раздела)
- [Чем локальный кластер отличается от рабочего](#чем-локальный-кластер-отличается-от-рабочего)
- [Удалить кластер](#удалить-кластер)

Материалы раздела содержат команды `kubectl` и манифесты, но выполнять их негде, пока нет кластера. Этот файл даёт минимальный учебный стенд на одной машине.

Читать его целиком до остальных материалов не обязательно. Достаточно вернуться сюда перед [04 Pod и контейнер](./04-pod-vs-container.md), где начинается практическая диагностика.

---

## Зачем нужен локальный кластер

Чтение про `Pending`, `CrashLoopBackOff` и пустой `EndpointSlice` даёт узнавание, но не навык. Разница появляется, когда состояние воспроизводится намеренно: запросить у Pod больше CPU, чем есть на узле, сломать readiness probe, удалить Pod с подключённым PVC.

Локальный кластер позволяет делать это без риска и без доступа к рабочему окружению.

---

## Что установить

| Инструмент | Зачем | Проверка |
| --- | --- | --- |
| Docker или другая совместимая среда выполнения контейнеров | kind запускает узлы кластера как контейнеры | `docker version` |
| kind | создаёт кластер Kubernetes внутри этих контейнеров | `kind version` |
| `kubectl` | клиент Kubernetes API | `kubectl version --client` |

Установка на macOS через Homebrew:

```bash
brew install kind kubectl
```

Альтернативы kind — minikube, k3d и Docker Desktop со встроенным Kubernetes. Команды ниже используют kind, потому что он не требует отдельной виртуальной машины и быстро создаёт кластер из нескольких узлов.

---

## Создать кластер одной командой

```bash
kind create cluster --name study
```

Команда создаёт кластер из одного узла, где плоскость управления и рабочая нагрузка живут на одной машине, и добавляет в `kubeconfig` контекст `kind-study`.

Сразу после создания полезно убедиться, что `kubectl` смотрит именно туда:

```bash
kubectl config current-context
```

Ожидаемый вывод:

```text
kind-study
```

Привычка проверять контекст перед каждой изменяющей командой разобрана в [шпаргалке kubectl](./05-kubectl-commands.md). Формировать её удобнее с самого начала.

---

## Первая проверка

```bash
kubectl cluster-info
kubectl get nodes -o wide
kubectl -n kube-system get pods
```

В выводе `kube-system` видны системные компоненты, разобранные в [материале про архитектуру](./01-kubernetes-architecture.md): `kube-apiserver`, `etcd`, `kube-scheduler`, `kube-controller-manager`, CoreDNS и сетевой плагин. В управляемом облачном кластере часть из них не отображается, потому что провайдер запускает их за недоступной пользователю границей.

Это удобный момент, чтобы связать таблицу компонентов с реальным выводом.

---

## Кластер из нескольких узлов

Одноузловой кластер не позволяет наблюдать размещение Pod по машинам, отказ узла и правила распределения. Для этих сценариев нужен кластер из нескольких узлов.

Файл `kind-study.yaml`:

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
  - role: worker
  - role: worker
```

```bash
kind delete cluster --name study
kind create cluster --name study --config kind-study.yaml
kubectl get nodes
```

Теперь у кластера три рабочих узла, и становится осмысленной практика из [материала про прерывания](./08-node-failure-and-disruptions.md): topology spread constraints, `drain`, `PodDisruptionBudget`.

---

## Запустить первое приложение

```bash
kubectl create namespace learning

kubectl -n learning create deployment web \
  --image=registry.k8s.io/echoserver:1.10 \
  --replicas=3

kubectl -n learning expose deployment web \
  --port=80 --target-port=8080

kubectl -n learning get deployment,replicaset,pod,service -o wide
```

Три команды создают цепочку `Deployment → ReplicaSet → Pod` и `Service` перед ней. Ровно эта цепочка разобрана в [карте основных объектов](./03-core-objects-and-deployment-flow.md).

Проверить, что `Service` действительно нашёл Pod:

```bash
kubectl -n learning get endpointslice -l kubernetes.io/service-name=web
```

Обратиться к приложению, не настраивая Ingress:

```bash
kubectl -n learning port-forward service/web 8080:80
```

В другом терминале:

```bash
curl -s http://localhost:8080 | head -n 5
```

Команды `create deployment` и `expose` удобны для учебного стенда, но в рабочем процессе желаемое состояние описывают манифестами и применяют через `kubectl apply`. Причина разобрана в [шпаргалке kubectl](./05-kubectl-commands.md).

---

## Добавить Ingress controller

По умолчанию в kind контроллера входящего трафика нет, поэтому объекты `Ingress` создаются, но не работают. Этот раздел нужен для практики по [материалу про сеть](./12-networking-service-dns-and-ingress.md).

Кластер нужно создать заново, пробросив порты `80` и `443` с машины на узел плоскости управления. Файл `kind-ingress.yaml`:

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    kubeadmConfigPatches:
      - |
        kind: InitConfiguration
        nodeRegistration:
          kubeletExtraArgs:
            node-labels: "ingress-ready=true"
    extraPortMappings:
      - containerPort: 80
        hostPort: 80
        protocol: TCP
      - containerPort: 443
        hostPort: 443
        protocol: TCP
  - role: worker
  - role: worker
```

```bash
kind delete cluster --name study
kind create cluster --name study --config kind-ingress.yaml

kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml

kubectl -n ingress-nginx wait \
  --for=condition=Ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=120s
```

Метка `ingress-ready=true` нужна потому, что манифест ingress-nginx для kind размещает контроллер именно на помеченном узле. Проброс портов даёт возможность обращаться к нему с локальной машины напрямую.

После установки появляется класс контроллера:

```bash
kubectl get ingressclass
```

Теперь приложение из предыдущего раздела можно опубликовать:

```bash
kubectl -n learning create ingress web \
  --class=nginx \
  --rule="localhost/*=web:80"

curl -s http://localhost/ | head -n 5
```

Полезное упражнение — сломать связь намеренно: указать в правиле несуществующий `Service` и посмотреть, каким кодом ответит контроллер. Различие ответов 404 и 503 разобрано в [материале про сеть](./12-networking-service-dns-and-ingress.md).

---

## Что можно потрогать по каждому материалу раздела

| Материал | Что воспроизвести локально |
| --- | --- |
| [01 архитектура](./01-kubernetes-architecture.md) | найти системные компоненты в `kube-system`, сравнить с таблицей ролей |
| [02 кластер и HA](./02-kubernetes-cluster-and-ha.md) | создать кластер из нескольких узлов и посмотреть `kubectl get nodes -o wide` |
| [03 основные объекты](./03-core-objects-and-deployment-flow.md) | удалить один Pod и увидеть, что `ReplicaSet` создаёт новый с другим именем |
| [04 Pod и контейнер](./04-pod-vs-container.md) | запросить `requests.cpu: "8"` и получить `Pending` с событием `FailedScheduling` |
| [05 kubectl](./05-kubectl-commands.md) | сломать команду запуска контейнера и пройти путь `describe → logs --previous` |
| [06 Helm](./06-helm.md) | собрать учебный chart и сравнить `helm template` с тем, что попало в кластер |
| [07 probes и graceful shutdown](./07-probes-and-graceful-shutdown.md) | указать в readiness probe несуществующий путь и увидеть, как endpoint уходит из `EndpointSlice` |
| [08 прерывания](./08-node-failure-and-disruptions.md) | выполнить `kubectl drain` для одного узла kind и наблюдать перенос Pod |
| [09 стратегии обновления](./09-update-strategies.md) | обновить образ с `maxUnavailable: 0` и посмотреть, как чередуются старые и новые Pod |
| [10 конфигурация и секреты](./10-config-and-secret-delivery.md) | изменить `ConfigMap` и убедиться, что процесс из env его не увидел без нового Pod |
| [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) | запустить пример с `heartbeat.log`, удалить Pod и убедиться, что данные остались |
| [10 сеть](./12-networking-service-dns-and-ingress.md) | проверить DNS-имя `Service` из отладочного Pod, затем установить Ingress controller и опубликовать приложение |
| [11 разбор манифеста](./13-practical-manifest-review.md) | применить учебный манифест и пройти маршрут диагностики целиком |

Полезное упражнение — ломать намеренно. Неправильный selector `Service`, опечатка в имени `ConfigMap`, слишком строгая liveness probe: каждая ошибка даёт узнаваемый набор событий, который потом быстро распознаётся в рабочем окружении.

---

## Чем локальный кластер отличается от рабочего

Стенд на kind не воспроизводит существенную часть поведения:

- плоскость управления не отказоустойчива: один узел, один участник `etcd`;
- «узлы» являются контейнерами на одной машине, поэтому настоящего отказа машины, сетевого разделения и зон здесь нет;
- по умолчанию доступен только локальный `StorageClass`, а зональная привязка дисков, `Multi-Attach` и особенности CSI-драйверов не воспроизводятся;
- `Service` типа `LoadBalancer` не получает внешний адрес без дополнительной настройки;
- Ingress controller и Metrics Server по умолчанию не установлены, поэтому `kubectl top` и правила `Ingress` не работают, пока их не добавить;
- ресурсы ограничены одной машиной, поэтому нагрузочные выводы переносить нельзя.

Из этого следует практическое правило: локальный кластер хорошо проверяет **связи между объектами** и реакцию приложения, но не проверяет отказоустойчивость и производительность.

---

## Удалить кластер

```bash
kind delete cluster --name study
```

Команда удаляет контейнеры узлов и запись контекста из `kubeconfig`. Данные учебных PVC при этом теряются, потому что они находились внутри удалённых контейнеров.

---

## Официальные источники

- [kind: Quick Start](https://kind.sigs.k8s.io/docs/user/quick-start/)
- [kind: Configuration](https://kind.sigs.k8s.io/docs/user/configuration/)
- [Install and Set Up kubectl](https://kubernetes.io/docs/tasks/tools/)
- [minikube: Get Started](https://minikube.sigs.k8s.io/docs/start/)
