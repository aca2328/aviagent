# Multi-stage build for VMware Avi LLM Agent
# Stage 1: Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o vmware-avi-llm-agent \
    ./cmd/server

# Stage 2: Runtime stage
FROM scratch AS runtime

# Copy ca-certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary
COPY --from=builder /app/vmware-avi-llm-agent /vmware-avi-llm-agent

# Copy web assets
COPY --from=builder /app/web /web

# Create a non-root user (we need to use a regular Linux image for this)
FROM alpine:latest AS final

# Install ca-certificates and create user
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 1000 appgroup \
    && adduser -D -s /bin/sh -u 1000 -G appgroup appuser

# Copy binary from builder
COPY --from=builder /app/vmware-avi-llm-agent /usr/local/bin/vmware-avi-llm-agent

# Copy web assets
COPY --from=builder /app/web /web

# Set permissions
RUN chmod +x /usr/local/bin/vmware-avi-llm-agent \
    && chown -R appuser:appgroup /web

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

# Set environment variables
ENV GIN_MODE=release
ENV TZ=UTC

# Run the binary
ENTRYPOINT ["/usr/local/bin/vmware-avi-llm-agent"]
CMD ["-config", "/etc/vmware-avi-llm-agent/config.yaml"]