# Service To Service TLS

Этот подпакет про то, как устроено шифрование трафика между edge, gateway и внутренними сервисами.

Как читать:
- сначала понять разницу между `TLS termination`, `re-encryption` и `mTLS`;
- затем понять, зачем нужны внутренние сертификаты и кто их выпускает;
- после этого уже переходить к service mesh или platform-specific реализации.

Материалы:
- [TLS Termination, Re-encryption And mTLS](./tls-termination-re-encryption-and-mtls.md)

Что важно уметь объяснить:
- почему `TLS termination at edge` не всегда означает plain HTTP внутри;
- когда внутренним сервисам тоже нужны сертификаты;
- чем `re-encryption` отличается от `mTLS`;
- зачем в zero-trust среде шифровать east-west traffic;
- почему “внутренняя сеть безопасна по умолчанию” часто плохое допущение.
