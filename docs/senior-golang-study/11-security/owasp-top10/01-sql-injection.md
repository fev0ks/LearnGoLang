# SQL Injection

SQL injection — атака, при которой пользовательский ввод "проникает" в SQL-запрос как часть кода, а не как данные. Атакующий может прочитать любые данные из БД, изменить их, удалить таблицы или (в худшем случае) исполнить команды на сервере БД.

Известна с 1998 года. По сей день — в OWASP Top 10. Главная причина — ленивая конкатенация строк при построении SQL.

## Содержание

- [Простая аналогия](#простая-аналогия)
- [Как работает атака](#как-работает-атака)
- [Типы SQL injection](#типы-sql-injection)
- [Защита: параметризованные запросы](#защита-параметризованные-запросы)
- [Особый случай — динамические идентификаторы](#особый-случай-динамические-идентификаторы)
- [ORM и query builders в Go](#orm-и-query-builders-в-go)
- [Дополнительные слои защиты](#дополнительные-слои-защиты)
- [К чему приводит успешная атака](#к-чему-приводит-успешная-атака)
- [Известные инциденты](#известные-инциденты)
- [Чек-лист защиты](#чек-лист-защиты)

---

## Простая аналогия

Представь форму заказа в кафе. Пользователь пишет в графе "имя": **`Иван' --`**. Бариста буквально копирует это в SQL: `INSERT INTO orders (name) VALUES ('Иван' --')`. Кавычка закрыла строку, `--` закомментировало остаток. Запрос ломается или, хуже того, делает что-то неожиданное.

**SQL injection — это смешение кода и данных.** Когда программа строит SQL через конкатенацию строк, граница между "это код, написанный программистом" и "это данные от пользователя" размывается. Атакующий пишет такие "данные", которые превращаются в код.

---

## Как работает атака

### Уязвимый код

```go
// НИКОГДА ТАК НЕ ДЕЛАЙ
query := fmt.Sprintf("SELECT * FROM users WHERE email = '%s'", email)
rows, _ := db.Query(query)
```

Пользователь вводит email через форму логина. Если ввести обычный email — запрос:
```sql
SELECT * FROM users WHERE email = 'alice@example.com'
```

А если ввести **`' OR '1'='1`** — получим:
```sql
SELECT * FROM users WHERE email = '' OR '1'='1'
```

Условие `'1'='1'` всегда истина — запрос вернёт всех пользователей. Если приложение логинит первого — атакующий вошёл как admin.

### Ещё хуже: stacked queries

```sql
SELECT * FROM users WHERE email = ''; DROP TABLE users; --'
```

Если драйвер БД позволяет несколько запросов через `;` — выполнятся оба. Таблицы больше нет. (Не все драйверы это разрешают: `database/sql` в Go по умолчанию запрещает stacked queries — но не во всех БД).

### Вытащить любые данные через UNION

```
email = ' UNION SELECT username, password_hash, NULL FROM admins --
```

Получится:
```sql
SELECT * FROM users WHERE email = '' UNION SELECT username, password_hash, NULL FROM admins --'
```

Запрос склеивает результаты — атакующий получает админ-хеши прямо в ответ API.

### Слепой SQL injection (blind)

Когда приложение не возвращает результаты SQL напрямую, но реагирует на содержимое (показывает "найдено" / "не найдено"). Атакующий побитово вытаскивает данные:

```
id = 1 AND (SELECT SUBSTRING(password, 1, 1) FROM admins WHERE id=1) = 'a'
```

Если ответ "найдено" — первая буква пароля 'a'. Не угадал — пробуем 'b', 'c'... Медленно, но автоматизируется.

### Time-based blind

Когда нет даже разницы в ответе — атакующий смотрит на время:

```
id = 1 AND IF(SUBSTRING(password,1,1)='a', SLEEP(5), 0)
```

Если запрос отвечает за 5 секунд — буква угадана. SQL Injection через secпundomer.

---

## Типы SQL injection

| Тип | Как работает | Пример |
|---|---|---|
| **In-band** | Результат возвращается в том же ответе | UNION-based, error-based |
| **Blind** | Ответ не содержит данных, но меняется поведение | Boolean-based |
| **Time-based blind** | Меняется только время ответа | `SLEEP()` для извлечения |
| **Out-of-band** | Данные эксфильтруются через другой канал (DNS, HTTP) | `LOAD_FILE()`, `xp_cmdshell` |
| **Second-order** | Вредоносный ввод сохраняется и срабатывает позже | Регистрация → атака при логине |

---

## Защита: параметризованные запросы

**Единственная надёжная защита** — параметризованные запросы (prepared statements). База данных получает SQL-шаблон отдельно от значений и никогда не путает их.

### Правильно

```go
// database/sql — встроенный механизм placeholder'ов
rows, err := db.QueryContext(ctx,
    "SELECT id, email FROM users WHERE email = $1",
    email,  // значение передаётся отдельно, не подставляется в строку
)

// PostgreSQL: $1, $2, $3
// MySQL/SQLite: ?
// SQL Server: @p1, @p2 или ?
```

Что бы пользователь ни ввёл — `email` будет интерпретирован как **значение строки**, не как SQL-код. Кавычки внутри значения автоматически экранируются драйвером.

### Почему это безопасно

БД получает запрос в два этапа:
1. Парсит шаблон `SELECT ... WHERE email = ?` — это структура запроса, неизменная
2. Привязывает значение `email` как параметр — оно не парсится, не интерпретируется

Даже если пользователь введёт `' OR '1'='1` — это просто строка, которая будет искаться в колонке email. Не найдётся, и слава богу.

### pgx (рекомендуется для PostgreSQL)

```go
import "github.com/jackc/pgx/v5"

rows, err := conn.Query(ctx,
    `SELECT id, email FROM users WHERE created_at > $1 AND status = $2`,
    since, status,
)
```

### sqlx — обёртка над database/sql

```go
import "github.com/jmoiron/sqlx"

var users []User
err := db.SelectContext(ctx, &users,
    "SELECT id, email FROM users WHERE org_id = $1",
    orgID,
)
```

### Named parameters

Для длинных запросов с многими параметрами:

```go
// sqlx.NamedQuery
_, err := db.NamedExec(`
    INSERT INTO users (email, name, org_id)
    VALUES (:email, :name, :org_id)
`, map[string]interface{}{
    "email":  email,
    "name":   name,
    "org_id": orgID,
})
```

---

## Особый случай — динамические идентификаторы

Параметризация работает для **значений**, но не для **идентификаторов** (имена таблиц, колонок) и SQL-ключевых слов (ASC/DESC, LIMIT). Их в placeholder положить нельзя.

### Уязвимый код

```go
// "Гибкая сортировка"
orderBy := r.URL.Query().Get("sort")  // user input!
query := fmt.Sprintf("SELECT * FROM posts ORDER BY %s", orderBy)
// orderBy = "title; DROP TABLE posts; --"
```

### Защита — allowlist

Никогда не подставляй пользовательский ввод в идентификатор. Проверяй по белому списку:

```go
var allowedSortColumns = map[string]string{
    "title":      "title",
    "created":    "created_at",
    "updated":    "updated_at",
}

func buildQuery(sort, dir string) (string, error) {
    column, ok := allowedSortColumns[sort]
    if !ok {
        return "", errors.New("invalid sort column")
    }

    direction := "ASC"
    if strings.ToUpper(dir) == "DESC" {
        direction = "DESC"
    }

    // column гарантированно из allowlist
    return fmt.Sprintf("SELECT * FROM posts ORDER BY %s %s", column, direction), nil
}
```

### Защита — экранирование идентификаторов (если allowlist невозможен)

В крайнем случае — использовать функцию quoting из библиотеки:

```go
import "github.com/jackc/pgx/v5/pgconn"

quoted := pgconn.QuoteIdentifier(userInput)
// безопасно подставить в SQL: SELECT * FROM users ORDER BY {quoted}
```

Но allowlist всё равно лучше — он явный и защищён от багов в quoting.

---

## ORM и query builders в Go

ORM **обычно** защищают от SQL injection — потому что внутри используют параметризацию. Но есть способы выстрелить себе в ногу.

### GORM

```go
// Безопасно — параметризованный запрос
db.Where("email = ?", email).First(&user)

// ОПАСНО — Raw с конкатенацией
db.Raw("SELECT * FROM users WHERE email = '" + email + "'").Scan(&user)

// ОПАСНО — Where с подстановкой
db.Where(fmt.Sprintf("email = '%s'", email)).First(&user)

// Безопасно — Raw с параметрами
db.Raw("SELECT * FROM users WHERE email = ?", email).Scan(&user)
```

### sqlc — генерация типобезопасных функций из SQL

```sql
-- queries.sql
-- name: GetUserByEmail :one
SELECT id, email, name FROM users WHERE email = $1;
```

```go
// Сгенерированный код — всегда параметризованный
user, err := q.GetUserByEmail(ctx, email)
```

sqlc — самый безопасный вариант: SQL пишется отдельно, генератор гарантирует параметризацию. Невозможно случайно сделать конкатенацию.

### squirrel — query builder

```go
import sq "github.com/Masterminds/squirrel"

q := sq.Select("id", "email").
    From("users").
    Where(sq.Eq{"email": email}).  // безопасно
    PlaceholderFormat(sq.Dollar)

sql, args, _ := q.ToSql()
// SELECT id, email FROM users WHERE email = $1
db.QueryContext(ctx, sql, args...)
```

---

## Дополнительные слои защиты

Параметризация — обязательна. Но defense-in-depth добавляет ещё несколько слоёв.

### 1. Принцип наименьших привилегий для БД-пользователя

У приложения должен быть свой DB-пользователь с минимумом прав:

```sql
-- Только нужные таблицы, только нужные операции
GRANT SELECT, INSERT, UPDATE ON users TO app_user;
GRANT SELECT ON config TO app_user;
-- НЕ давать DROP, ALTER, CREATE, GRANT
-- НЕ использовать superuser/postgres/root
```

Даже если SQL injection произошёл — атакующий не сможет удалить таблицы или прочитать данные других схем.

### 2. Row-Level Security (RLS) в PostgreSQL

```sql
ALTER TABLE posts ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON posts
    USING (org_id = current_setting('app.current_org_id')::uuid);
```

Даже если запрос возвращает все строки — БД отфильтрует только разрешённые. Защищает от tenant isolation breach.

### 3. WAF (Web Application Firewall)

Cloudflare, AWS WAF, ModSecurity — детектят типичные SQL injection паттерны (`UNION SELECT`, `OR 1=1`, `'; --`) на уровне HTTP. Это не замена параметризации, а дополнительная сетка.

### 4. Логирование подозрительных запросов

```go
// Если запрос к БД упал из-за syntax error — это сигнал.
// Нормальный пользователь syntax error не вызовет.
if isPostgresSyntaxError(err) {
    slog.WarnContext(ctx, "potential sql injection attempt",
        "user_id", userID, "input", suspiciousInput, "err", err)
}
```

Аномалия "10 syntax errors с одного IP за минуту" — почти всегда автоматический сканер.

### 5. Валидация ввода (defense-in-depth, не основная защита)

Если ожидаешь UUID — проверь формат. Если число — преобразуй явно. Не как замена параметризации, а как первый фильтр:

```go
userID, err := uuid.Parse(r.PathValue("id"))
if err != nil {
    http.Error(w, "invalid id", http.StatusBadRequest)
    return
}
// Дальше userID — гарантированно валидный UUID
```

---

## К чему приводит успешная атака

В порядке убывания тяжести:

**1. Полный data exfiltration.** Атакующий выкачивает всю БД: пользователи, пароли (хеши), email, платёжные данные, переписку. Самый частый исход.

**2. Authentication bypass.** Через `' OR '1'='1` атакующий логинится как любой пользователь, включая admin'ов. Полный доступ к приложению с правами выбранного пользователя.

**3. Data tampering.** UPDATE/DELETE через injection. Атакующий меняет цены, балансы, отправителей платежей. Без логов трудно даже понять что произошло.

**4. RCE на сервере БД.** В некоторых конфигурациях — через `xp_cmdshell` (MSSQL), `COPY ... FROM PROGRAM` (PostgreSQL с superuser), UDF в MySQL. БД-сервер становится точкой входа в инфраструктуру.

**5. Lateral movement.** С DB-сервера — доступ к другим внутренним сервисам через метаданные cloud (IMDS), credentials в `pg_user`, network access.

**6. Регуляторные последствия.** GDPR-штраф до 4% годового оборота. PCI-DSS — потеря права обрабатывать карты. HIPAA — миллионные штрафы за утечку медицинских данных.

---

## Известные инциденты

**Heartland Payment Systems (2008)** — SQL injection дал доступ к 134 млн карт. Один из крупнейших инцидентов в истории платежей. Штрафы и расходы — $145 млн.

**Yahoo (2012)** — через SQL injection украдено 450k паролей в plain text (sic!). Yahoo не использовал хеширование.

**TalkTalk (2015, UK)** — SQL injection через старую страницу, забытую в продакшне. 157k записей пользователей. Штраф £400k от ICO. Атакующему — 4 года тюрьмы.

**Equifax (2017)** — не SQL injection, но иллюстрация: уязвимость в Apache Struts → доступ к БД → 147 млн записей. $700 млн settlement.

**Cisco (2018)** — SQL injection в их Prime License Manager. Authentication bypass для администраторов.

**LinkedIn / Tumblr / MySpace (2012-2016)** — серия утечек, многие через SQL injection. Миллиарды пар email/пароль распространились в публичных утечках, до сих пор используются в credential stuffing атаках.

Реальность: SQL injection не "ушёл в прошлое". Каждый год находят в legacy-системах, в новых проектах, в хайповых стартапах. Это не сложная атака — это автоматизированный сканер с sqlmap.

---

## Чек-лист защиты

**Обязательно:**
- ✅ Все SQL-запросы — через параметризацию (`$1`, `?`)
- ✅ Никакой `fmt.Sprintf` или конкатенации с пользовательским вводом в SQL
- ✅ ORM — только методы, которые гарантируют параметризацию (`Where("col = ?", val)`)
- ✅ Динамические идентификаторы (имена колонок) — через allowlist
- ✅ DB-пользователь приложения — минимум прав, никогда superuser
- ✅ Code review с явным фокусом: "где здесь конкатенация SQL?"

**Сильно желательно:**
- ✅ Использовать `sqlc` или query builder — компилятор не даст забыть параметризацию
- ✅ RLS на multi-tenant таблицах
- ✅ WAF на edge
- ✅ Логирование SQL-syntax-error как security event
- ✅ Static analysis (gosec для Go) — детектит подозрительные паттерны

**Тестирование:**
- ✅ Включить sqlmap в pipeline (или хотя бы пройти руками против test environment)
- ✅ Fuzz тесты с пограничными вводами (`'`, `--`, `\0`, очень длинные строки)
- ✅ Penetration testing раз в год

**Если используется AI-кодогенерация:**
- ✅ Особенно внимательно проверять SQL — модели любят генерировать `fmt.Sprintf` запросы
- ✅ Запрещать через linter паттерн `db.Query(fmt.Sprintf(...`
