# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o control-plane ./cmd

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/control-plane .
COPY --from=builder /app/scripts/init.sql ./scripts/
COPY --from=builder /app/index.html .
COPY --from=builder /app/index.js .

EXPOSE 9900

ENTRYPOINT ["./control-plane"]
CMD ["-l", "0.0.0.0:9900"]
