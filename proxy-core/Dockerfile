FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /proxy-core .

FROM scratch
COPY --from=builder /proxy-core /proxy-core
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/proxy-core"]
CMD ["--config", "/app/config.yaml"]
