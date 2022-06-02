#!/bin/sh
set -xe

cd /dev/shm

apt-get -y update && apt-get -y update
apt-get -y install bash git mysql-server-8.0 build-essential curl ca-certificates lsb-release ubuntu-keyring nginx

# Go1.18
wget https://go.dev/dl/go1.18.3.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.18.3.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> /home/isucon/.bashrc
