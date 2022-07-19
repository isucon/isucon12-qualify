ENV['DEBIAN_FRONTEND'] = 'noninteractive'
include_recipe '../cookbooks/webapp/build.rb'
