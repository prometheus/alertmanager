FROM        sdurrheimer/alpine-golang-make-onbuild
MAINTAINER  Prometheus Team <prometheus-developers@googlegroups.com>

USER root
RUN  mkdir /alertmanager \
     && chown golang:golang /alertmanager

USER        golang
WORKDIR     /alertmanager
EXPOSE      9093
