# Stage 1: Builder
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/tiangong ./cmd/tiangong

# Stage 2: Runtime
FROM alpine:3.20

COPY --from=builder /app/bin/tiangong /app/tiangong

EXPOSE 8080

ENTRYPOINT ["/app/tiangong"]
