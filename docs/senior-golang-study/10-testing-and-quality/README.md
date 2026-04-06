# Testing And Quality

Сюда собирай материалы по проверке качества и инженерным практикам.

Темы:
- unit, integration, contract, end-to-end tests;
- table-driven tests;
- mocks vs fakes vs real dependencies;
- `testify`, `go-cmp`, `gomock`, `testcontainers-go`;
- race tests, fuzz tests, benchmarks;
- linters: `golangci-lint`, `staticcheck`, `govulncheck`;
- code review checklist;
- migration testing и rollback safety.

Senior-акцент:
- какие тесты реально защищают от регрессий;
- где mock-driven development вредит;
- как держать test suite быстрым и надежным;
- как выстроить quality gates в CI.

## Подборка

- [Package testing](https://pkg.go.dev/testing)
- [Fuzzing](https://go.dev/doc/fuzz/)
- [Coverage for Go applications](https://go.dev/doc/build-cover)
- [govulncheck Tutorial](https://go.dev/doc/tutorial/govulncheck)
- [golangci-lint Docs](https://golangci-lint.run/docs/)
- [Staticcheck Docs](https://staticcheck.dev/docs/)
- [Testcontainers for Go](https://golang.testcontainers.org/)

## Вопросы

- какие тесты ты бы написал первыми для критичного Go-сервиса;
- когда mock уместен, а когда лучше поднимать реальную зависимость;
- чем integration test полезнее десятка unit test в конкретном кейсе;
- как не превратить CI в медленный и хрупкий bottleneck;
- зачем нужны race test, fuzz test и benchmark, и где они реально окупаются;
- как ревьюить тесты, а не только production code.
