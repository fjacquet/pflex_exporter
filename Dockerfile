# Stage 1: Build
FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static build; querySelectedStatistics.json is embedded via go:embed.
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o pflex_exporter .

# Stage 2: Runtime
FROM alpine:latest

RUN apk --no-cache add ca-certificates && \
    adduser -D -u 10001 pflex

COPY --from=builder /app/pflex_exporter /usr/bin/pflex_exporter
COPY config.yaml /etc/pflex_exporter/config.yaml

EXPOSE 2112

USER pflex

ENTRYPOINT ["/usr/bin/pflex_exporter"]
CMD ["--config", "/etc/pflex_exporter/config.yaml"]
