FROM       golang:latest
MAINTAINER Prometheus Team <prometheus-developers@googlegroups.com>

RUN apt-get -qy update && apt-get -qy install vim-common
ENV PKGPATH $GOPATH/src/github.com/prometheus/alertmanager
ENV GOROOT  /usr/src/go

ADD . $PKGPATH
RUN cd $PKGPATH && go get -d && make

USER       nobody
WORKDIR    /alertmanager
ENTRYPOINT [ "/go/src/github.com/prometheus/alertmanager/alertmanager" ]
EXPOSE     9093
