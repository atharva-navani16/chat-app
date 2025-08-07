# Use official Golang image as a builder
FROM golang:1.21-alpine AS builder

# Set working directory
WORKDIR /app

# Install dependencies
RUN apk update && apk add --no-cache git

# Copy go mod files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the Go binary
RUN go build -o chat-app .

# Final image
FROM alpine:latest

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/chat-app .

# Expose port
EXPOSE 8000

# Run the binary
CMD ["./chat-app"]
