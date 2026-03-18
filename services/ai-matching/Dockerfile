FROM python:3.13-slim

WORKDIR /app

# System deps for numpy/torch
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY pyproject.toml .

# Install PyTorch CPU-only first (much smaller than full CUDA build),
# then install the rest of the dependencies.
RUN pip install --no-cache-dir --timeout=120 \
    torch --index-url https://download.pytorch.org/whl/cpu \
    && pip install --no-cache-dir --timeout=120 .

COPY src/ src/

EXPOSE 8002

CMD ["uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8002"]
