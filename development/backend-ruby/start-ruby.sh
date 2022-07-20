#!/bin/bash
set -ex

bundle install -j8
exec bundle exec puma -e production -p ${SERVER_APP_PORT:-3000} -w 2
