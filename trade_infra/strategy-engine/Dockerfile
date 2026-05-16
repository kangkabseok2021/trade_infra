FROM golang:1.26-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /strategy-engine ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /strategy-engine /usr/local/bin/strategy-engine
ENV ORDER_SVC_URL=http://order-svc:8080 \
    POSITION_LIMIT_MW=40 \
    COOLDOWN_SECS=30 \
    METRICS_ADDR=:9104
CMD ["strategy-engine"]
