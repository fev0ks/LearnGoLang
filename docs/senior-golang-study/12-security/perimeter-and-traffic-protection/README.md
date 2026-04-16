# Perimeter And Traffic Protection

Этот подпакет про защиту внешнего периметра:
- `DDoS protection`
- `WAF`
- coarse traffic filtering
- bot mitigation

Как читать:
- сначала понять, что именно считается perimeter-защитой;
- затем разобрать, чем `DDoS protection` отличается от обычного rate limiting;
- после этого уже связывать это с edge, CDN, gateway и ingress.

Материалы:
- [DDoS Protection](./ddos-protection.md)

Что важно уметь объяснить:
- почему `DDoS protection` ставят раньше приложения;
- чем volumetric атака отличается от API abuse;
- почему backend не должен быть первой линией защиты;
- как perimeter controls сочетаются с app-level rate limiting и auth.
