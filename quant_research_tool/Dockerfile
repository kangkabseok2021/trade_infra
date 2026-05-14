# ── Stage 1: Build Rust extension ────────────────────────────────────────────
FROM rust:1.78-slim AS rust-builder

RUN apt-get update && apt-get install -y \
    cmake ninja-build python3 python3-dev python3-pip curl \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir uv

WORKDIR /app
COPY Cargo.toml pyproject.toml CMakeLists.txt ./
COPY .cargo/ ./.cargo/
COPY src/ ./src/
COPY backtester/ ./backtester/
COPY strategies/ ./strategies/

RUN uv pip install --system scikit-build-core cmake
RUN uv pip install --system -e . --no-build-isolation

# ── Stage 2: Build React frontend ────────────────────────────────────────────
FROM node:20-slim AS node-builder

WORKDIR /frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ── Stage 3: Production image ─────────────────────────────────────────────────
FROM python:3.12-slim AS final

RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*
RUN pip install --no-cache-dir uv

WORKDIR /app

# Copy built .so and Python source
COPY --from=rust-builder /app/backtester/ ./backtester/
COPY --from=rust-builder /app/strategies/ ./strategies/

# Copy compiled React app into FastAPI's static dir
COPY --from=node-builder /frontend/dist/ ./backtester/static/

# Install only runtime Python deps (no Rust build needed)
RUN uv pip install --system fastapi uvicorn yfinance

EXPOSE 8000
CMD ["uvicorn", "backtester.api:app", "--host", "0.0.0.0", "--port", "8000"]
