FROM golang:1.22 AS builder

ADD . /app
WORKDIR /app

RUN make feature-proxy

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /opt/feature-proxy
COPY --from=builder /app/feature-proxy /opt/feature-proxy/app

CMD ["/opt/feature-proxy/app"]
