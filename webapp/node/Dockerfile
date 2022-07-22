FROM node:18.6.0-bullseye

WORKDIR /tmp
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
  apt-get -y upgrade && \
  apt-get install -y wget gcc g++ make sqlite3 libsqlite3-dev && \
  wget -q https://dev.mysql.com/get/mysql-apt-config_0.8.22-1_all.deb && \
  apt-get -y install ./mysql-apt-config_*_all.deb && \
  apt-get -y update && \
  apt-get -y install mysql-client

RUN useradd --uid=1001 --create-home isucon
USER isucon

RUN mkdir -p /home/isucon/webapp/node
WORKDIR /home/isucon/webapp/node
COPY --chown=isucon:isucon ./ /home/isucon/webapp/node/

CMD [ "bash", "-c", "npm install && npm run serve" ]
