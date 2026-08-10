# Stage 1: Build
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Instalar ca-certificates e tzdata para o build se necessário
RUN apk add --no-cache ca-certificates tzdata

# Cache de dependências
COPY go.mod go.sum ./
RUN go mod download

# Copiar código fonte
COPY . .

# Build estático (sem CGO)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /app/bin/coord-server \
    ./cmd/server

# Stage 2: Runtime
FROM alpine:3.20

LABEL org.opencontainers.image.title="coord-server" \
      org.opencontainers.image.description="Coordination server and registry for the Goy mesh network" \
      org.opencontainers.image.vendor="Goy Co" \
      org.opencontainers.image.licenses="Goy Source Available"

RUN apk add --no-cache ca-certificates tzdata

# Criar utilizador e grupo não-root
RUN addgroup -S coord && adduser -S coord -G coord

WORKDIR /app

# Diretoria para persistência de dados SQLite
RUN mkdir -p /app/data && chown -R coord:coord /app

# Copiar executável
COPY --from=builder /app/bin/coord-server /app/coord-server

USER coord

EXPOSE 8080

VOLUME ["/app/data"]

ENTRYPOINT ["/app/coord-server"]
CMD ["--config", "/app/config.toml"]
