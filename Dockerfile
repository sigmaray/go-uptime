FROM golang:1.25.7-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o go-uptime .

FROM alpine:3.21.3

RUN apk add --no-cache ca-certificates wget \
    && adduser -D -u 1000 appuser

WORKDIR /app
COPY --from=builder /app/go-uptime .
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

USER appuser
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=5s --retries=5 \
    CMD wget -qO- http://127.0.0.1:8080/health || exit 1

CMD ["./go-uptime", "server"]
