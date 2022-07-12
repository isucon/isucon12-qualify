#!/bin/bash

set -eo pipefail
rm -f ci.log
./bench -target-addr nginx:443 -target-url https://t.isucon.dev -strict-prepare=false -duration 3m | tee ci.log
