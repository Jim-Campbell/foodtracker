FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=local go build -ldflags="-s -w" -o food ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/food .
COPY --from=builder /app/internal/db/migrations ./migrations
COPY --from=builder /app/pwa ./pwa

ENV MIGRATIONS_DIR=migrations
ENV PWA_DIR=pwa
EXPOSE 8082
CMD ["./food"]
