# Stage 1: Build C++ shared library
FROM ubuntu:22.04 AS cpp-builder
RUN apt-get update && apt-get install -y \
    cmake build-essential git \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /build
COPY CMakeLists.txt ./
COPY include/ include/
COPY src/ src/
RUN cmake -B build -S . -DCMAKE_BUILD_TYPE=Release -DBUILD_TESTS=OFF \
    && cmake --build build --target riskcalc -j"$(nproc)"

# Stage 2: Build Go binary (CGo)
# ${SRCDIR} for internal/riskcalc/riskcalc.go resolves to /app/internal/riskcalc/
# so ${SRCDIR}/../../include = /app/include and ${SRCDIR}/../../build = /app/build
FROM golang:1.26-bookworm AS go-builder
RUN apt-get update && apt-get install -y build-essential \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=cpp-builder /build/include ./include
COPY --from=cpp-builder /build/build/libriskcalc.so ./build/libriskcalc.so
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /risk-svc ./cmd/server

# Stage 3: Runtime
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=cpp-builder /build/build/libriskcalc.so /app/build/libriskcalc.so
# Register /app/build so the dynamic linker finds libriskcalc.so
RUN echo "/app/build" > /etc/ld.so.conf.d/riskcalc.conf && ldconfig
COPY --from=go-builder /risk-svc /usr/local/bin/risk-svc
ENV API_ADDR=:8081 \
    METRICS_ADDR=:9103
CMD ["risk-svc"]
