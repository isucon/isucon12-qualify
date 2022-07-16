group "isucon-admin" do
  gid 1001
end

user "isucon-admin" do
  uid 1001
  gid 1001
  create_home true
  shell "/bin/bash"
end

directory "/home/isucon-admin/.ssh" do
  owner "isucon-admin"
  group "isucon-admin"
  mode "0700"
end

file "/etc/sudoers.d/isucon-admin" do
  owner "root"
  group "root"
  mode "0644"
  content "isucon-admin ALL=(ALL) NOPASSWD:ALL
"
end

remote_file "/home/isucon-admin/.ssh/authorized_keys" do
  owner "isucon-admin"
  group "isucon-admin"
  mode "0600"
  source "isucon-admin.pub"
end
