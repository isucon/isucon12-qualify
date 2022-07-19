%w[go python ruby php perl].each do |lang|
  execute "build webapp #{lang}" do
    command "docker compose -f docker-compose-#{lang}.yml build"
    user 'isucon'
    cwd '/home/isucon/webapp'
    not_if { File.exist?("/.dockerenv") }
  end
end
