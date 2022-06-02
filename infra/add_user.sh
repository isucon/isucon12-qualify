#!/bin/sh
set -xe

useradd --groups sudo --create-home --shell /bin/bash isucon
passwd -d isucon

mkdir /home/isucon/.ssh
chown isucon:isucon /home/isucon/.ssh
chmod 700 /home/isucon/.ssh
echo 'isucon ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/isucon
chmod 440 /etc/sudoers.d/isucon

useradd -s /bin/bash -m -p '*' isucon-admin
mkdir -p /home/isucon-admin/.ssh
mv /dev/shm/isucon-admin.pub /home/isucon-admin/.ssh/authorized_keys
chmod 700 /home/isucon-admin/.ssh
chmod 600 /home/isucon-admin/.ssh/authorized_keys
chown -R isucon-admin:isucon-admin /home/isucon-admin/.ssh
echo 'isucon-admin ALL=(ALL) NOPASSWD:ALL' | tee /etc/sudoers.d/isucon-admin
