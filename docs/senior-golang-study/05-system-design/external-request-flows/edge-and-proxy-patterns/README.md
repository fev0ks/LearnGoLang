# Edge And Proxy Patterns

Этот подпакет нужен, чтобы не путать между собой:
- CDN;
- edge provider;
- reverse proxy;
- load balancer;
- ingress;
- API gateway.

Материалы:
- [01 Edge Roles And Terms](./01-edge-roles-and-terms.md)
- [02 Edge Tools Comparison Table](./02-edge-tools-comparison-table.md)
- [03 Where Nginx Can Stand](./03-where-nginx-can-stand.md)
- [04 Typical Edge Topologies](./04-typical-edge-topologies.md)

Что важно понять:
- `edge` это не конкретный продукт, а роль на внешнем периметре;
- один и тот же инструмент может играть разные роли в разных системах;
- `Cloudflare` и `nginx` не прямые аналоги, хотя оба могут стоять "перед приложением";
- system design схема должна показывать, кто именно первый принимает внешний трафик и кто что делает дальше.
