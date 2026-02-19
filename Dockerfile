# Multi-stage build for minimal image size
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o fidex-node ./cmd/fidex-node

# Final stage - minimal runtime image
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 fidex && \
    adduser -D -u 1000 -G fidex fidex

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/fidex-node .

# Create necessary directories
RUN mkdir -p /app/fidex/outbox /app/fidex/archive && \
    chown -R fidex:fidex /app

# Switch to non-root user
USER fidex

# Expose ports
EXPOSE 8080 8443

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8443/health || exit 1

# Run the application
CMD ["./fidex-node"]
