FROM golang:1.25.6 AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o consumer ./consumer

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=builder /app/consumer .

CMD ["./consumer"]