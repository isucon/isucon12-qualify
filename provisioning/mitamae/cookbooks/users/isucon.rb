group "isucon" do
  gid 2000
end

user "isucon" do
  uid 2000
  gid 2000
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
