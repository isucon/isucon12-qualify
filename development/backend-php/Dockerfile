FROM php:8.1.8-bullseye

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
  apt-get -y upgrade && \
  apt-get install -y wget make libzip-dev unzip sqlite3 && \
  wget -q https://dev.mysql.com/get/mysql-apt-config_0.8.22-1_all.deb && \
  apt-get -y install ./mysql-apt-config_*_all.deb && \
  apt-get -y update && \
  apt-get -y install mysql-client && \
  apt-get clean && \
  rm -rf /var/lib/apt/lists/*

RUN curl -sLO https://github.com/cli/cli/releases/download/v2.13.0/gh_2.13.0_linux_amd64.tar.gz && \
  tar xf gh_2.13.0_linux_amd64.tar.gz && \
  install gh_2.13.0_linux_amd64/bin/gh /usr/local/bin

RUN docker-php-ext-configure zip && \
    docker-php-ext-install zip && \
    docker-php-ext-install pdo_mysql

ENV DOCKERIZE_VERSION v0.6.1
RUN wget https://github.com/jwilder/dockerize/releases/download/$DOCKERIZE_VERSION/dockerize-alpine-linux-amd64-$DOCKERIZE_VERSION.tar.gz \
    && tar -C /usr/local/bin -xzvf dockerize-alpine-linux-amd64-$DOCKERIZE_VERSION.tar.gz \
    && rm dockerize-alpine-linux-amd64-$DOCKERIZE_VERSION.tar.gz

RUN ln -sf /usr/share/zoneinfo/Asia/Tokyo /etc/localtime

COPY ./development/backend-php/php.ini /usr/local/etc/php/php.ini

WORKDIR /home/isucon/webapp/php

COPY ./webapp/php /home/isucon/webapp/php

RUN ./composer.phar install

CMD ["./composer.phar", "start"]
