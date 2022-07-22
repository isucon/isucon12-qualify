FROM perl:5.36.0-bullseye

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

RUN mkdir -p /home/isucon/webapp/perl
WORKDIR /home/isucon/webapp/perl

COPY cpanfile ./
RUN cpm install

COPY --chown=isucon:isucon ./ /home/isucon/webapp/perl/
ENV PERL5LIB=/home/isucon/webapp/perl/local/lib/perl5
ENV PATH=/home/isucon/webapp/perl/local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

CMD ["./local/bin/plackup", "-s", "Starlet", "-p", "3000", "-Ilib", "app.psgi"]
