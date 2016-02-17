FROM        golang:1.5.3
MAINTAINER  The Prometheus Authors <prometheus-developers@googlegroups.com>

WORKDIR /go/src/github.com/prometheus/alertmanager
COPY    . /go/src/github.com/prometheus/alertmanager

RUN apt-get install make \
    && make build \
    && cp alertmanager /bin/ \
    && mkdir -p /etc/alertmanager/template \
    && mv ./doc/examples/simple.yml /etc/alertmanager/config.yml \
    && rm -rf /go

EXPOSE     9093
VOLUME     [ "/alertmanager" ]
WORKDIR    /alertmanager
ENTRYPOINT [ "/bin/alertmanager" ]
CMD        [ "-config.file=/etc/alertmanager/config.yml", \
             "-storage.path=/alertmanager" ]
