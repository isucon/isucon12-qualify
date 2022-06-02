builder: go.* *.go ../webapp/go/*
	@go build -o builder .

build: builder
	@./builder 10
	mysqldump -uisucon -pisucon -B isuports --no-tablespaces > 90_data.sql

clean:
	rm -f *.db *.sql *.json

data.tar.gz:
	tar czf data.tar.gz *.db *.sql *.json

install: clean build
	mv cp.db ../webapp/tenant_db/
	mv cp.sql ../webapp/sql/admin/
	mv cp.json ../bench/
