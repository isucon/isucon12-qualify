package "sqlite3"
package "docker.io"
package "build-essential"

file "/etc/hosts" do
  owner "root"
  group "root"
  mode "0644"
  content <<-EOS
127.0.0.1 localhost

192.168.0.11 isuports-1.t.isucon.dev
192.168.0.12 isuports-2.t.isucon.dev
192.168.0.13 isuports-3.t.isucon.dev
  EOS
end

file "/etc/ssh/sshd_config.d/pubkey.conf" do
  owner "root"
  group "root"
  mode "0644"
  content "PubkeyAcceptedAlgorithms=+ssh-rsa
"
end
