package "nginx"

remote_file "/etc/nginx/sites-available/isuports.conf" do
  owner "root"
  group "root"
  mode "0644"
  source "isuports.conf"
end

remote_file "/etc/nginx/sites-available/isuports-php.conf" do
  owner "root"
  group "root"
  mode "0644"
  source "isuports-php.conf"
end

link "/etc/nginx/sites-enabled/isuports.conf" do
  to "/etc/nginx/sites-available/isuports.conf"
end

remote_file "/etc/nginx/sites-available/default" do
  owner "root"
  group "root"
  mode "0644"
  source "default.conf"
end

remote_directory "/etc/nginx/tls" do
  owner "root"
  group "root"
  mode "0755"
  source "tls"
end
