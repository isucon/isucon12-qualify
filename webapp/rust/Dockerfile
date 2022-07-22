FROM rust:1.62-bullseye

WORKDIR /tmp
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get -y upgrade && \
    apt-get install -y wget gcc g++ make sqlite3 && \
    wget -q https://dev.mysql.com/get/mysql-apt-config_0.8.22-1_all.deb && \
    apt-get -y install ./mysql-apt-config_*_all.deb && \
    apt-get -y update && \
    apt-get -y install mysql-client

RUN useradd --uid=1001 --create-home isucon
USER isucon

RUN mkdir -p /home/isucon/webapp/rust
WORKDIR /home/isucon/webapp/rust
COPY --chown=isucon:isucon ./ /home/isucon/webapp/rust/

CMD [ "cargo", "run", "--release" ]
