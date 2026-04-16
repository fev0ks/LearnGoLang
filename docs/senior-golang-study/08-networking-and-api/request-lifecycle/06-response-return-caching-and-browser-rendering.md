# Response Return Caching And Browser Rendering

После того как backend сформировал ответ, работа не заканчивается. Ответ должен пройти обратный путь и быть обработан браузером.

## Содержание

- [1. Ответ идет обратно через те же слои](#1-ответ-идет-обратно-через-те-же-слои)
- [2. Cache headers решают дальнейшее поведение](#2-cache-headers-решают-дальнейшее-поведение)
- [3. Browser получает HTML и начинает парсинг](#3-browser-получает-html-и-начинает-парсинг)
- [4. Дополнительные запросы](#4-дополнительные-запросы)
- [5. Browser рендерит страницу](#5-browser-рендерит-страницу)
- [Где тут бывают проблемы](#где-тут-бывают-проблемы)
- [Что важно помнить](#что-важно-помнить)
- [Что могут спросить на интервью](#что-могут-спросить-на-интервью)

## 1. Ответ идет обратно через те же слои

Обычно response проходит обратно через:
- application server;
- reverse proxy или ingress;
- load balancer;
- CDN;
- browser network stack.

На каждом уровне возможны:
- header rewrite;
- compression;
- caching;
- logging;
- timeout или truncation problems.

## 2. Cache headers решают дальнейшее поведение

Response headers вроде этих сильно меняют картину:
- `Cache-Control`
- `ETag`
- `Last-Modified`
- `Vary`
- `Set-Cookie`

Если ответ кэшируемый:
- browser может использовать его повторно;
- CDN может не дергать origin;
- следующие запросы могут быть conditional, а не full fetch.

Если ответ персонализированный:
- caching обычно становится сложнее и опаснее.

## 3. Browser получает HTML и начинает парсинг

Когда документ пришел:
- браузер начинает строить DOM;
- парсит HTML;
- обнаруживает ссылки на CSS, JS, изображения, шрифты;
- инициирует новые requests на subresources.

То есть один ввод `google.com` быстро превращается во множество сетевых запросов.

## 4. Дополнительные запросы

После первого HTML обычно догружаются:
- CSS;
- JS bundles;
- images;
- fonts;
- API calls за данными;
- analytics, telemetry и другие third-party resources.

У каждого такого запроса свой маршрут:
- возможно reuse существующего соединения;
- возможно новый DNS lookup;
- возможно новый cache hit.

## 5. Browser рендерит страницу

Дальше браузер строит:
- DOM;
- CSSOM;
- render tree;
- layout;
- paint;
- compositing.

Важно:
- "ответ сервера пришел" не равно "страница уже видна пользователю";
- large JS bundle или render-blocking CSS могут быть bottleneck даже при быстром backend.

## Где тут бывают проблемы

- HTML пришел быстро, но JS тяжелый;
- CDN отдает старую статику;
- неправильные cache headers;
- слишком много blocking resources;
- browser main thread занят;
- огромный payload тормозит parse и render.

## Что важно помнить

Для user-perceived latency важен не только TTFB:
- еще важны resource waterfall;
- render blocking;
- hydration;
- работа main thread в браузере.

## Что могут спросить на интервью

- чем TTFB отличается от полной загрузки страницы;
- зачем нужны `ETag` и `Cache-Control`;
- почему HTML response порождает дополнительные requests;
- почему "backend быстрый" не гарантирует "страница быстрая".
