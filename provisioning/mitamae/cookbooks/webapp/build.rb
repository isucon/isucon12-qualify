# TODO 各言語分やる
execute 'build webapp go' do
  command 'docker compose -f docker-compose-go.yml build'
  user 'isucon'
  cwd '/home/isucon/webapp'
  not_if { File.exist?("/.dockerenv") }
end
