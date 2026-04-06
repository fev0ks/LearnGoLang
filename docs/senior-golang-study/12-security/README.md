# Security

Сюда собирай практические темы по безопасности backend-сервисов.

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
