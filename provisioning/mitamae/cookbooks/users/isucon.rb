group "isucon" do
  gid 1001
end

user "isucon" do
  uid 1001
  gid 1001
  create_home true
  shell "/bin/bash"
end

execute 'add groups to isucon' do
  command 'usermod -G sudo,docker isucon'
end

directory "/home/isucon" do
  owner "isucon"
  group "isucon"
  mode "0711"
end

directory "/home/isucon/.ssh" do
  owner "isucon"
  group "isucon"
  mode "0700"
end

directory "/home/isucon/tmp" do
  owner "isucon"
  group "isucon"
  mode "1777"
end

file "/etc/sudoers.d/isucon" do
  owner "root"
  group "root"
  mode "0644"
  content "isucon ALL=(ALL) NOPASSWD:ALL
"
end

file "/home/isucon/.hushlogin" do
  owner "isucon"
  group "isucon"
  mode "0600"
  content ""
end

directory "/home/isucon/.docker" do
  owner "isucon"
  group "isucon"
  mode "0755"
end

directory "/home/isucon/.docker/cli-plugins" do
  owner "isucon"
  group "isucon"
  mode "0755"
end

http_request "/home/isucon/.docker/cli-plugins/docker-compose" do
  url "https://github.com/docker/compose/releases/download/v2.6.1/docker-compose-linux-x86_64"
  owner "isucon"
  group "isucon"
  mode "0755"
end
