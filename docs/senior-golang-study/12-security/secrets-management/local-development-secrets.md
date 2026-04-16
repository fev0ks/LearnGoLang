# Local Development Secrets

Local dev почти всегда требует компромисса между удобством и безопасностью. Цель не в том, чтобы локальная среда была enterprise-grade, а в том, чтобы:
- не утекали реальные production secrets;
- новый разработчик мог быстро стартовать;
- значения не коммитились случайно в git.

## Что обычно работает лучше всего

### `.env.example`

Хороший базовый паттерн:
- в репозитории лежит `.env.example`;
- там только имена переменных и безопасные заглушки;
- реальные значения разработчик кладет в `.env.local` или `.env`.

Пример:

```env
APP_ENV=local
POSTGRES_DSN=postgres://app:app@localhost:5432/app?sslmode=disable
JWT_SECRET=change-me-local-only
```

Важно:
- реальные production keys в такой файл не кладут;
- `.env.local` должен быть в `.gitignore`.

### `direnv`

Подходит, когда:
- хочется автоматическую загрузку env vars в shell;
- команда комфортно работает через CLI.

Плюсы:
- быстро;
- удобно локально;
- не требует тащить секреты в Docker image.

### Local secret manager / password manager

Примеры:
- `1Password`
- `Bitwarden`
- локальный `Vault` dev setup

Подходит, когда:
- команда уже живет в password manager;
- секретов много;
- хочется не раздавать их по чатам и wiki.

## Чего не делать

- не хранить production secrets в `.env`;
- не коммитить `.env`;
- не копировать токены в README;
- не использовать один и тот же long-lived secret для local, staging и prod.

## Practical rule

Для local dev обычно достаточно:
- `.env.example` в репозитории;
- `.env.local` в `.gitignore`;
- dev-only credentials и отдельные local keys.

Если проект серьезнее:
- секреты для dev уже лучше раздавать через password manager или dev secret store.

## Что важно проговорить

- local convenience не оправдывает утечку production credentials;
- local dev secrets должны быть отдельны от production;
- onboarding должен быть удобным, но управляемым.
