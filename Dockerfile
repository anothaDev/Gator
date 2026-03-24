FROM golang:1.25-alpine AS go-builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -tags release -o /usr/local/bin/gator .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app

COPY --from=go-builder /usr/local/bin/gator /usr/local/bin/gator

ENV PORT=8080
ENV DATABASE_PATH=/data/gator.db

VOLUME ["/data"]

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD sh -c 'wget -qO- "http://127.0.0.1:${PORT}/health" >/dev/null || exit 1'

ENTRYPOINT ["/usr/local/bin/gator"]
