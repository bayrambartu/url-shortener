# -------------------------
# 1. Build Stage
# -------------------------
FROM golang:1.25.6 AS builder
# Container içindeki çalışma dizini
WORKDIR /app

# Önce mod dosyalarını kopyala
COPY go.mod go.sum ./

# Bağımlılıkları indir
RUN go mod download

# Geri kalan kaynak kodlarını kopyala
COPY . .

# Binary oluştur
RUN CGO_ENABLED=0 GOOS=linux go build -o app .

# -------------------------
# 2. Runtime Stage
# -------------------------
FROM debian:bookworm-slim

# Çalışma dizini
WORKDIR /app

# Build aşamasındaki binary'yi kopyala
COPY --from=builder /app/app .

# Uygulamanın dinlediği port
EXPOSE 8080

# Container başladığında çalışacak komut
CMD ["./app"]