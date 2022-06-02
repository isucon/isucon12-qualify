.PHONY: build clean
build: files.tar.gz webapp.tar.gz isucon-admin.pub
	packer build -on-error=ask qualify.pkr.hcl

files.tar.gz: sshd_config *.sh *.service *.pub *.conf isuports.conf
	tar cvzf files.tar.gz $^

isuports.conf:
	cp ../nginx/conf.d/default.conf isuports.conf

webapp.tar.gz:
	cd ../webapp && rm -f go/isuports sql/admin/90_data.sql tenant_db/*.db
	cd ../data && make install
	cd .. && tar cvzf infra/webapp.tar.gz webapp

clean:
	rm -f isucon-admin *.tar.gz
	cd ../webapp && rm -f go/isuports sql/admin/90_data.sql tenant_db/*.db

isucon-admin: isucon-admin.encrypted
	sops -d isucon-admin.encrypted > isucon-admin.tmp
	mv isucon-admin.tmp isucon-admin
	chmod 400 isucon-admin
	rm -f isucon-admin.tmp
