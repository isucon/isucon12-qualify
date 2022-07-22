directory "/opt/isucon-env-checker" do
  owner "root"
  group "root"
  mode "0755"
end

remote_file "/usr/local/bin/isucon-env-checker" do
  owner "root"
  group "root"
  mode "0755"
  source "isucon-env-checker"
end

remote_file "/opt/isucon-env-checker/run-isucon-env-checker.sh" do
  owner "root"
  group "root"
  mode "0700"
  source "run-isucon-env-checker.sh"
end

remote_file "/opt/isucon-env-checker/warmup.sh" do
  owner "root"
  group "root"
  mode "0700"
  source "warmup.sh"
end

remote_file "/etc/systemd/system/isucon-env-checker.service" do
  owner "root"
  group "root"
  mode "0644"
  source "isucon-env-checker.service"
  not_if { File.exist?("/.dockerenv") }
end

execute "systemctl daemon-reload" do
  not_if { File.exist?("/.dockerenv") }
end

execute "systemctl enable isucon-env-checker" do
  not_if { File.exist?("/.dockerenv") }
end
