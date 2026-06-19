# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git gcc musl-dev sqlite-dev

WORKDIR /app

# Copy go mod files
COPY server/go.mod ./
RUN go mod tidy && go mod download

# Copy source code
COPY server/ .

# Download dependencies and build
RUN go mod tidy
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o meshvpn-server ./cmd/server/main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates sqlite-libs libc6-compat

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/meshvpn-server .

# Create data directory
RUN mkdir -p /data

# Expose ports
EXPOSE 8080 3478/udp

# Volume for persistent data
VOLUME ["/data"]

# Run
CMD ["./meshvpn-server", "-http=:8080", "-db=/data/meshvpn.db", "-stun=:3478"]
