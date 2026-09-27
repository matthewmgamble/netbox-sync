# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o netbox-sync ./cmd/sync

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 netbox && \
    adduser -D -u 1000 -G netbox netbox

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/netbox-sync /app/netbox-sync

# Create directories for config and data
RUN mkdir -p /config /data && \
    chown -R netbox:netbox /app /config /data

# Switch to non-root user
USER netbox

# Set default config path
ENV CONFIG_FILE=/config/config.yaml

# Run the sync
ENTRYPOINT ["/app/netbox-sync"]
CMD ["-config", "/config/config.yaml"]
