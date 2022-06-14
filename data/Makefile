builder: go.* *.go ../webapp/go/* cmd/builder/*
	@go build -o builder cmd/builder/main.go

build: builder
	@./builder 100
	mysqldump -uisucon -pisucon --host 127.0.0.1 -B isuports --no-tablespaces > 90_data.sql

build-for-local: builder
	@./builder 10
	mysqldump -uisucon -pisucon --host 127.0.0.1 -B isuports --no-tablespaces > 90_data.sql

clean:
	rm -f *.db *.sql *.json

data.tar.gz:
	tar czf data.tar.gz *.db *.sql *.json

install:
	cp *.db ../webapp/tenant_db/
	cp *.db ../initial_data/
	cp *.sql ../webapp/sql/admin/
	cp *.json ../bench/
