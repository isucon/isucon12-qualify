bench: *.go go.* cmd/bench/*.go
	go build -o bench cmd/bench/main.go

run: bench
	./bench

prepare: bench
	./bench -prepare-only

5s: bench
	./bench -duration 5s

ci: bench
	./bench -target-addr 127.0.0.1:3000 -target-url http://t.isucon.dev

ci-loadtype-light: bench
	./bench -target-addr 127.0.0.1:3000 -target-url http://t.isucon.dev -load-type light

.PHONY: clean
clean:
	rm -f bench
