# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk --no-cache add ca-certificates git

WORKDIR /app

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY internal/ internal/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o tracker -ldflags="-w -s" ./cmd/tracker

# Final stage - using debian for Chrome support
FROM debian:bookworm-slim

# Install Chrome and dependencies for browser automation
# Also install ca-certificates for HTTPS connections
RUN apt-get update && apt-get install -y \
    chromium \
    chromium-driver \
    ca-certificates \
    fonts-liberation \
    libasound2 \
    libatk-bridge2.0-0 \
    libatk1.0-0 \
    libcups2 \
    libdbus-1-3 \
    libgdk-pixbuf2.0-0 \
    libglib2.0-0 \
    libgtk-3-0 \
    libnspr4 \
    libnss3 \
    libx11-6 \
    libxcomposite1 \
    libxdamage1 \
    libxext6 \
    libxfixes3 \
    libxrandr2 \
    xdg-utils \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user for security
RUN useradd -m -u 1000 tracker

WORKDIR /home/tracker

# Copy binary from builder and set ownership before switching users
COPY --from=builder /app/tracker ./tracker
RUN chmod +x ./tracker && \
    chown tracker:tracker ./tracker

# Create directory for state files
RUN mkdir -p /tmp/case-tracker-states && \
    chown -R tracker:tracker /tmp/case-tracker-states

# Switch to non-root user
USER tracker

# Set Chrome path for chromedp
ENV CHROME_BIN=/usr/bin/chromium
ENV CHROMEDP_DISABLE_GPU=true
ENV STATE_FILE_DIR=/tmp/case-tracker-states/

# Health check
HEALTHCHECK --interval=5m --timeout=10s --start-period=30s \
    CMD pgrep -f tracker || exit 1

CMD ["./tracker"]
