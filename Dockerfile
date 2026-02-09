# Frontend build stage
FROM oven/bun:latest AS frontend

WORKDIR /app/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ .
RUN bun run build

# Backend build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o control-plane ./cmd

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/control-plane .
COPY --from=builder /app/scripts/init.sql ./scripts/
COPY --from=frontend /app/web/dist ./dist/

EXPOSE 9900 9901

ENTRYPOINT ["./control-plane"]
CMD ["-l", "0.0.0.0:9900"]
