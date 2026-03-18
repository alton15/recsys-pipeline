### Builder stage
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

# Copy go.work and module files first for layer caching.
COPY go.work go.work
COPY shared/go/go.mod shared/go/go.sum* shared/go/
COPY services/event-collector/go.mod services/event-collector/go.sum* services/event-collector/

WORKDIR /build/services/event-collector
RUN go mod download

WORKDIR /build

# Copy source code.
COPY shared/go/ shared/go/
COPY services/event-collector/ services/event-collector/

WORKDIR /build/services/event-collector
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /event-collector ./cmd/server

### Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /event-collector /usr/local/bin/event-collector

EXPOSE 8080 2112

ENTRYPOINT ["event-collector"]
