FROM public.ecr.aws/docker/library/ruby:3.1.2-bullseye

WORKDIR /tmp
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
  apt-get -y upgrade && \
  curl -sSfLO https://dev.mysql.com/get/mysql-apt-config_0.8.22-1_all.deb && \
  apt-get -y install ./mysql-apt-config_*_all.deb && \
  apt-get -y update && \
  apt-get -y install sqlite3 mysql-client

RUN curl -sLO https://github.com/cli/cli/releases/download/v2.13.0/gh_2.13.0_linux_amd64.tar.gz && \
  tar xf gh_2.13.0_linux_amd64.tar.gz && \
  install gh_2.13.0_linux_amd64/bin/gh /usr/local/bin

WORKDIR /home/isucon/webapp/ruby
RUN bundle config set deployment true && bundle config set without development && bundle config set path /home/isucon/ruby/gems
COPY start-ruby.sh /home/isucon/ruby/

CMD ["/home/isucon/ruby/start-ruby.sh"]
