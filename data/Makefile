builder: go.* *.go ../webapp/go/* cmd/builder/*
	@go build -o builder cmd/builder/main.go

build: builder
	./builder 100
	mysqldump -uisucon -pisucon --host 127.0.0.1 -B isuports --no-tablespaces > 90_data.sql

build-for-docker-compose: builder
	./builder \
		-out-dir=../initial_data \
		-db-dsn='isucon:isucon@tcp(127.0.0.1:13306)/isuports?interpolateParams=true' \
		100
	mysqldump -uisucon -pisucon \
		--host 127.0.0.1 --port 13306 \
		-B isuports --no-tablespaces > ../initial_data/90_data.sql
	cp *.json ../bench/

clean:
	rm -f *.db *.sql *.json
	rm -f ../webapp/tenant_db/*.db
	rm -f ../initial_data/*.db

data.tar.gz:
	tar czf data.tar.gz *.db *.sql *.json

install:
	cp *.db ../webapp/tenant_db/
	cp *.db ../initial_data/
	cp *.sql ../webapp/sql/admin/
	cp *.sql ../initial_data/
	cp *.json ../bench/
