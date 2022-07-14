variable "revision" {
  type    = string
  default = "unknown"
}

locals {
  name = "isucon12-qualify-${formatdate("YYYYMMDD-hhmm", timestamp())}"
  ami_tags = {
    Project  = "qualify"
    Family   = "isucon12-qualify"
    Name     = "${local.name}"
    Revision = "${var.revision}"
    Packer   = "1"
  }
  run_tags = {
    Project = "qualify"
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

source "amazon-ebs" "qualify" {
  ami_name    = "${local.name}"
  ami_regions = ["ap-northeast-1"]

  tags          = local.ami_tags
  snapshot_tags = local.ami_tags

  source_ami    = "${data.amazon-ami.ubuntu-jammy.id}"
  region        = "ap-northeast-1"
  instance_type = "t3.micro"

  run_tags        = local.run_tags
  run_volume_tags = local.run_tags

  ssh_interface           = "public_ip"
  ssh_username            = "ubuntu"
  temporary_key_pair_type = "ed25519"
}

build {
  sources = ["source.amazon-ebs.qualify"]

  provisioner "file" {
    destination = "/dev/shm/mitamae.tar.gz"
    source      = "./mitamae.tar.gz"
  }
  provisioner "file" {
    destination = "/dev/shm/isucon-admin.pub"
    source      = "./mitamae/cookbooks/users/isucon-admin.pub"
  }

  provisioner "shell" {
    env = {
      DEBIAN_FRONTEND = "noninteractive"
    }
    inline = [
      "cd /dev/shm",
      "tar xf mitamae.tar.gz",
      "cd mitamae",
      "sudo ./setup.sh",
      "sudo ./mitamae local roles/default.rb",

      # Remove authorized_keys for packer
      "sudo truncate -s 0 /home/ubuntu/.ssh/authorized_keys",
      "sudo truncate -s 0 /etc/machine-id",
    ]
  }
}
