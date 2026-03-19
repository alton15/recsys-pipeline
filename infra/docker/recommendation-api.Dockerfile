### Builder stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

# Disable go.work — build with module replace directives only.
ENV GOWORK=off

# Copy module files first for layer caching.
COPY shared/go/go.mod shared/go/go.sum* shared/go/
COPY services/recommendation-api/go.mod services/recommendation-api/go.sum* services/recommendation-api/

WORKDIR /build/services/recommendation-api
RUN go mod download

WORKDIR /build

# Copy source code.
COPY shared/go/ shared/go/
COPY services/recommendation-api/ services/recommendation-api/

WORKDIR /build/services/recommendation-api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /recommendation-api ./cmd/server

### Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /recommendation-api /usr/local/bin/recommendation-api

EXPOSE 8090 2112

ENTRYPOINT ["recommendation-api"]
