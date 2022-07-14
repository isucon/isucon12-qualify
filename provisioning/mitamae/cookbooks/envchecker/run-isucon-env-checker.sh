#!/bin/bash
set -x

retried=0
while [[ $retried -le 15 ]]; do
  sleep $(( RANDOM % 15 ))
  /usr/local/bin/isucon-env-checker boot && exit 0
  retried=$(( retried + 1 ))
done
