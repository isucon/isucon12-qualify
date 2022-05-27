#!/usr/bin/env bash
set -xe

sudo apt update
sudo apt-get install -y git mysql-server-8.0 build-essential curl gnupg2 ca-certificates lsb-release ubuntu-keyring

# Go1.18
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.18.2.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> /home/isucon/.bashrc

# nginx
curl https://nginx.org/keys/nginx_signing.key | gpg --dearmor | sudo tee /usr/share/keyrings/nginx-archive-keyring.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/nginx-archive-keyring.gpg] \
http://nginx.org/packages/ubuntu `lsb_release -cs` nginx" \
    | sudo tee /etc/apt/sources.list.d/nginx.list
echo -e "Package: *\nPin: origin nginx.org\nPin: release o=nginx\nPin-Priority: 900\n" \
    | sudo tee /etc/apt/preferences.d/99nginx

sudo apt update
sudo apt -y install nginx
