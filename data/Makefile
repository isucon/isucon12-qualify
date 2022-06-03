builder: go.* *.go ../webapp/go/*
	@go build -o builder .

build: builder
	@./builder 100
	mysqldump -uisucon -pisucon --host 127.0.0.1 -B isuports --no-tablespaces > 90_data.sql

clean:
	rm -f *.db *.sql *.json

data.tar.gz:
	tar czf data.tar.gz *.db *.sql *.json

install:
	cp *.db ../webapp/tenant_db/
	cp *.sql ../webapp/sql/admin/
	cp *.json ../bench/
