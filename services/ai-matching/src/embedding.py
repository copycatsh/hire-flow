import asyncio
import hashlib
import logging
from typing import Protocol

import numpy as np

from src.models import JobEvent, ProfileEvent

logger = logging.getLogger(__name__)


def build_job_text(event: JobEvent) -> str:
    parts = [f'Job: "{event.title}".']
    if event.description:
        parts.append(f"Description: {event.description}.")
    return " ".join(parts)


def build_profile_text(event: ProfileEvent) -> str:
    parts = [f'Profile: "{event.full_name}".']
    if event.bio:
        parts.append(f"Bio: {event.bio}.")
    if event.skills:
        skill_names = ", ".join(s.name for s in event.skills)
        parts.append(f"Skills: {skill_names}.")
    return " ".join(parts)


class Embedder(Protocol):
    def embed(self, text: str) -> list[float]: ...


class SentenceTransformerEmbedder:
    def __init__(self, model_name: str = "all-MiniLM-L6-v2") -> None:
        from sentence_transformers import SentenceTransformer

        logger.info("Loading embedding model: %s", model_name)
        self._model = SentenceTransformer(model_name)
        logger.info("Model loaded successfully")

    def embed(self, text: str) -> list[float]:
        vector = self._model.encode(text, normalize_embeddings=True)
        return vector.tolist()


class FakeEmbedder:
    def __init__(self, dim: int = 384) -> None:
        self._dim = dim

    def embed(self, text: str) -> list[float]:
        h = hashlib.sha256(text.encode()).digest()
        rng = np.random.default_rng(seed=int.from_bytes(h[:8], "big"))
        vector = rng.standard_normal(self._dim)
        norm = np.linalg.norm(vector)
        if norm > 0:
            vector = vector / norm
        return vector.tolist()


async def embed_async(embedder: Embedder, text: str) -> list[float]:
    return await asyncio.to_thread(embedder.embed, text)
