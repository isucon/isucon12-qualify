FROM ubuntu:22.04

ENV GO_VERSION 1.18.2
RUN apt-get update && apt-get -y upgrade && apt-get install -y wget gcc g++ make sqlite3&& \
  wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz && \
  tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz && rm go${GO_VERSION}.linux-amd64.tar.gz

RUN mkdir -p /home/isucon
COPY . /home/isucon/webapp
WORKDIR /home/isucon/webapp/go

CMD ["/usr/local/go/bin/go", "run", "cmd/isuports/main.go"]


