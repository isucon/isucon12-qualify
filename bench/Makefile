export SHELL=/bin/bash
export ENV?=stg
export TAG?=$(shell git log -1 --format=%H)
export ECR?=$(shell jq -r '.credHelpers | keys | .[0]' .docker/config.json)

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

.PHONY: clean image push
clean:
	rm -f bench benchmarker*.json ../initial_data.tar.gz

../initial_data.tar.gz:
	gh release list | awk '/Latest/{print $$3}' | xargs gh release download --dir ..

benchmarker.json: ../initial_data.tar.gz
	tar xzf ../initial_data.tar.gz -C ..

benchmarker_tenant.json: benchmarker.json

image: benchmarker.json benchmarker_tenant.json
	docker --config .docker \
		pull 497146501771.dkr.ecr.ap-northeast-1.amazonaws.com/$(ENV)/benchmarker:supervisor
	docker --config .docker \
		build \
		-f Dockerfile \
		-t $(ECR)/$(ENV)/benchmarker:$(TAG) \
		..

push:
	docker --config .docker \
		push $(ECR)/$(ENV)/benchmarker:$(TAG)
