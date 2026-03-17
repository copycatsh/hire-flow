from fastapi import FastAPI

app = FastAPI(title="ai-matching")


@app.get("/health")
async def health():
    return {"status": "ok"}
