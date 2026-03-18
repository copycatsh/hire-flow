import asyncio
import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI
from qdrant_client import AsyncQdrantClient

from src.api import create_router
from src.config import Settings
from src.consumer import NATSConsumer
from src.embedding import SentenceTransformerEmbedder
from src.qdrant_store import QdrantStore

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)
logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI):
    settings: Settings = app.state.settings

    # Load embedding model
    embedder = SentenceTransformerEmbedder(settings.embedding_model)
    app.state.embedder = embedder

    # Connect to Qdrant
    qdrant_client = AsyncQdrantClient(
        host=settings.qdrant_host,
        port=settings.qdrant_port,
    )
    qdrant_store = QdrantStore(client=qdrant_client, embedding_dim=settings.embedding_dim)
    await qdrant_store.ensure_collections()
    app.state.qdrant_store = qdrant_store

    # Start NATS consumer as background task
    shutdown_event = asyncio.Event()
    consumer = NATSConsumer(embedder=embedder, qdrant_store=qdrant_store)
    consumer_task = asyncio.create_task(
        consumer.run(
            nats_url=settings.nats_url,
            batch_size=settings.consumer_batch_size,
            max_deliver=settings.consumer_max_deliver,
            shutdown_event=shutdown_event,
        )
    )
    logger.info("AI Matching service started")

    yield

    # Shutdown
    logger.info("Shutting down...")
    shutdown_event.set()
    consumer_task.cancel()
    try:
        await consumer_task
    except asyncio.CancelledError:
        pass
    await qdrant_client.close()
    logger.info("AI Matching service stopped")


def create_app(settings: Settings | None = None) -> FastAPI:
    if settings is None:
        settings = Settings()

    app = FastAPI(
        title="ai-matching",
        lifespan=lifespan,
    )
    app.state.settings = settings

    router = create_router()
    app.include_router(router, prefix="/api/v1")

    @app.get("/health")
    async def health():
        return {"status": "ok"}

    return app


app = create_app()
