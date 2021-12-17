# Build image
FROM nexus.adsrv.wtf/click/golang:1.17.1-buster as build

WORKDIR /app
COPY ./ ./
ARG BUILD_VERSION=""
RUN make build


# Main image
FROM ubuntu:focal-20210921

WORKDIR /app

ENTRYPOINT [ "/app/alertmanager" ]