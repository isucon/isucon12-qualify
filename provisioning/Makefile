.PHONY: build clean
build: mitamae.tar.gz
	packer build qualify.pkr.hcl

isucon-env-checker/isucon-env-checker: isucon-env-checker/*.go isucon-env-checker/go.*
	cd isucon-env-checker && GOOS=linux GOARCH=amd64 go build .

clean:
	rm -f isucon-env-checker/isucon-env-checker isucon-admin mitamae.tar.gz

isucon-admin: isucon-admin.encrypted
	sops -d isucon-admin.encrypted > isucon-admin.tmp
	mv isucon-admin.tmp isucon-admin
	chmod 400 isucon-admin
	rm -f isucon-admin.tmp

mitamae.tar.gz: isucon-env-checker/isucon-env-checker
	install isucon-env-checker/isucon-env-checker mitamae/cookbooks/envchecker/
	tar czvf mitamae.tar.gz mitamae/
