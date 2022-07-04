bench: *.go go.* cmd/bench/*.go
	go build -o bench cmd/bench/main.go

run: bench
	./bench

prepare: bench
	./bench -prepare-only -strict-prepare=false

5s: bench
	./bench -duration 5s

ci: bench
	rm -f ci.log
	./bench -target-addr 127.0.0.1:443 -target-url https://t.isucon.dev | tee ci.log

ci-with-comment: ci
	./pr-comment ci.log

ci-loadtype-light: bench
	./bench -target-addr 127.0.0.1:443 -target-url https://t.isucon.dev -load-type light -strict-prepare=false

.PHONY: clean
clean:
	rm -f bench
