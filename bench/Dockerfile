FROM golang:1.18.4-bullseye AS bench-builder

RUN mkdir /opt/isucon12-qualify
ADD . /opt/isucon12-qualify
WORKDIR /opt/isucon12-qualify/bench
RUN make bench

FROM 497146501771.dkr.ecr.ap-northeast-1.amazonaws.com/stg/benchmarker:supervisor AS supervisor

FROM debian:bullseye-slim

RUN apt-get -y update && apt-get -y install ca-certificates curl unzip jq

WORKDIR /tmp
RUN curl -s "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip" && \
	unzip -q awscliv2.zip && \
	./aws/install && \
	rm -rf awscliv2.zip ./aws/
ADD bench/entrypoint /usr/local/bin/

RUN mkdir -p /opt/isucon12-qualify/bench
COPY --from=bench-builder /opt/isucon12-qualify/bench/bench /opt/isucon12-qualify/bench/bench
COPY --from=bench-builder /opt/isucon12-qualify/bench/benchmarker*.json /opt/isucon12-qualify/bench/
COPY --from=bench-builder /opt/isucon12-qualify/bench/*.pem /opt/isucon12-qualify/bench/
COPY --from=supervisor /usr/local/bin/isuxportal-supervisor /usr/local/bin/isuxportal-supervisor
COPY public/ /opt/isucon12-qualify/public/

WORKDIR /opt/isucon12-qualify/bench

ENV ISUXPORTAL_SUPERVISOR_INSTANCE_NAME=
ENV ISUXPORTAL_SUPERVISOR_INTERVAL_AFTER_EMPTY_RECEIVE=5
ENV ISUXPORTAL_SUPERVISOR_ENDPOINT_URL=http://localhost:3000
ENV ISUXPORTAL_SUPERVISOR_TOKEN=dummy

ENTRYPOINT [ "/usr/local/bin/entrypoint" ]
CMD ["/usr/local/bin/isuxportal-supervisor", "./bench"]
