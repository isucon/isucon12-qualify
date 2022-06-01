builder: go.* *.go ../webapp/go/*
	@go build -o builder .

build: clean builder
	@./builder 10

clean:
	rm -f builder *.db
