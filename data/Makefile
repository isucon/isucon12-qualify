builder: go.* *.go ../webapp/go/* cmd/builder/*
	@go build -o builder cmd/builder/main.go

build: builder
	@./builder 100
	mysqldump -uisucon -pisucon --host 127.0.0.1 -B isuports --no-tablespaces > 90_data.sql

build-for-local: builder
	@ISUPORTS_DATA_HUGE_TENANT_SCALE=2 \
		./builder 10
	mysqldump -uisucon -pisucon --host 127.0.0.1 -B isuports --no-tablespaces > 90_data.sql

clean:
	rm -f *.db *.sql *.json

data.tar.gz:
	tar czf data.tar.gz *.db *.sql *.json

install:
	cp *.db ../webapp/tenant_db/
	cp *.db ../initial_data/
	cp *.sql ../webapp/sql/admin/
	cp *.sql ../initial_data/
	cp *.json ../bench/
