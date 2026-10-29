# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/stress-test .

# ---- final stage ----
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/stress-test /stress-test
USER 65534:65534
ENTRYPOINT ["/stress-test"]
