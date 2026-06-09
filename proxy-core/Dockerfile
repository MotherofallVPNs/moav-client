FROM golang:1.25-alpine AS builder
# Go-module fetch resilience: pipe (|) falls through to the next proxy on ANY
# error (incl. the 403 Google's module CDN returns under rate-limiting from some
# IPs/regions); goproxy.cn isn't subject to those limits. GOSUMDB=off drops the
# sum.golang.org dependency — integrity is still checked against the committed
# go.sum. Override via build args if you run your own mirror.
ARG GOPROXY="https://proxy.golang.org|https://goproxy.cn|direct"
ARG GOSUMDB=off
ENV GOPROXY=${GOPROXY} GOSUMDB=${GOSUMDB}
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
