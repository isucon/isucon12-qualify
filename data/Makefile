builder: go.* *.go ../webapp/go/*
	@go build -o builder .

build: builder
	@./builder 10
	mysqldump -uisucon -pisucon -B isuports --no-tablespaces > 90_data.sql

clean:
	rm -f *.db isuports.sql

data.tar.gz: clean build
	tar czf data.tar.gz *.db isuports.sql

install: clean build
	mv *.db ../webapp/tenant_db/
	mv *.sql ../webapp/sql/admin/
