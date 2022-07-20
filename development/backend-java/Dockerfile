FROM openjdk:17-bullseye

WORKDIR /tmp
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
  apt-get -y upgrade && \
  apt-get install -y wget gcc g++ make sqlite3 && \
  wget -q https://dev.mysql.com/get/mysql-apt-config_0.8.22-1_all.deb && \
  apt-get -y install ./mysql-apt-config_*_all.deb && \
  apt-get -y update && \
  apt-get -y install mariadb-client

RUN curl -sLO https://github.com/cli/cli/releases/download/v2.13.0/gh_2.13.0_linux_amd64.tar.gz && \
  tar xf gh_2.13.0_linux_amd64.tar.gz && \
  install gh_2.13.0_linux_amd64/bin/gh /usr/local/bin

WORKDIR /home/isucon/webapp/java
CMD ["./gradlew", "bootRun"]