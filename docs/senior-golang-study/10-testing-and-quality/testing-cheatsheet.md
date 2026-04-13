# Testing Cheatsheet

Короткий набор шаблонов, которые удобно копировать и адаптировать под задачу.

## Table-Driven Test

```go
func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "ok", input: "+7 (999) 111-22-33", want: "79991112233"},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePhone(tt.input)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
```

Когда использовать:
- простая business logic;
- parsing/validation;
- edge cases;
- deterministic behavior.

## `cmp.Diff`

```go
if diff := cmp.Diff(want, got); diff != "" {
	t.Fatalf("mismatch (-want +got):\n%s", diff)
}
```

Когда использовать:
- nested structs;
- slices/maps;
- DTO/domain mapping;
- JSON-like response objects.

## `httptest` For Handler

```go
req := httptest.NewRequest(http.MethodGet, "/v1/users/42", nil)
rr := httptest.NewRecorder()

handler.ServeHTTP(rr, req)

if rr.Code != http.StatusOK {
	t.Fatalf("got status %d", rr.Code)
}
if body := rr.Body.String(); body == "" {
	t.Fatal("expected non-empty body")
}
```

Когда использовать:
- HTTP handlers;
- middleware;
- auth/headers/status code checks;
- request/response behavior без реальной сети.

## `httptest.Server` For HTTP Client

```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"id":"42"}`))
}))
defer srv.Close()

client := NewClient(srv.URL)
got, err := client.GetUser(context.Background(), "42")
if err != nil {
	t.Fatalf("unexpected error: %v", err)
}
if got.ID != "42" {
	t.Fatalf("got id %q", got.ID)
}
```

Когда использовать:
- HTTP client code;
- retries;
- status code handling;
- headers, timeouts, malformed responses.

## `gomock`

```go
ctrl := gomock.NewController(t)
defer ctrl.Finish()

repo := NewMockUserRepo(ctrl)
repo.EXPECT().
	GetByID(gomock.Any(), "42").
	Return(User{ID: "42"}, nil)

svc := NewService(repo)
got, err := svc.LoadUser(context.Background(), "42")
if err != nil {
	t.Fatalf("unexpected error: %v", err)
}
if got.ID != "42" {
	t.Fatalf("got id %q", got.ID)
}
```

Когда использовать:
- важен exact interaction;
- notifier/publisher/audit calls;
- side effect must happen;
- dependency слишком дорогая для реального поднятия.

Не лучший выбор:
- repository tests;
- HTTP client tests;
- случаи, где результат важнее sequence of calls.

## Fake Repository

```go
type fakeRepo struct {
	users map[string]User
	err   error
}

func (f *fakeRepo) GetByID(ctx context.Context, id string) (User, error) {
	if f.err != nil {
		return User{}, f.err
	}
	u, ok := f.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}
```

Когда использовать:
- business logic tests;
- stateful dependency;
- нужен простой controlled setup без mock framework noise.

## `testcontainers-go`

```go
ctx := context.Background()
pg, err := postgres.Run(ctx, "postgres:16-alpine")
if err != nil {
	t.Fatal(err)
}
defer func() { _ = testcontainers.TerminateContainer(pg) }()

dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
if err != nil {
	t.Fatal(err)
}

db, err := sql.Open("pgx", dsn)
if err != nil {
	t.Fatal(err)
}
defer db.Close()
```

Когда использовать:
- Postgres/Redis/Kafka integration;
- migrations;
- repository layer;
- transactional behavior;
- real infra semantics.

## Fuzz Test

```go
func FuzzNormalizePhone(f *testing.F) {
	f.Add("+7 (999) 111-22-33")
	f.Add("")

	f.Fuzz(func(t *testing.T, input string) {
		_, _ = NormalizePhone(input)
	})
}
```

Запуск:

```bash
go test -fuzz=Fuzz -run=^$
```

Когда использовать:
- parsers;
- validators;
- input normalization;
- security-sensitive input handling.

## Benchmark

```go
func BenchmarkNormalizePhone(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = NormalizePhone("+7 (999) 111-22-33")
	}
}
```

Запуск:

```bash
go test -bench=. ./...
go test -bench=. -benchmem ./...
```

Когда использовать:
- hot path;
- compare two implementations;
- allocations/perf regressions.

## Race Detector

Запуск:

```bash
go test -race ./...
```

Когда использовать:
- goroutines;
- shared state;
- channels/mutex/atomic;
- shutdown logic;
- worker pools and caches.

## Handy Commands

```bash
go test ./...
go test ./... -run TestUserService
go test ./... -count=1
go test -race ./...
go test -fuzz=Fuzz -run=^$
go test -bench=. -benchmem ./...
```

## Quick Rule Of Thumb

`unit`:
- logic, validation, branching.

`httptest`:
- handlers and middleware.

`httptest.Server`:
- HTTP clients.

`go-cmp`:
- compare rich results.

`gomock`:
- verify important interactions.

`testcontainers-go`:
- real integration with infra.
