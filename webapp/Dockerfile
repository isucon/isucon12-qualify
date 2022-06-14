FROM ubuntu:22.04

ENV GO_VERSION 1.18.2
RUN apt-get update && apt-get -y upgrade && apt-get install -y wget gcc g++ make mysql-client-8.0 sqlite3&& \
  wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz && \
  tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz && rm go${GO_VERSION}.linux-amd64.tar.gz

RUN mkdir -p /home/isucon/webapp/go
WORKDIR /home/isucon/webapp/go

ENV GOPATH /home/isucon/go
ENV PATH /home/isucon/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
RUN go install github.com/cespare/reflex@latest

CMD [ "reflex", "-r", "(\\.go$|go\\.mod)", "-s", "go", "run", "cmd/isuports/main.go" ]
