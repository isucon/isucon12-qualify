FROM public.ecr.aws/docker/library/ruby:3.1.2-bullseye

WORKDIR /tmp
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
  apt-get -y upgrade && \
  curl -sSfLO https://dev.mysql.com/get/mysql-apt-config_0.8.22-1_all.deb && \
  apt-get -y install ./mysql-apt-config_*_all.deb && \
  apt-get -y update && \
  apt-get -y install sqlite3 mysql-client

RUN useradd --uid=1001 --create-home isucon
USER isucon

RUN mkdir -p /home/isucon/webapp/ruby
WORKDIR /home/isucon/webapp/ruby

ADD Gemfile Gemfile.lock /home/isucon/webapp/ruby/
RUN bundle config set deployment true && bundle config set without development && bundle config set path /home/isucon/tmp/ruby/gems

COPY --chown=isucon:isucon ./ /home/isucon/webapp/ruby
CMD ["/home/isucon/webapp/ruby/start-ruby.sh"]
