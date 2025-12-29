# Build stage - Multi-stage build for smaller image
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /app/missav-bot \
    ./cmd/bot/main.go

# Runtime stage - Minimal image with Chrome for headless browser
FROM alpine:3.19

# Install Chrome and dependencies for headless browser (rod)
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    chromium \
    chromium-chromedriver \
    nss \
    freetype \
    harfbuzz \
    ttf-freefont \
    && rm -rf /var/cache/apk/*

# Set timezone
ENV TZ=Asia/Shanghai

# Set Chrome path for rod
ENV CHROME_PATH=/usr/bin/chromium-browser

# Create non-root user for security
RUN adduser -D -g '' appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/missav-bot /app/missav-bot

# Set ownership
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Health check endpoint
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

# Expose HTTP server port
EXPOSE 8080

# Run the binary
ENTRYPOINT ["/app/missav-bot"]
