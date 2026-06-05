# LLM и AI Integration

Интеграция Large Language Models (LLM) в backend-сервисы становится "новой грамотностью" 2026 года. Здесь — практические темы для backend-разработчика: как работать с LLM API, как делать reliable AI-features, как строить RAG и vector search.

Раздел НЕ про обучение моделей или ML-инженерию. Он про **использование готовых LLM** (OpenAI, Anthropic, локальные через Ollama) в production-сервисах.

## Структура

- **rag/** — RAG (Retrieval-Augmented Generation), векторные БД, чанкинг, embeddings
- (planned) **api-integration/** — OpenAI/Anthropic API, streaming, function calling, токеномика
- (planned) **prompt-engineering/** — system prompts, context management, prompt caching
- (planned) **reliability/** — fallback при отказе провайдера, retries, rate limits, latency

## Что должен знать senior backend в 2026

**Базовое:**
- что такое LLM с точки зрения API (модель, токены, context window, temperature)
- разница между chat completion, embedding, function calling
- стоимость и латенси разных моделей

**Архитектурное:**
- когда LLM подходит, когда нет (примеры anti-patterns)
- RAG vs fine-tuning vs prompt engineering — когда что
- streaming vs batch generation
- как тестировать AI-features

**Production:**
- handling провайдер outages (fallback, circuit breaker)
- управление токенами и стоимостью
- prompt injection защита
- логирование и debug AI-flow

**Security:**
- prompt injection (атака пользовательских вводов на LLM)
- утечка контекста через chained prompts
- secret management для API keys

## Что делает RAG особенно важным

RAG — это сейчас **самый распространённый паттерн** AI-интеграции в backend. Когда компания говорит "хочу chat с нашими доками" или "AI ассистент по нашей базе знаний" — это почти всегда RAG.

Преимущества:
- Работает с **актуальными** данными (не датой обучения модели)
- Работает с **приватными** данными компании
- Дешевле и быстрее чем fine-tuning
- Можно цитировать источники

Поэтому начинаем с RAG — см. [rag/README.md](./rag/README.md).

## Внешние ссылки

- [OpenAI API Reference](https://platform.openai.com/docs/api-reference)
- [Anthropic API Reference](https://docs.anthropic.com/en/api/)
- [LangChain Go](https://github.com/tmc/langchaingo) — Go-обёртка для AI workflows
- [pgvector](https://github.com/pgvector/pgvector) — векторный поиск в PostgreSQL
- [Qdrant](https://qdrant.tech/), [Weaviate](https://weaviate.io/) — специализированные vector БД
