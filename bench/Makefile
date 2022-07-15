export SHELL=/bin/bash

bench: *.go go.* cmd/bench/*.go
	go build -o bench cmd/bench/main.go

run: bench
	./bench

prepare: bench
	./bench -prepare-only -strict-prepare=false

5s: bench
	./bench -duration 5s

ci: bench
	./run_ci.sh

ci-with-comment: ci
	./pr-comment ci.log

.PHONY: clean
clean:
	rm -f bench
