FROM golang:1.27-alpine3.24 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/E5SubBot .

FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata && mkdir /log
WORKDIR /
COPY --from=builder /out/E5SubBot /E5SubBot
COPY config.yml.example /config.yml

ENTRYPOINT ["/E5SubBot"]
