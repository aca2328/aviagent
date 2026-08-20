# Multi-stage build for VMware Avi LLM Agent with Python support
# Stage 1: Build Go components
FROM golang:1.23-alpine AS go-builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod file
COPY go.mod ./

# Initialize go.sum if it doesn't exist and download dependencies
RUN if [ ! -f go.sum ]; then go mod tidy; fi && go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o aviagent \
    .

# Stage 2: Build Python components
FROM python:3.11-slim AS python-builder

# Set working directory
WORKDIR /app

# Install Python dependencies
COPY python_mistral/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy Python source code
COPY python_mistral/ python_mistral/

# Install additional dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Stage 3: Runtime stage
FROM python:3.11-slim AS runtime

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Copy from Go builder
COPY --from=go-builder /app/aviagent /usr/local/bin/aviagent

# Copy from Python builder
COPY --from=python-builder /app/python_mistral /app/python_mistral
COPY --from=python-builder /usr/local/lib/python3.11/site-packages /usr/local/lib/python3.11/site-packages

# Copy web assets
COPY web /web

# Set permissions
RUN chmod +x /usr/local/bin/aviagent \
    && mkdir -p /etc/aviagent \
    && chown -R 1000:1000 /web /etc/aviagent

# Create non-root user
RUN addgroup --gid 1000 appgroup && adduser --uid 1000 --gid 1000 --disabled-password --gecos "" appuser

# Switch to non-root user
USER appuser

# Set working directory
WORKDIR /web

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 -O /dev/null http://localhost:8080/api/health || exit 1

# Set environment variables
ENV PYTHONUNBUFFERED=1
ENV PYTHONDONTWRITEBYTECODE=1
ENV GIN_MODE=release
ENV TZ=UTC
ENV CONFIG_DIR=/etc/aviagent

# Run the Go binary
ENTRYPOINT ["/usr/local/bin/aviagent"]
CMD ["-config", "/etc/aviagent/config.yaml"]