# Multi-stage build for VMware Avi LLM Agent
# Stage 1: Build stage
FROM golang:1.23-alpine AS builder

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

# Stage 2: Runtime stage
FROM alpine:latest AS runtime

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata wget bash \
    && addgroup -g 1000 appgroup \
    && adduser -D -s /bin/sh -u 1000 -G appgroup appuser

# Copy ca-certificates and timezone data from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary
COPY --from=builder /app/aviagent /usr/local/bin/aviagent

# Copy web assets
COPY --from=builder /app/web /web

# Set permissions
RUN chmod +x /usr/local/bin/aviagent \
    && chown -R appuser:appgroup /web \
    && chown appuser:appgroup /usr/local/bin/aviagent

# Create config directory
RUN mkdir -p /etc/aviagent && chown appuser:appgroup /etc/aviagent

# Switch to non-root user
USER appuser

# Set working directory
WORKDIR /web

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

# Set environment variables
ENV GIN_MODE=release
ENV TZ=UTC
ENV CONFIG_DIR=/etc/aviagent

# Run the binary
ENTRYPOINT ["/usr/local/bin/aviagent"]
CMD ["-config", "/etc/aviagent/config.yaml"]