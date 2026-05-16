# Stage 1: Build
FROM rust:latest AS builder
RUN apt-get update && apt-get install -y \
    cmake build-essential git libpq-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /build
COPY . .
RUN RUSTC_REAL="$(rustup which rustc)" \
    && cmake -B build -S . -DCMAKE_BUILD_TYPE=Release -DBUILD_TESTS=OFF \
        -DRust_COMPILER="${RUSTC_REAL}" \
    && cmake --build build --target market_data_svc -j"$(nproc)"

# Stage 2: Runtime
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y libpq5 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /build/build/market_data_svc /usr/local/bin/
COPY --from=builder /build/build/libmarketdata.so /usr/local/lib/
RUN ldconfig
ENV NODE_NAME=HB_NORTH \
    BASE_LMP=45.0 \
    VOLATILITY=5.0 \
    INTERVAL_MS=1000 \
    METRICS_PORT=9101
CMD ["market_data_svc"]
