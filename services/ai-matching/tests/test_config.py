from src.config import Settings


def test_settings_defaults():
    settings = Settings(
        nats_url="nats://localhost:4222",
        qdrant_host="localhost",
    )
    assert settings.nats_url == "nats://localhost:4222"
    assert settings.qdrant_host == "localhost"
    assert settings.qdrant_port == 6333
    assert settings.embedding_model == "all-MiniLM-L6-v2"
    assert settings.embedding_dim == 384
    assert settings.consumer_batch_size == 10
    assert settings.consumer_max_deliver == 5
    assert settings.batch_concurrency == 3


def test_settings_from_env(monkeypatch):
    monkeypatch.setenv("NATS_URL", "nats://nats:4222")
    monkeypatch.setenv("QDRANT_HOST", "qdrant")
    monkeypatch.setenv("QDRANT_PORT", "7333")
    monkeypatch.setenv("EMBEDDING_MODEL", "custom-model")

    settings = Settings()
    assert settings.nats_url == "nats://nats:4222"
    assert settings.qdrant_host == "qdrant"
    assert settings.qdrant_port == 7333
    assert settings.embedding_model == "custom-model"
