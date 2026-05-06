FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /163-wrapper .

FROM scratch
COPY --from=builder /163-wrapper /163-wrapper
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
VOLUME ["/data"]
EXPOSE 1993
ENTRYPOINT ["/163-wrapper", "-c", "/data/config.yaml", "-d", "/data"]
