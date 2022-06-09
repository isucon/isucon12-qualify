variable "revision" {
  type    = string
  default = "unknown"
}

locals {
  name = "isucon12-quarify-${formatdate("YYYYMMDD-hhmm", timestamp())}"
  ami_tags = {
    Project  = "portal"
    Family   = "isucon12-quarify"
    Name     = "${local.name}"
    Revision = "${var.revision}"
    Packer   = "1"
  }

  run_tags = {
    Project = "quarify"
    Name    = "packer-${local.name}"
    Packer  = "1"
    Ignore  = "1"
  }
}

data "amazon-ami" "ubuntu-jammy" {
  filters = {
    name                = "ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"
    root-device-type    = "ebs"
    virtualization-type = "hvm"
  }
  most_recent = true
  owners      = ["099720109477"]
  region      = "ap-northeast-1"
}

source "amazon-ebs" "quarify" {
  ami_name    = "${local.name}"
  ami_regions = ["ap-northeast-1"]

  tags          = local.ami_tags
  snapshot_tags = local.ami_tags

  source_ami    = "${data.amazon-ami.ubuntu-jammy.id}"
  region        = "ap-northeast-1"
  instance_type = "t3.medium"

  run_tags        = local.run_tags
  run_volume_tags = local.run_tags

  ssh_interface           = "public_ip"
  ssh_username            = "ubuntu"
  temporary_key_pair_type = "ed25519"
}

build {
  sources = ["source.amazon-ebs.quarify"]

  provisioner "file" {
    destination = "/dev/shm/files.tar.gz"
    source      = "./files.tar.gz"
  }

  provisioner "file" {
    destination = "/dev/shm/webapp.tar.gz"
    source      = "./webapp.tar.gz"
  }

  provisioner "file" {
    destination = "/dev/shm/bench.tar.gz"
    source      = "./bench.tar.gz"
  }

  provisioner "file" {
    destination = "/dev/shm/blackauth.tar.gz"
    source      = "./blackauth.tar.gz"
  }

  provisioner "shell" {
    inline = [
      # Write REVISION
      "sudo sh -c 'echo ${var.revision} > /etc/REVISION'",

      # provisioning
      "cd /dev/shm && tar xvf files.tar.gz",
      "sudo /dev/shm/add_user.sh",
      "cd /home/isucon && sudo tar xvf /dev/shm/webapp.tar.gz",
      "cd /home/isucon && sudo tar xvf /dev/shm/bench.tar.gz",
      "cd /home/isucon && sudo tar xvf /dev/shm/blackauth.tar.gz",
      "sudo chown -R isucon:isucon /home/isucon/webapp /home/isucon/bench",
      "sudo /dev/shm/provisioning.sh",

      # Install isuport-go.service
      "sudo mv /dev/shm/*.service /etc/systemd/system/",
      "sudo chown root:root /etc/systemd/system/isuports-*.service",
      "sudo chown root:root /etc/systemd/system/blackauth.service",

      "sudo systemctl daemon-reload",
      "sudo systemctl enable isuports-go.service",
      "sudo systemctl enable blackauth.service",

      # Configure nginx
      "sudo mv /dev/shm/nginx.conf /etc/nginx/nginx.conf",
      "sudo mv /dev/shm/isuports.conf /etc/nginx/conf.d/isuports.conf",
      "sudo chown -R root:root /etc/nginx",

      # Configure SSH for isucon user
      "cat /dev/shm/sshd_config | sudo tee -a /etc/ssh/sshd_config",
      # Disable motd
      "sudo -u isucon touch /home/isucon/.hushlogin",

      # Remove authorized_keys for packer
      "sudo truncate -s 0 /home/ubuntu/.ssh/authorized_keys",
      "sudo truncate -s 0 /etc/machine-id",
    ]
  }
}
