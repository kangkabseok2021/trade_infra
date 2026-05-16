# Stage 1: Build
FROM golang:1.22-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /order-svc ./cmd/server

# Stage 2: Runtime
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /order-svc /usr/local/bin/order-svc
ENV API_ADDR=:8080 \
    METRICS_ADDR=:9102
CMD ["order-svc"]
