FROM        prom/busybox:latest
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

COPY amtool                       /bin/amtool
COPY alertmanager                 /bin/alertmanager
COPY examples/ha/alertmanager.yml /etc/alertmanager/alertmanager.yml

RUN mkdir -p /alertmanager && \
    chown -R nobody:nogroup etc/alertmanager /alertmanager

USER       nobody
EXPOSE     9093
VOLUME     [ "/alertmanager" ]
WORKDIR    /alertmanager
ENTRYPOINT [ "/bin/alertmanager" ]
CMD        [ "--config.file=/etc/alertmanager/alertmanager.yml", \
             "--storage.path=/alertmanager" ]
