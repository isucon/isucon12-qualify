FROM python:3.9.13-bullseye

WORKDIR /tmp
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
  apt-get -y upgrade && \
  apt-get install -y wget gcc g++ make sqlite3 && \
  wget -q https://dev.mysql.com/get/mysql-apt-config_0.8.22-1_all.deb && \
  apt-get -y install ./mysql-apt-config_*_all.deb && \
  apt-get -y update && \
  apt-get -y install mysql-client build-essential libssl-dev libffi-dev python3-dev cargo

RUN useradd --uid=1001 --create-home isucon
USER isucon

RUN mkdir -p /home/isucon/webapp/python
WORKDIR /home/isucon/webapp/python

ADD requirements.txt /home/isucon/webapp/python
RUN pip install -r requirements.txt

COPY --chown=isucon:isucon ./ /home/isucon/webapp/python/

CMD [ "python", "main.py" ]
