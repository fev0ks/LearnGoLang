# OWASP Top 10

Самые распространённые web-уязвимости по версии OWASP. Покрывает то, как атакующий получает доступ к данным или системам через типичные ошибки в приложении.

## Материалы

- [01. SQL Injection](./01-sql-injection.md) — параметризованные запросы, ORM pitfalls, динамические идентификаторы, RLS, известные инциденты (Heartland, TalkTalk, Yahoo)
- [02. XSS (Cross-Site Scripting)](./02-xss.md) — три типа XSS, html/template в Go, bluemonday, CSP, известные инциденты (Samy worm, Magecart, British Airways)
- [03. SSRF (Server-Side Request Forgery)](./03-ssrf.md) — cloud metadata, IMDSv2, allowlist URL, DNS rebinding, безопасный HTTP-клиент, известные инциденты (Capital One, Shopify)

## Что должен знать senior

- почему параметризация — единственная надёжная защита от SQL injection
- разница между stored, reflected, DOM-based XSS
- почему `html/template` безопасен по умолчанию, а `text/template` — нет
- что такое cloud metadata service и почему через SSRF её можно достать
- роль IMDSv2 и hop-limit в облачных deployment
- defense-in-depth: WAF, CSP, RLS, network isolation

## Связанные разделы

- [Authentication](../authentication/) — auth-bypass через SQL injection, session hijacking через XSS
- [CORS и CSRF](../cors-and-browser-api-security/) — другие browser-side угрозы
- [Perimeter Protection](../perimeter-and-traffic-protection/) — WAF и rate limiting

## Внешние ссылки

- [OWASP Top 10 (2021)](https://owasp.org/www-project-top-ten/)
- [OWASP Cheat Sheet Series](https://cheatsheetseries.owasp.org/)
- [PortSwigger Web Security Academy](https://portswigger.net/web-security) — бесплатные лабы для практики
