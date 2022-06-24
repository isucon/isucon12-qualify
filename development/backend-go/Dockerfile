FROM golang:1.18.3-bullseye

WORKDIR /tmp
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
  apt-get -y upgrade && \
  apt-get install -y wget gcc g++ make sqlite3 && \
  wget -q https://dev.mysql.com/get/mysql-apt-config_0.8.22-1_all.deb && \
  apt-get -y install ./mysql-apt-config_*_all.deb && \
  apt-get -y update && \
  apt-get -y install mysql-client

RUN mkdir -p /home/isucon/webapp/go
WORKDIR /home/isucon/webapp/go

ENV GOPATH /home/isucon/go
ENV PATH /home/isucon/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
RUN go install github.com/cespare/reflex@latest

CMD [ "reflex", "-r", "(\\.go$|go\\.mod)", "-s", "go", "run", "cmd/isuports/main.go" ]
