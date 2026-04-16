# CORS And Browser API Security

Этот подпакет про `CORS` и смежные browser-facing security моменты для API.

Как читать:
- сначала понять, что именно защищает `CORS` и чего он не защищает;
- потом разобраться, как работает preflight;
- после этого уже смотреть, где такую политику обычно настраивают.

Материалы:
- [CORS Basics And Where To Configure It](./cors-basics-and-where-to-configure-it.md)
- [CORS Middleware Example](./cors-middleware-example.md)

Что важно уметь объяснить:
- что `CORS` — это часть browser security model, а не perimeter defense;
- почему `CORS` не заменяет auth, CSRF protection или rate limiting;
- что такое preflight `OPTIONS`;
- когда `CORS` стоит настраивать на gateway/proxy, а когда в приложении.
