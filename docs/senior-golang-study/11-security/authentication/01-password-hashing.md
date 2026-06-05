# Хеширование паролей

Нельзя хранить пароли в открытом виде или через обычный хеш (MD5, SHA-256). Для паролей нужны специальные slow hash функции: `bcrypt` или `argon2id`.

## Содержание

- [Почему нельзя SHA-256](#почему-нельзя-sha-256)
- [bcrypt](#bcrypt)
- [argon2id — современный выбор](#argon2id--современный-выбор)
- [Прозрачный апгрейд алгоритма](#прозрачный-апгрейд-алгоритма)
- [Что никогда не делать](#что-никогда-не-делать)

---

## Почему нельзя SHA-256

SHA-256 разработан чтобы работать быстро. На современном GPU перебирают миллиарды SHA-256 хешей в секунду. Если утекает база с SHA-256 паролями — словарные атаки взламывают её за часы.

Slow hash намеренно тратит ресурсы (CPU, память): один хеш считается 50-300ms. Это не заметно при логине, но делает брутфорс базы практически невозможным.

---

## bcrypt

Стандарт де-факто, поддерживается в Go через `golang.org/x/crypto/bcrypt`.

```go
import "golang.org/x/crypto/bcrypt"

// Хеширование при регистрации
func HashPassword(password string) (string, error) {
    // cost=12 — разумный минимум; 14 для высокой чувствительности
    hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
    if err != nil {
        return "", fmt.Errorf("hash password: %w", err)
    }
    return string(hash), nil
}

// Проверка при логине
func CheckPassword(hash, password string) error {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
        return ErrInvalidCredentials  // не раскрывать деталь
    }
    return err
}
```

**Лимит bcrypt: 72 байта.** bcrypt молча обрезает пароль до 72 байт. Два пароля которые совпадают в первых 72 байтах — дадут одинаковый хеш.

```go
// Явно проверить лимит перед хешированием
func HashPassword(password string) (string, error) {
    if len(password) > 72 {
        return "", ErrPasswordTooLong  // или обрезать сами — но тогда документировать
    }
    hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
    // ...
}
```

**Cost factor:** каждое увеличение на 1 удваивает время. При cost=12 хеширование занимает ~250ms на среднем сервере. Измерить на своём железе:

```go
func BenchmarkBcrypt(b *testing.B) {
    for b.Loop() {
        bcrypt.GenerateFromPassword([]byte("password"), 12)
    }
}
```

---

## argon2id — современный выбор

Argon2id выиграл Password Hashing Competition (2015). Устойчив к GPU-атакам за счёт memory hardness. OWASP рекомендует его как первый выбор.

```go
import (
    "crypto/rand"
    "encoding/base64"
    "fmt"

    "golang.org/x/crypto/argon2"
)

type argon2Params struct {
    Memory      uint32 // KB
    Iterations  uint32
    Parallelism uint8
    SaltLength  uint32
    KeyLength   uint32
}

// OWASP минимальные параметры для argon2id
var defaultArgon2Params = argon2Params{
    Memory:      64 * 1024,  // 64 MB
    Iterations:  3,
    Parallelism: 2,
    SaltLength:  16,
    KeyLength:   32,
}

func HashPassword(password string) (string, error) {
    p := defaultArgon2Params

    salt := make([]byte, p.SaltLength)
    if _, err := rand.Read(salt); err != nil {
        return "", fmt.Errorf("generate salt: %w", err)
    }

    hash := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)

    // Формат: $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
    // Параметры хранятся рядом с хешем — можно менять без потери совместимости
    encoded := fmt.Sprintf(
        "$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
        argon2.Version,
        p.Memory, p.Iterations, p.Parallelism,
        base64.RawStdEncoding.EncodeToString(salt),
        base64.RawStdEncoding.EncodeToString(hash),
    )
    return encoded, nil
}

func CheckPassword(encodedHash, password string) error {
    p, salt, hash, err := decodeArgon2Hash(encodedHash)
    if err != nil {
        return fmt.Errorf("decode hash: %w", err)
    }

    otherHash := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)

    // Constant-time сравнение — защита от timing attack
    if !subtle.ConstantTimeCompare(hash, otherHash) {
        return ErrInvalidCredentials
    }
    return nil
}
```

**Почему constant-time сравнение важно:** обычное `bytes.Equal` возвращает false на первом несовпавшем байте — по времени ответа атакующий может угадывать правильные префиксы. `subtle.ConstantTimeCompare` всегда обходит оба слайса целиком.

---

## Прозрачный апгрейд алгоритма

Хеш хранит тип алгоритма и параметры. При логине можно проверить алгоритм и перехешировать на новый — пользователь ничего не замечает.

```go
func (s *AuthService) Login(ctx context.Context, email, password string) (*Session, error) {
    account, err := s.repo.GetByEmail(ctx, email)
    if err != nil {
        // Constant-time: даже при "не найден" выполнить фейковую проверку
        bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
        return nil, ErrInvalidCredentials
    }

    if err := s.checkPassword(account.PasswordHash, password); err != nil {
        return nil, ErrInvalidCredentials
    }

    // Перехешировать если алгоритм устарел или параметры изменились
    if needsRehash(account.PasswordHash) {
        newHash, err := HashPassword(password)
        if err == nil {
            // best-effort — не фейлить логин из-за ошибки апгрейда
            _ = s.repo.UpdatePasswordHash(ctx, account.ID, newHash)
        }
    }

    return s.issueSession(ctx, account)
}

func needsRehash(hash string) bool {
    // bcrypt: проверить cost
    if strings.HasPrefix(hash, "$2") {
        cost, err := bcrypt.Cost([]byte(hash))
        return err != nil || cost < 12
    }
    // argon2id: проверить параметры
    if strings.HasPrefix(hash, "$argon2id") {
        p, _, _, err := decodeArgon2Hash(hash)
        return err != nil || p.Memory < 64*1024 || p.Iterations < 3
    }
    return true  // неизвестный формат — перехешировать
}
```

---

## Что никогда не делать

```go
// Никогда не логировать пароли
log.Info("login attempt", "email", email, "password", password)  // ← НЕЛЬЗЯ

// Никогда не сравнивать хеши через ==
if account.PasswordHash == HashPassword(password) { ... }  // timing attack

// Никогда не хранить пароль в структуре дольше необходимого
type LoginRequest struct {
    Email    string
    Password string  // OK пока не обработали — но не хранить в сессии или БД
}

// После проверки — явно затереть
defer func() { req.Password = "" }()

// Никогда не возвращать разные ошибки для "нет пользователя" и "неверный пароль"
// — атакующий перебирает email по различию ответов (user enumeration)
if !userFound {
    return ErrUserNotFound   // ← НЕЛЬЗЯ — раскрывает что пользователя нет
}
return ErrInvalidCredentials  // ← одна ошибка для обоих случаев
```

**Парольная политика (минимум):**
- минимальная длина: 12 символов
- максимальная длина: 72 байта (bcrypt) или 1000 байт (argon2id, против DoS)
- не требовать спецсимволы — NIST SP 800-63B рекомендует проверять по списку скомпрометированных паролей вместо сложных правил
