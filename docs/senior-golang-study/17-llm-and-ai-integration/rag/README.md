# RAG — Retrieval-Augmented Generation

RAG — паттерн, при котором LLM **сначала ищет** релевантную информацию во внешнем источнике (БД, документы), а потом **генерирует** ответ на основе найденного. Это решение трёх главных ограничений "голого" LLM: устаревшие знания, отсутствие приватных данных, hallucinations.

## Материалы

- [01. RAG Fundamentals](./01-rag-fundamentals.md) — что это, зачем, как работает, базовый pipeline, простой Go-пример, когда нужен и когда нет
- (planned) **02. Embeddings** — что это, какие модели, размерности, OpenAI vs open-source
- (planned) **03. Vector databases** — pgvector, Qdrant, Weaviate — сравнение, когда что выбирать
- (planned) **04. Chunking strategies** — как разбивать документы, влияние на качество ответов
- (planned) **05. Hybrid search** — vector + keyword (BM25), reranking
- (planned) **06. RAG pitfalls** — stale data, prompt injection через документы, evaluation, hallucinations

## Что должен знать senior

- зачем RAG (vs fine-tuning vs plain LLM)
- что такое embedding и similarity search
- как выглядит RAG pipeline в production
- основные подводные камни (chunking, stale data, prompt injection)
- как оценивать качество RAG-системы
- стоимость и латенси типичной RAG-системы
