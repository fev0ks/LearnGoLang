# Security

Сюда собирай практические темы по безопасности backend-сервисов.

Базовые заметки:
- [Secrets Management](./secrets-management/README.md)
- [Service To Service TLS](./service-to-service-tls/README.md)
- [Perimeter And Traffic Protection](./perimeter-and-traffic-protection/README.md)
- [CORS And Browser API Security](./cors-and-browser-api-security/README.md)

Темы:
- authentication и authorization;
- JWT, opaque tokens, session-based auth;
- password hashing;
- mTLS, TLS termination;
- secrets management;
- SQL injection, SSRF, CSRF, XSS для API и админок;
- rate limiting как элемент защиты;
- audit trail;
- dependency and supply-chain security;
- least privilege для сервисов и баз.

Полезный senior-фокус:
- какие угрозы актуальны именно для твоего сервиса;
- как найти баланс между security и delivery speed;
- какие меры обязательны по умолчанию в новых сервисах.

Структура раздела:
- `secrets-management` - где хранить секреты, как передавать их в сервис и как не утекать в git, CI, images и логи
- `service-to-service-tls` - как устроены `TLS termination`, `re-encryption`, `mTLS` и зачем внутренним сервисам могут понадобиться сертификаты
- `perimeter-and-traffic-protection` - как думать про DDoS, perimeter filters и почему backend не должен быть первой линией защиты
- `cors-and-browser-api-security` - как работает `CORS`, что такое preflight и где эту политику обычно держат

## Подборка

- [OWASP Cheat Sheet Series](https://cheatsheetseries.owasp.org/)
- [Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
- [Secrets Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html)
- [Denial of Service Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html)
- [SSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)
- [gRPC Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/gRPC_Security_Cheat_Sheet.html)
- [govulncheck Tutorial](https://go.dev/doc/tutorial/govulncheck)

## Вопросы

- какие угрозы для backend API самые вероятные именно в твоем домене;
- где хранятся secrets и кто имеет к ним доступ;
- как защитить сервис от SSRF, replay и brute force атак;
- почему rate limiting относится не только к performance, но и к security;
- как встроить dependency scanning и vuln management в обычный pipeline;
- какие security controls должны быть стандартом еще до первого релиза.
