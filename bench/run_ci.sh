#!/bin/bash

set -eo pipefail
rm -f ci.log
./bench -target-addr 127.0.0.1:443 -target-url https://t.isucon.dev | tee ci.log
