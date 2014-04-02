FROM       ubuntu
MAINTAINER Johannes 'fish' Ziemke <fish@docker.com> (@discordianfish)

RUN        apt-get update && apt-get install -yq curl git mercurial make
RUN        curl -s https://go.googlecode.com/files/go1.2.linux-amd64.tar.gz | tar -C /usr/local -xzf -
ENV        PATH    /usr/local/go/bin:$PATH
ENV        GOPATH  /go

ADD        . /go/src/github.com/prometheus/alertmanager
RUN        make -C /go/src/github.com/prometheus/alertmanager build

WORKDIR    /alertmanager
ENTRYPOINT [ "/go/src/github.com/prometheus/alertmanager/alertmanager" ]
EXPOSE     9090
