#!/bin/bash

set -eo pipefail
rm -f ci.log
./bench -target-addr nginx:443 -target-url https://t.isucon.dev -strict-prepare=false | tee ci.log
