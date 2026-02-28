FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/dkpbot ./cmd/dkpbot

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/dkpbot /usr/local/bin/dkpbot
EXPOSE 8080
ENTRYPOINT ["dkpbot"]
CMD ["--config", "/etc/dkpbot/config.yaml"]
