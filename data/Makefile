builder: go.* *.go ../webapp/go/*
	@go build -o builder .

build: builder
	@./builder 10
	mysqldump -uisucon -pisucon isuports > isuports.sql

clean:
	rm -f *.db isuports.sql
