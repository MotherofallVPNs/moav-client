FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Stamp the client version into the binary (overrides the in-code default).
ARG MOAV_VERSION=1.3.1
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-X github.com/ibeezhan/moav-client/proxy-core/cmd.Version=${MOAV_VERSION}" \
      -o /proxy-core .

FROM scratch
COPY --from=builder /proxy-core /proxy-core
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/proxy-core"]
CMD ["--config", "/app/config.yaml"]
