FROM ubuntu:22.04

RUN apt-get update && apt-get install -y curl && curl -fsSL https://deb.nodesource.com/setup_18.x | bash - && apt-get install -y nodejs

RUN mkdir -p /home/isucon/frontend
WORKDIR /home/isucon/frontend

CMD ["npm", "run", "serve"]