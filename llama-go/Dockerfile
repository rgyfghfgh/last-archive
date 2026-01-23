FROM golang:1.21-bookworm as builder

WORKDIR /app

# Install build dependencies
RUN apt-get update && apt-get install -y \
    gcc \
    g++ \
    git \
    make \
    && rm -rf /var/lib/apt/lists/*

# Copy go.mod and go.sum
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build with optimizations
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o llm-server .

# ============================================================================
# Runtime stage - Use Debian Slim for better compatibility with Ollama
# ============================================================================
FROM debian:bookworm-slim

WORKDIR /app

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    curl \
    zstd \
    bash \
    pciutils \
    && rm -rf /var/lib/apt/lists/*

# Pre-install Ollama during build
RUN curl -fsSL https://ollama.com/install.sh | sh

# Copy binary from builder
COPY --from=builder /app/llm-server /app/llm-server

# Create models directory
RUN mkdir -p /root/.ollama

# Expose port
EXPOSE 1410

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=60s \
  CMD curl -f http://localhost:1410/health || exit 1

# Run
CMD ["/app/llm-server"]
