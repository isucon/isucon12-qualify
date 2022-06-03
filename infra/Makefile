.PHONY: build clean

export GOARCH = amd64
export GOOS = linux

build: files.tar.gz webapp.tar.gz bench.tar.gz isucon-admin.pub
	packer build -on-error=ask qualify.pkr.hcl

files.tar.gz: sshd_config *.sh *.service *.pub *.conf isuports.conf
	tar cvzf files.tar.gz $^

isuports.conf:
	cp ../nginx/conf.d/default.conf isuports.conf

webapp.tar.gz:
	cd ../webapp && rm -f go/isuports sql/admin/90_data.sql tenant_db/*.db
	cd ../data && make clean build install
	cd .. && tar cvzf infra/webapp.tar.gz webapp

bench.tar.gz:
	cd ../bench && make clean bench
	cd .. && tar czvf infra/bench.tar.gz bench

clean:
	rm -f isucon-admin *.tar.gz
	cd ../webapp && rm -f go/isuports sql/admin/90_data.sql tenant_db/*.db

isucon-admin: isucon-admin.encrypted
	sops -d isucon-admin.encrypted > isucon-admin.tmp
	mv isucon-admin.tmp isucon-admin
	chmod 400 isucon-admin
	rm -f isucon-admin.tmp
