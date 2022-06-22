bench: *.go go.* cmd/bench/*.go
	go build -o bench cmd/bench/main.go

run: bench
	./bench

prepare: bench
	./bench -prepare-only

5s: bench
	./bench -duration 5s


.PHONY: clean
clean:
	rm -f bench
