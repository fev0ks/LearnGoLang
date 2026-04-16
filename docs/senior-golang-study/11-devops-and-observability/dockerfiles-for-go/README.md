# Dockerfiles For Go

Этот подпакет про Dockerfile patterns для Go-проектов: production, distroless/scratch, dev hot reload и случаи с `CGO`.

Материалы:
- [Dockerfile Anatomy](./dockerfile-anatomy.md)
- [Dockerfiles For Go Projects](./dockerfiles-for-go-projects.md)
- [Why Dockerfile Is Needed](./why-dockerfile-is-needed.md)
- [Multi-stage Scratch Example](./Dockerfile.scratch.example)
- [Distroless Example](./Dockerfile.distroless.example)
- [Dev Hot Reload Example](./Dockerfile.dev-hot-reload.example)
- [CGO Runtime Example](./Dockerfile.cgo-runtime.example)

Что важно уметь объяснить:
- зачем нужен multi-stage build;
- когда подходит `scratch`, а когда нужен `distroless` или Debian-like runtime;
- как Dockerfile для dev отличается от production;
- почему `CGO` меняет выбор runtime image.
