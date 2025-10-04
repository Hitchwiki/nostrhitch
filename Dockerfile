# Build stage
FROM golang:1.21-alpine AS builder

# Install SQLite dependencies
RUN apk add --no-cache sqlite-dev gcc musl-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with CGO enabled for SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o nostrhitch-daemon .

# Final stage
FROM alpine:latest

# Install ca-certificates and SQLite for HTTPS requests and database
RUN apk --no-cache add ca-certificates tzdata sqlite

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/nostrhitch-daemon .

# Create necessary directories
RUN mkdir -p logs hitchmap-dumps

# Expose port (if needed for health checks)
EXPOSE 8080

# Run the daemon
CMD ["./nostrhitch-daemon"]
