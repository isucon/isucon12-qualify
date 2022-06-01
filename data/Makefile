builder: go.* *.go ../webapp/go/*
	@go build -o builder .

run: builder
	@./builder

clean:
	rm -f builder *.db
