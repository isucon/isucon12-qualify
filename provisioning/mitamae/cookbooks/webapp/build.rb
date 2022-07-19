# TODO 各言語分やる
execute 'build webapp go' do
  command 'docker compose -f docker-compose-go.yml build'
  user 'isucon'
  cwd '/home/isucon/webapp'
  not_if { File.exist?("/.dockerenv") }
end

execute 'build webapp python' do
  command 'docker compose -f docker-compose-python.yml build'
  user 'isucon'
  cwd '/home/isucon/webapp'
  not_if { File.exist?("/.dockerenv") }
end

execute 'build webapp ruby' do
  command 'docker compose -f docker-compose-ruby.yml build'
  user 'isucon'
  cwd '/home/isucon/webapp'
  not_if { File.exist?("/.dockerenv") }
end

execute 'build webapp php' do
  command 'docker compose -f docker-compose-php.yml build'
  user 'isucon'
  cwd '/home/isucon/webapp'
  not_if { File.exist?("/.dockerenv") }
end
