FROM golang:1.22 AS builder

ADD . /app
WORKDIR /app

RUN make clean && make && mv mirage-ecs /stash/
RUN cp -a html /stash/
RUN cp docker/example-config.yml /stash/

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
RUN mkdir -p /opt/mirage/html
COPY --from=builder /stash/mirage-ecs /opt/mirage/
COPY --from=builder /stash/example-config.yml /opt/mirage/
COPY --from=builder /stash/html/* /opt/mirage/html/
WORKDIR /opt/mirage
ENV MIRAGE_LOG_LEVEL info
ENV MIRAGE_CONF ""
RUN /opt/mirage/mirage-ecs -version
