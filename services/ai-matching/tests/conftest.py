import pytest

from src.config import Settings
from src.embedding import FakeEmbedder


@pytest.fixture
def settings() -> Settings:
    return Settings(
        nats_url="nats://localhost:4222",
        qdrant_host="localhost",
    )


@pytest.fixture
def fake_embedder() -> FakeEmbedder:
    return FakeEmbedder(dim=384)
