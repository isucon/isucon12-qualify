=begin
%w[go python ruby php perl node java rust].each do |lang|
  execute "build webapp #{lang}" do
    command "docker compose -f docker-compose-#{lang}.yml build"
    user 'isucon'
    cwd '/home/isucon/webapp'
    not_if { File.exist?("/.dockerenv") }
  end
end
=end

%w[go python ruby php perl node node/node_modules java rust rust/target rust/registry].each do |d|
  directory "/home/isucon/tmp/#{d}" do
    owner 'isucon'
    group 'isucon'
    mode '1777'
  end
end
