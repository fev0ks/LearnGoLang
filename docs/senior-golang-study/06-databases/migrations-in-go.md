# Migrations In Go

Эта заметка про то, как выбирать инструмент миграций в Go-проекте и как с ним работать без лишней магии.

## Содержание

- [Короткий ответ](#короткий-ответ)
- [Популярные инструменты](#популярные-инструменты)
- [`golang-migrate`](#golang-migrate)
- [`goose`](#goose)
- [`Atlas`](#atlas)
- [`gormigrate`](#gormigrate)
- [`dbmate`](#dbmate)
- [Что выбрать на практике](#что-выбрать-на-практике)
- [Что лучше не делать](#что-лучше-не-делать)
- [Нормальный production workflow](#нормальный-production-workflow)
- [Практические правила](#практические-правила)
- [Моя практическая рекомендация](#моя-практическая-рекомендация)
- [Главный принцип: Forward-Only In Production](#главный-принцип-forward-only-in-production)
- [migrate-down: roll back the last migration (DEV ONLY — never run in prod)](#migrate-down-roll-back-the-last-migration-dev-only--never-run-in-prod)
- [Кто и где запускает миграции](#кто-и-где-запускает-миграции)
- [Schema source of truth](#schema-source-of-truth)
- [Zero-downtime patterns: Expand / Contract](#zero-downtime-patterns-expand--contract)
- [Locks, timeouts, и DDL safety](#locks-timeouts-и-ddl-safety)
- [Dirty state recovery](#dirty-state-recovery)
- [Конкурентное применение и advisory locks](#конкурентное-применение-и-advisory-locks)
- [Schema review process](#schema-review-process)
- [Что отличает Atlas в этом контексте](#что-отличает-atlas-в-этом-контексте)
- [Production checklist](#production-checklist)
- [Финальное правило](#финальное-правило)

## Короткий ответ

Если нужен самый практичный выбор:
- `goose` + SQL миграции — когда хочешь boring и удобный production-путь;
- `Atlas` — когда у тебя GORM или schema-as-code workflow, и нужен diff, lint и versioned migrations;
- `golang-migrate` — когда уже есть папка SQL миграций и нужен простой раннер без дополнительной философии.

Что я бы не делал основным production-подходом:
- не полагался бы только на `GORM AutoMigrate` для зрелого сервиса.

Причина: сами docs GORM пишут, что `AutoMigrate` работает во многих случаях, но в какой-то момент стоит переходить к versioned migrations, и отдельно указывают на официальную интеграцию с Atlas.

Источники:
- [GORM Migration](https://gorm.io/docs/migration.html)
- [Atlas Versioned Migrations](https://atlasgo.io/versioned/intro)

## Популярные инструменты

## `golang-migrate`

Что это:
- классический инструмент для запуска миграций как CLI и как Go library;
- работает с `up/down` SQL файлами;
- хорошо подходит как "движок применения" миграций.

Плюсы:
- простой и понятный;
- много драйверов;
- легко встроить в CI/CD или отдельный admin command;
- хороший вариант, если миграции уже пишутся вручную в SQL.

Минусы:
- почти не помогает с планированием schema changes;
- сам не решает diff, lint и safety-анализ;
- DX обычно более утилитарный, чем у Atlas или goose.

Когда брать:
- legacy проект;
- уже есть готовая папка `migrations/*.sql`;
- нужен именно исполнитель миграций, а не экосистема вокруг схемы.

Источник:
- [golang-migrate/migrate](https://github.com/golang-migrate/migrate)

## `goose`

Что это:
- migration tool с CLI и library;
- поддерживает SQL migrations и Go migrations;
- умеет embedded migrations, `validate`, `fix`, out-of-order migrations.

Плюсы:
- очень удобный DX для Go-команд;
- SQL-first подход без лишней ORM-магии;
- можно хранить миграции рядом с приложением и запускать отдельной командой;
- часто хороший boring choice для Postgres-сервиса на Go.

Минусы:
- это все еще инструмент применения миграций, а не полноценный planner;
- если нужен schema diff и policy/lint вокруг миграций, придется дополнять процесс вручную.

Когда брать:
- `pgx`, `database/sql`, `sqlc`, ручной SQL;
- хочется ясный и предсказуемый production workflow;
- не нужен ORM-driven generation.

Источник:
- [pressly/goose](https://github.com/pressly/goose)

## `Atlas`

Что это:
- tooling вокруг versioned migrations и schema management;
- умеет diff, lint, inspect, apply и CI-friendly workflow;
- имеет официальную интеграцию с GORM.

Плюсы:
- сильный workflow для контроля schema changes;
- помогает не только "запустить SQL", но и спланировать миграции;
- лучше подходит для зрелого процесса, где важны review и safety checks;
- естественный выбор, если проект уже использует GORM, но хочет уйти от `AutoMigrate`.

Минусы:
- сложнее и тяжелее по процессу, чем просто раннер SQL;
- для маленьких pet-проектов может быть избыточен.

Когда брать:
- используешь GORM;
- хочешь versioned migrations вместо `AutoMigrate`;
- нужен более серьезный schema workflow для команды и CI.

Источники:
- [Atlas Docs](https://atlasgo.io/docs)
- [Atlas Versioned Migrations](https://atlasgo.io/versioned/intro)
- [GORM Migration](https://gorm.io/docs/migration.html)

## `gormigrate`

Что это:
- минималистичный helper поверх GORM для миграций кодом.

Плюсы:
- удобно, если хочешь оставаться внутри GORM API;
- низкий порог входа для небольшого проекта.

Минусы:
- migration logic остается завязанной на приложение и ORM;
- хуже читается и ревьюится, чем явные SQL миграции;
- для production schema evolution обычно слабее, чем `goose` или `Atlas`.

Когда брать:
- маленький проект;
- команда уверенно живет внутри GORM и осознанно принимает этот trade-off.

Источник:
- [go-gormigrate/gormigrate](https://github.com/go-gormigrate/gormigrate)

## `dbmate`

Что это:
- легкий framework-agnostic инструмент миграций.

Плюсы:
- простой;
- удобен для SQL-first подхода;
- не привязывает к ORM.

Минусы:
- в Go-экосистеме обычно встречается реже, чем `goose` и `golang-migrate`;
- если выбирать один "boring default" именно для Go, чаще выигрывает `goose`.

Когда брать:
- нужен очень легкий SQL migration tool;
- проект не хочет зависеть от ORM-specific подхода.

Источник:
- [amacneil/dbmate](https://github.com/amacneil/dbmate)

## Что выбрать на практике

### Сценарий 1: `pgx` или `database/sql`

Рекомендация:
- чаще всего `goose`.

Почему:
- SQL остается явным;
- удобно запускать и локально, и в CI;
- хорошо ложится на production backend без лишней магии.

### Сценарий 2: `GORM`

Рекомендация:
- скорее `Atlas`, чем `AutoMigrate`;
- `AutoMigrate` оставить максимум для раннего прототипа или dev-only сценариев.

Почему:
- у GORM есть официальный путь к Atlas;
- versioned migrations лучше подходят для production и review процесса;
- меньше риска незаметных drift-проблем между кодом и схемой.

### Сценарий 3: legacy проект с готовыми SQL миграциями

Рекомендация:
- `golang-migrate` или `goose`.

Практический выбор:
- если нужен максимально прямой SQL runner — `golang-migrate`;
- если хочется более приятный DX в Go-команде — `goose`.

## Что лучше не делать

- не запускать миграции на старте каждого pod/process без контроля, если это production;
- не смешивать schema migration и тяжелый data backfill в одну неуправляемую миграцию;
- не рассчитывать, что `down` всегда спасет production rollback;
- не делать destructive schema changes в один шаг, если код еще использует старую схему.

## Нормальный production workflow

1. Пишешь versioned migration.
2. Прогоняешь локально на чистой базе и на базе после предыдущих миграций.
3. Проверяешь lock/timeout/rollback risk.
4. Деплоишь миграцию как отдельный шаг release pipeline.
5. Только потом раскатываешь код, если изменение требует новой схемы.
6. Удаление старых колонок и cleanup делаешь отдельной поздней миграцией.

## Практические правила

- schema changes делай forward-compatible;
- для больших таблиц думай про lock impact;
- индексы и backfill оценивай отдельно;
- migration tool выбирай по workflow команды, а не по количеству звезд на GitHub;
- в review обсуждай не только SQL, но и rollout plan.

## Моя практическая рекомендация

Если нужен короткий выбор без лишней теории:
- `GORM` -> `Atlas`
- `pgx` / `database/sql` / `sqlc` -> `goose`
- legacy SQL migrations -> `golang-migrate`

Это не единственно правильный ответ, а инженерный default, который обычно дает меньше боли в реальном backend-проекте.

---

# Production-grade Migration Operations

Сам по себе инструмент (`goose`, `golang-migrate`, `Atlas`) — это только 20% задачи. Остальные 80% — это операционная обвязка вокруг него. Ниже — то, что отличает "у меня есть Makefile с migrate-up" от "так это работает в реальной компании".

## Главный принцип: Forward-Only In Production

В dev окружении можно делать `migrate down`. В production — **нельзя**.

Почему:
- `down` миграции почти никогда не тестируются на реальных данных;
- если миграция уже применилась к production базе и приложение записало новые данные в новую схему, `down` потеряет эти данные;
- rollback кода и rollback схемы — это разные вещи, и второе обычно невозможно сделать безопасно;
- даже если технически `down` отработает, состояние БД после него может не соответствовать ни старой, ни новой версии кода.

Правило:
- **в production только forward**;
- если миграция плохая — пишешь новую миграцию, которая откатывает изменения как **новый forward step** (revert PR);
- `down` файлы существуют только для локальной разработки и тестов.

В Makefile это стоит явно подписать:

```makefile
## migrate-down: roll back the last migration (DEV ONLY — never run in prod)
migrate-down:
	migrate -path $(MIGRATE_PATH) -database "$(DB_DSN)" down 1
```

## Кто и где запускает миграции

В production миграции **никогда не запускает человек руками** и **никогда не запускает приложение на старте**. Есть несколько типовых паттернов:

### 1. CI/CD pipeline step

Самый распространенный вариант:

```
build → test → migrate → deploy → smoke test
```

Pipeline применяет миграции отдельным шагом перед раскаткой нового кода. Если миграция падает — деплой не продолжается.

Плюсы: один источник правды, audit trail в CI, нельзя "забыть применить".

### 2. Kubernetes Init Container

```yaml
# фрагмент Deployment manifest
spec:
  template:
    spec:
      initContainers:
        - name: migrate
          image: migrate/migrate:v4.18.3
          args:
            - "-path=/migrations"
            - "-database=$(DB_DSN)"
            - "up"
```

Init container запускается до основного контейнера приложения. Если миграция падает — pod не стартует. Подходит для k8s окружений и хорошо сочетается с rolling update.

Минус: если у тебя несколько replicas, init container запускается на каждом pod. golang-migrate использует advisory lock в `schema_migrations`, поэтому конкурентного применения не будет, но шум в логах будет.

### 3. Helm pre-upgrade hook / ArgoCD pre-sync hook

Job, который запускается **один раз** перед upgrade релиза:

```yaml
metadata:
  annotations:
    "helm.sh/hook": pre-upgrade,pre-install
    "helm.sh/hook-weight": "-5"
    "helm.sh/hook-delete-policy": before-hook-creation
```

Это самый "правильный" k8s-native вариант: миграция запускается ровно один раз, отделена от приложения, статус виден в Helm/ArgoCD.

### 4. Standalone admin job

В простых проектах — отдельный CI job или GitHub Action с manual approval, который запускают перед релизом. Менее автоматизировано, но прозрачно и легко контролировать.

## Schema source of truth

Только миграционных файлов недостаточно — их сложно ревьюить целиком. В production принято дополнительно держать **schema dump**:

```bash
pg_dump --schema-only --no-owner --no-privileges shortener > db/schema.sql
```

Файл `schema.sql` коммитится в репозиторий **рядом** с миграциями. Это дает:

- **обзор всей текущей схемы** в одном месте — проще ревьюить и обсуждать;
- **drift detection в CI** — джоба прогоняет миграции на чистой базе, делает dump, сравнивает с закоммиченным `schema.sql`. Если diff не пустой — сборка падает. Это ловит ситуацию "миграцию написали, schema.sql забыли обновить" и любой ручной ALTER в живой базе;
- **ускоренный bootstrap** для тестов — можно загрузить `schema.sql` напрямую вместо прогона всех миграций по очереди (особенно когда их становится 200+).

Типичный CI шаг:

```yaml
- name: Verify schema dump
  run: |
    make migrate-up
    pg_dump --schema-only ... > /tmp/actual.sql
    diff -u db/schema.sql /tmp/actual.sql
```

## Zero-downtime patterns: Expand / Contract

Самая частая ошибка в миграциях — destructive changes в один шаг, пока старая версия кода еще работает. Это ломает rolling deploy: на короткий момент в production одновременно живут pod-ы со старым и новым кодом, и новая схема несовместима со старым кодом (или наоборот).

Правильный паттерн — **expand / contract**, он же **two-phase migration**:

### Пример: переименовать колонку `name → full_name`

**Неправильно (одна миграция):**
```sql
ALTER TABLE users RENAME COLUMN name TO full_name;
```

Старые pod-ы упадут с ошибкой "column name does not exist" сразу после применения.

**Правильно (три релиза):**

**Релиз 1 — Expand.** Миграция добавляет новую колонку и синхронизирует данные:
```sql
ALTER TABLE users ADD COLUMN full_name TEXT;
UPDATE users SET full_name = name WHERE full_name IS NULL;
```
Приложение **читает старую колонку, пишет в обе.** Старые pod-ы продолжают работать.

**Релиз 2 — Switch.** Приложение переключается на чтение из новой колонки. Все еще пишет в обе для безопасности.

**Релиз 3 — Contract.** После того как все pod-ы обновлены и данные в `full_name` гарантированно консистентны:
```sql
ALTER TABLE users DROP COLUMN name;
```

### Правила expand/contract

- **Никогда не делать `DROP COLUMN` в том же релизе, что добавляет код, который ее использует.**
- **Не добавлять `NOT NULL` колонку без `DEFAULT`** в одну миграцию — на больших таблицах это full table rewrite с long-running lock. Правильно: `ADD COLUMN NULL` → backfill отдельной миграцией → `ALTER COLUMN SET NOT NULL` отдельной миграцией.
- **Не переименовывать колонки** одним `RENAME` — добавь новую, скопируй, потом удали старую.
- **Не менять тип колонки** напрямую — добавь новую, перенеси данные, удали старую.
- **Backfill больших таблиц делается батчами**, не одним `UPDATE`. Часто — отдельным background job, не миграцией.

## Locks, timeouts, и DDL safety

В Postgres большинство DDL команд берут `ACCESS EXCLUSIVE LOCK`. Если миграция ждет лок, а другие транзакции его держат — миграция блокирует **все** запросы к таблице, пока лок не получен. Это классический outage.

### Обязательные настройки в каждой миграции

```sql
SET lock_timeout = '5s';
SET statement_timeout = '60s';

ALTER TABLE links ADD COLUMN ...;
```

- `lock_timeout` — если миграция не получила лок за 5 секунд, она падает с ошибкой вместо того чтобы блокировать production траффик. Лучше упавшая миграция, чем 5 минут downtime.
- `statement_timeout` — защита от случайно бесконечного `UPDATE`.

### `CREATE INDEX CONCURRENTLY`

Обычный `CREATE INDEX` берет `SHARE LOCK` и блокирует записи на все время построения. На таблице в 100M строк это часы downtime.

```sql
CREATE INDEX CONCURRENTLY idx_links_created_at ON links (created_at DESC);
```

`CONCURRENTLY` строит индекс без блокировки записи. Цена:
- **нельзя обернуть в транзакцию** — у golang-migrate для этого нужна специальная директива в файле миграции (`-- +goose NO TRANSACTION` для goose, или отдельный файл без транзакции для golang-migrate);
- **может оставить invalid index** если что-то пошло не так — нужен мониторинг и runbook на DROP + retry.

### Чек-лист для каждой миграции

- [ ] установлен `lock_timeout`?
- [ ] установлен `statement_timeout`?
- [ ] индексы создаются `CONCURRENTLY`?
- [ ] нет `DROP COLUMN`/`RENAME` без expand/contract?
- [ ] нет `NOT NULL` без `DEFAULT` на большой таблице?
- [ ] нет полного `UPDATE` без батчей?
- [ ] есть rollout plan, если миграция падает на середине?

## Dirty state recovery

Когда миграция падает посередине, golang-migrate (и goose) помечают `schema_migrations.dirty = true`. После этого любая попытка применить миграции упирается в:

```
error: Dirty database version N. Fix and force version.
```

Это **намеренное поведение** — инструмент не знает, в каком состоянии база, и отказывается продолжать. Recovery runbook:

1. **Не паниковать.** Не запускать `migrate up` повторно, не запускать `migrate down`.
2. **Понять, что именно упало.** Найти миграцию версии N, прочитать SQL, понять, какие операторы успели применится.
3. **Привести базу руками в одно из двух состояний:**
   - **полностью применить** оставшиеся операторы вручную (если уверены), затем `migrate force N`;
   - **полностью откатить** применившиеся изменения вручную, затем `migrate force N-1`.
4. **Зафиксить SQL** в файле миграции, чтобы он был идемпотентным или хотя бы безопасным при повторном запуске.
5. **Запустить `migrate up`** заново.

`migrate force` **не меняет данные** — он только перезаписывает значения `version` и `dirty` в служебной таблице. Нельзя использовать его как "магическую кнопку, чтобы прошло".

В production должен быть алерт на `schema_migrations.dirty = true` — это всегда инцидент.

## Конкурентное применение и advisory locks

В golang-migrate и goose в служебной таблице `schema_migrations` используется advisory lock на время применения. Это значит:
- **нельзя одновременно** запустить две `migrate up` против одной базы — вторая будет ждать;
- если pipeline упал и оставил лок повисшим, его можно увидеть через `pg_locks` и убить вручную (`SELECT pg_advisory_unlock_all()`);
- в k8s init container на нескольких replicas работает корректно — только один pod реально применяет, остальные ждут и видят "no change".

## Schema review process

В зрелых командах миграции ревьюятся **отдельно от обычного кода**, потому что у них другой профиль рисков. Обычно есть:

- **PR template** для schema changes с чек-листом из предыдущих секций;
- **обязательный апрувер** от platform-team или DBA на любую миграцию, изменяющую существующие таблицы;
- **lint в CI** — например, `atlas migrate lint` или [squawk](https://github.com/sbdchd/squawk) для Postgres, которые умеют детектить опасные паттерны (`DROP COLUMN`, `ALTER TYPE`, `CREATE INDEX` без `CONCURRENTLY`, `NOT NULL` без `DEFAULT`);
- **testing matrix** — миграция прогоняется на пустой базе, на базе с production-like данными (sanitized snapshot), и на базе после предыдущей миграции.

## Что отличает Atlas в этом контексте

`Atlas` популярен именно потому, что бесплатно дает несколько вещей из этого списка:

- `atlas migrate lint` — встроенные правила безопасности (`MF101: data loss`, `DS103: drop column`, и т.д.);
- `atlas migrate hash` — контрольная сумма миграций, чтобы нельзя было незаметно отредактировать уже примененную;
- `atlas schema diff` — сравнение текущей базы с целевой схемой, drift detection из коробки;
- `atlas migrate apply --baseline` — bootstrap на legacy базе без применения существующих миграций;
- CI plugins для GitHub Actions и GitLab.

Если ты в `golang-migrate` и тебе нужно построить весь этот workflow руками, миграция на Atlas часто оправдана. Если ты в `goose` и `goose validate` тебя устраивает — оставайся на нем.

## Production checklist

Когда ты говоришь "у меня production-grade миграции", это значит, что у тебя есть:

- [ ] forward-only правило в production задокументировано и enforced;
- [ ] миграции запускаются автоматически из CI/CD или через init container, а не руками;
- [ ] есть `schema.sql` дамп в репозитории и drift-check в CI;
- [ ] PR template с expand/contract чек-листом;
- [ ] `lock_timeout` и `statement_timeout` в каждой миграции;
- [ ] `CREATE INDEX` всегда `CONCURRENTLY` на больших таблицах;
- [ ] runbook для dirty state recovery;
- [ ] алерт на `schema_migrations.dirty = true`;
- [ ] lint миграций в CI (Atlas / squawk / самописное);
- [ ] backup или PITR подтвержден перед apply в production;
- [ ] тестовая база с production-like данными для прогона миграций;
- [ ] никто не может запустить `migrate down` против production (роли БД, IAM, или просто отсутствие команды в runbook).

Если из этого списка набирается 8+ — это уже зрелая практика. Если 3 и меньше — ты в категории "у меня есть Makefile target", и при первом инциденте это всплывет.

## Финальное правило

Инструмент миграций — это самая нижняя часть пирамиды. Над ним обязательно должны быть:
1. **процесс** (review, lint, expand/contract);
2. **автоматизация** (CI/CD, init containers, drift detection);
3. **runbook** (dirty recovery, lock timeout reaction, rollback plan).

Если ты выбираешь между инструментами, помни: разница между `goose` и `golang-migrate` — это 5% производственной зрелости. Остальные 95% — это то, что ты построишь вокруг любого из них.
