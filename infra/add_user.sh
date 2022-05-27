#!usr/bin/env bash
set -xe

useradd --groups sudo --create-home --shell /bin/bash isucon
passwd -d isucon

mkdir /home/isucon/.ssh
chown isucon:isucon /home/isucon/.ssh
chmod 700 /home/isucon/.ssh
echo 'isucon ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/isucon
chmod 440 /etc/sudoers.d/isucon
