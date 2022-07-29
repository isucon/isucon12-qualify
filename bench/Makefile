export SHELL=/bin/bash
export TAG?=$(shell git log -1 --format=%H)

bench: *.go go.* cmd/bench/*.go
	go build -o bench cmd/bench/main.go

.PHONY: clean
clean:
	rm -f bench benchmarker*.json ../initial_data.tar.gz

../initial_data.tar.gz:
	gh release list | awk '/Latest/{print $$3}' | xargs gh release download --dir ..

benchmarker.json: ../initial_data.tar.gz
	tar xzf ../initial_data.tar.gz -C ..

benchmarker_tenant.json: benchmarker.json
