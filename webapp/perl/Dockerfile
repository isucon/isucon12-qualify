FROM perl:5.36.0-bullseye

RUN mkdir -p /home/isucon/webapp/perl
WORKDIR /home/isucon/webapp/perl

COPY cpanfile ./
RUN cpm install -g
COPY app.psgi lib ./

CMD ["plackup", "-p", "3000", "-Ilib", "app.psgi"]
