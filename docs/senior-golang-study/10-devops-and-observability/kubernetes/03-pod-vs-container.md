# Pod и container: в чём разница

Container — запускаемый изолированный процесс, а Pod — минимальная единица размещения и жизненного цикла в Kubernetes. Один Pod обычно содержит один основной container, но может объединять несколько тесно связанных процессов.

## Содержание

- [Короткое сравнение](#короткое-сравнение)
- [Что разделяют containers внутри Pod](#что-разделяют-containers-внутри-pod)
- [Кто и что перезапускает](#кто-и-что-перезапускает)
- [Multi-container Pod](#multi-container-pod)
- [Init containers](#init-containers)
- [Pod phase и container state](#pod-phase-и-container-state)
- [Interview-ready answer](#interview-ready-answer)

## Короткое сравнение

| | Container | Pod |
| --- | --- | --- |
| Что это | процесс с filesystem view, env и resource controls | один или несколько containers с общей сетевой identity |
| Кто запускает | container runtime | kubelet через container runtime |
| Единица scheduling | нет | да, scheduler выбирает Node для Pod |
| IP в Kubernetes | использует сеть Pod | Pod получает IP |
| Жизненный цикл | может быть перезапущен внутри Pod | конкретный Pod не переносится на другую Node; создаётся новый |

Kubernetes не создаёт standalone container как workload API object: container описывается внутри `PodSpec`. На практике Pod stateless-сервиса создаёт Deployment через ReplicaSet.

## Что разделяют containers внутри Pod

Containers одного Pod:

- используют один network namespace, IP и пространство портов;
- обращаются друг к другу через `localhost`;
- могут монтировать одни и те же Pod volumes;
- планируются на одну Node и живут в рамках одного Pod lifecycle.

При этом у каждого container собственные image filesystem, environment, security context и resource requests/limits. Process namespace по умолчанию не общий; его можно включить через `shareProcessNamespace`.

Общий network namespace позволяет sidecar proxy видеть трафик приложения, но само перенаправление обычно настраивается через iptables, eBPF или другой dataplane, а не возникает автоматически из-за `localhost`.

## Кто и что перезапускает

Здесь важно разделять два механизма:

1. **Kubelet перезапускает отдельный container** внутри того же Pod согласно container restart policy.
2. **Workload controller создаёт новый Pod**, если Pod удалён, потерян вместе с Node или больше не соответствует желаемому template.

Новый Pod получает другой UID и обычно другой IP. Kubernetes не «переносит» существующий Pod между Node.

Поэтому утверждение «при падении container всегда пересоздаётся весь Pod» неверно. Например, `CrashLoopBackOff` обычно означает повторные перезапуски одного container kubelet-ом с увеличивающейся задержкой.

## Multi-container Pod

Дополнительный container оправдан, когда он должен разделять сеть или volume и иметь общий lifecycle с основным приложением:

- proxy или service-mesh sidecar;
- агент, преобразующий или отправляющий локальные данные;
- вспомогательный процесс, обслуживающий только этот Pod.

Не стоит помещать в один Pod два независимо масштабируемых сервиса. Их нельзя отдельно разместить, масштабировать или обновить.

Kubernetes также поддерживает **native sidecar containers**: они описываются в `initContainers` с `restartPolicy: Always`, запускаются до основных containers и продолжают работать вместе с ними. Kubelet учитывает их порядок при завершении Pod. Обычные containers в `spec.containers` такой специальной гарантии порядка shutdown не дают.

## Init containers

Обычный init container должен успешно завершиться до запуска следующего init container и основных containers:

```yaml
spec:
  initContainers:
    - name: prepare-config
      image: registry.example/config-renderer:v2
      command: ["/app/render", "--output=/work/config.yaml"]
      volumeMounts:
        - name: work
          mountPath: /work
  containers:
    - name: api
      image: registry.example/api:v1.2.3
      volumeMounts:
        - name: work
          mountPath: /etc/app
  volumes:
    - name: work
      emptyDir: {}
```

Хорошие применения: подготовить файлы, проверить локальные prerequisites, получить одноразовый bootstrap artifact.

Миграцию общей базы опасно запускать в init container каждой реплики: несколько Pod могут выполнять её одновременно, а rollout блокируется до завершения. Обычно миграции оформляют отдельным `Job` с контролируемой совместимостью схемы.

## Pod phase и container state

Pod phase — грубое итоговое состояние:

| Phase | Смысл |
| --- | --- |
| `Pending` | Pod принят API, но один или несколько containers ещё не готовы к запуску |
| `Running` | Pod назначен Node, containers созданы, хотя бы один основной container работает или стартует повторно |
| `Succeeded` | все containers завершились успешно и не будут перезапущены |
| `Failed` | все containers завершились, и хотя бы один завершился неуспешно либо Pod завершён системой как failed |
| `Unknown` | состояние Pod не удалось получить, обычно из-за связи с Node |

У container отдельно есть state: `Waiting`, `Running` или `Terminated`, а также `reason`, restart count и last termination state.

`CrashLoopBackOff`, `ImagePullBackOff` и `Terminating` — не Pod phases. Это отображаемые `kubectl` причины/состояния, собранные из нескольких полей. Поэтому для диагностики нужны `kubectl describe pod` и `kubectl logs --previous`, а не только колонка `STATUS`.

## Interview-ready answer

**1. Чем Pod отличается от container?**

Container — изолированный процесс, а Pod — единица scheduling и lifecycle Kubernetes. Containers одного Pod используют общий IP и могут делить volumes; scheduler всегда размещает их вместе.

**2. Перезапускается container или весь Pod?**

Kubelet может перезапустить упавший container внутри того же Pod. Если нужна замена Pod целиком, Deployment/ReplicaSet создаёт новый объект с новым UID; существующий Pod между Node не переносится.

**3. Когда нужен multi-container Pod?**

Когда процессы тесно связаны сетью, volume и lifecycle, например приложение и proxy sidecar. Независимо масштабируемые сервисы должны быть разными workload.

**4. Является ли `CrashLoopBackOff` фазой Pod?**

Нет. Это причина ожидания container между повторными запусками. Pod phase при этом часто остаётся `Running` или `Pending`; точную причину смотрят в container status, events и предыдущих logs.

## Официальные источники

- [Pods](https://kubernetes.io/docs/concepts/workloads/pods/)
- [Pod lifecycle](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/)
- [Init containers](https://kubernetes.io/docs/concepts/workloads/pods/init-containers/)
- [Sidecar containers](https://kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/)
