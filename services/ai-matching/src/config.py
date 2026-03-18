from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    nats_url: str = "nats://localhost:4222"
    qdrant_host: str = "localhost"
    qdrant_port: int = 6333
    embedding_model: str = "all-MiniLM-L6-v2"
    embedding_dim: int = 384
    consumer_batch_size: int = 10
    consumer_max_deliver: int = 5
    batch_concurrency: int = 3
