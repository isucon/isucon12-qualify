#!/bin/sh

set -xe
export DEBIAN_FRONTEND=noninteractive
apt-get update && apt-get -y upgrade && apt-get -y install curl git openssh-server sudo rsync

curl -sL -o ./mitamae https://github.com/itamae-kitchen/mitamae/releases/download/v1.13.0/mitamae-x86_64-linux
chmod +x ./mitamae
./mitamae version
