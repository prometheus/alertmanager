FROM openshift/origin-release:golang-1.10 AS builder

ARG ALERTMANAGER_GOPATH=/go/src/github.com/prometheus/alertmanager
COPY . ${ALERTMANAGER_GOPATH}
RUN cd ${ALERTMANAGER_GOPATH} && \
    make build

FROM        openshift/origin-base
MAINTAINER  The Prometheus Authors <prometheus-developers@googlegroups.com>

ARG ALERTMANAGER_GOPATH=/go/src/github.com/prometheus/alertmanager
COPY --from=builder ${ALERTMANAGER_GOPATH}/amtool                       /bin/amtool
COPY --from=builder ${ALERTMANAGER_GOPATH}/alertmanager                 /bin/alertmanager
COPY --from=builder ${ALERTMANAGER_GOPATH}/examples/ha/alertmanager.yml /etc/alertmanager/alertmanager.yml

EXPOSE     9093
VOLUME     [ "/alertmanager" ]
WORKDIR    /etc/alertmanager
ENTRYPOINT [ "/bin/alertmanager" ]
CMD        [ "--storage.path=/alertmanager" ]
