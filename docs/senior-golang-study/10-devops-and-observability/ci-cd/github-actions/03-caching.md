# GitHub Actions: Кеширование

## Содержание

- [Как работает кеш в GHA](#как-работает-кеш-в-gha)
- [Автоматический кеш (setup-go cache: true)](#автоматический-кеш-setup-go-cache-true)
- [Ручной кеш: cache/restore + cache/save](#ручной-кеш-cacherestore--cachesave)
- [restore-keys: fallback стратегия](#restore-keys-fallback-стратегия)
- [Кеш Go modules и build cache](#кеш-go-modules-и-build-cache)
- [Docker layer cache в GHA](#docker-layer-cache-в-gha)
- [Антипаттерны кеширования](#антипаттерны-кеширования)

---

## Как работает кеш в GHA

GitHub Actions хранит кеш на серверах GitHub (до 10GB на репо, потом старые вытесняются).

Жизненный цикл:
1. **Restore**: ищет кеш по точному ключу. Если нет — ищет по `restore-keys` (prefix match, берёт самый свежий).
2. **Job работает**.
3. **Save**: сохраняет кеш с новым ключом (только если кеш не был найден точно).

Кеш **изолирован между ветками** по умолчанию: PR может использовать кеш из main, но main не может использовать кеш из PR.

```
Порядок поиска:
1. Точный ключ:     go-ubuntu-latest-abc123-lint    ← exact match
2. restore-keys[0]: go-ubuntu-latest-abc123-         ← prefix, самый свежий
3. restore-keys[1]: go-ubuntu-latest-                ← более широкий prefix
```

---

## Автоматический кеш (setup-go cache: true)

Простейший способ — включить встроенный кеш в `actions/setup-go`:

```yaml
- uses: actions/setup-go@v5
  with:
    go-version-file: go.mod
    cache: true                      # default: true
    cache-dependency-path: go.sum    # по умолчанию ищет go.sum в репо
```

Под капотом это делает то же самое что и ручной кеш, но с меньшим контролем. **Нет** `restore-keys` — при изменении `go.sum` кеш не восстанавливается.

Когда подходит:
- Простой монорепо с одним `go.mod`.
- Не нужен partial cache fallback.
- Хочется минимум конфигурации.

---

## Ручной кеш: cache/restore + cache/save

Разделение restore и save на отдельные шаги даёт контроль: сохранять только если кеш не был найден.

```yaml
- uses: actions/setup-go@v5
  id: setup-go
  with:
    go-version-file: go.mod
    cache: false                     # отключаем встроенный

- name: Restore Go cache
  id: go-cache
  uses: actions/cache/restore@v4
  with:
    path: |
      ~/go/pkg/mod
      ~/.cache/go-build
    key: go-${{ runner.os }}-${{ steps.setup-go.outputs.go-version }}-${{ hashFiles('go.sum') }}-${{ github.job }}
    restore-keys: |
      go-${{ runner.os }}-${{ steps.setup-go.outputs.go-version }}-${{ hashFiles('go.sum') }}-
      go-${{ runner.os }}-${{ steps.setup-go.outputs.go-version }}-

# ... основные шаги job ...

- name: Save Go cache
  if: steps.go-cache.outputs.cache-hit != 'true'    # только если не было exact match
  uses: actions/cache/save@v4
  with:
    path: |
      ~/go/pkg/mod
      ~/.cache/go-build
    key: go-${{ runner.os }}-${{ steps.setup-go.outputs.go-version }}-${{ hashFiles('go.sum') }}-${{ github.job }}
```

Почему `${{ github.job }}` в ключе:
- У каждого job свой ключ — lint и test могут иметь разные наборы скомпилированных пакетов.
- Без job-суффикса первый сохранённый job "побеждает", остальные не сохраняют свой кеш.

---

## restore-keys: fallback стратегия

`restore-keys` — список prefix для partial match. Если точный ключ не найден, GHA ищет самый свежий кеш с этим prefix.

```yaml
key: go-ubuntu-latest-1.22.3-abc123def456-lint
restore-keys: |
  go-ubuntu-latest-1.22.3-abc123def456-    # тот же go.sum, другой job
  go-ubuntu-latest-1.22.3-                 # та же версия Go, любой go.sum
  go-ubuntu-latest-                        # широкий fallback
```

Сценарий при обновлении зависимостей:
1. `go.sum` изменился → точный ключ не найден.
2. `restore-keys[0]` не найден (новый хеш).
3. `restore-keys[1]` — найден самый свежий кеш с той же вер��ией Go.
4. Job восстанавлива��т старый кеш, скачивает только новые модули.
5. Сохраняет новый кеш с актуальным ключом.

Без `restore-keys` при каждом изменении `go.sum` — полный cache miss, скачиваются все модули.

---

## Кеш Go modules и build cache

### ~/go/pkg/mod — скачанные модули

Кешируются zip-архивы исходников зависимостей. Инвалидировать при изменении `go.sum`.

```
~/go/pkg/mod/
├── cache/download/         # zip архивы
└── github.com/jackc/pgx/  # распакованные исходники
```

### ~/.cache/go-build — build cache

Скомпилированные пакеты. Значительно ускоряет повторные сборки — Go переиспользует уже скомпилированные пакеты если исходники и флаги не изменились.

```
~/.cache/go-build/
└── 3f/3fad2...   # скомпилированные .a файлы
```

**Оба пути нужны для максимального ускорения.** Только `~/go/pkg/mod` — модули скачаны, но сборка с нуля. Только `~/.cache/go-build` — сборка быстрее, но модули нужно скачать.

### go.work монорепо

При использовании `go.work` (Go workspace) кеш-ключ должен включать все `go.sum`:

```yaml
key: go-${{ runner.os }}-${{ hashFiles('go.work', 'go.work.sum', '**/go.sum') }}
```

---

## Docker layer cache в GHA

`docker/build-push-action` поддерживает несколько backend для кеша:

### type=gha (GitHub Actions cache)

```yaml
- uses: docker/build-push-action@v6
  with:
    cache-from: type=gha,scope=myapp
    cache-to: type=gha,mode=max,scope=myapp,ignore-error=true
```

- `mode=max` — кешировать все intermediate layers, не только финальный.
- `scope` — изолирует кеш между разными образами в одном репо.
- `ignore-error=true` — не падать если не удалось сохранить кеш (например, превышен лимит).

Ограничение: кеш занимает место в 10GB квоте репо.

### type=registry (реестр как кеш)

```yaml
- uses: docker/build-push-action@v6
  with:
    cache-from: |
      type=gha,scope=myapp
      type=registry,ref=ghcr.io/myorg/myapp:main-latest
    cache-to: type=gha,mode=max,scope=myapp,ignore-error=true
```

Двойной источник: сначала пробуем GHA кеш (быстрее), потом registry (всегда актуален, не вытесняется).

Из skibookers:
```yaml
# Проверяем существует ли образ в registry перед использованием как кеш
- id: registry-cache
  run: |
    if gcloud artifacts docker images describe "${CACHE_REF}" >/dev/null 2>&1; then
      echo "registry_cache_from=type=registry,ref=${CACHE_REF}" >> "${GITHUB_OUTPUT}"
    else
      echo "registry_cache_from=" >> "${GITHUB_OUTPUT}"
    fi

- uses: docker/build-push-action@v6
  with:
    cache-from: |
      type=gha,scope=${{ matrix.name }}
      ${{ steps.registry-cache.outputs.registry_cache_from }}
```

---

## Антипаттерны кеширования

**Кешировать без `go-version` в ключе** — при обновлении Go старый build cache несовместим с новой версией компилятора.

```yaml
# плохо
key: go-${{ runner.os }}-${{ hashFiles('go.sum') }}

# хорошо
key: go-${{ runner.os }}-${{ steps.setup-go.outputs.go-version }}-${{ hashFiles('go.sum') }}
```

**Использовать `actions/cache@v4` вместо `cache/restore` + `cache/save`** — нельзя добавить условие `if: cache-hit != 'true'` на сохранение. Каждый раз пытается сохранить, даже если к��ш уже был.

**Один ключ для всех jobs** — линтер и тесты пишут в один кеш, возникает race condition, кеш одного job перезаписывает кеш другого.

**Не кешировать `~/.cache/go-build`** — модули скачаны, но каждый раз перекомпилируютс�� все пакеты с нуля. На проектах с большим количеством зависимостей потеря 1-3 минут.

**`cache-to` без `ignore-error=true`** — при превышении квоты или сетевой ошибке весь job упадёт только из-за невозможности сохранить кеш. Кеш — оптимизация, не требование.
