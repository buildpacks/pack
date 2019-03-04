FROM golang:alpine AS builder

RUN apk update && apk add git && apk add ca-certificates
RUN adduser -D -g '' appuser
WORKDIR /app/server

COPY go.* /app/
COPY server /app/server

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-w -s" -o /tmp/dockerdial-server

FROM scratch

WORKDIR /app
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /tmp/dockerdial-server ./server
USER appuser
CMD ["./server"]
