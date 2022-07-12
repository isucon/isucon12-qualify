.PHONY: build clean
build: isucon-env-checker/isucon-env-checker isucon-admin.pub
	packer build envcheck.pkr.hcl

isucon-env-checker/isucon-env-checker: isucon-env-checker/*.go isucon-env-checker/go.*
	cd isucon-env-checker && GOOS=linux GOARCH=amd64 go build .

clean:
	rm -f isucon-env-checker/isucon-env-checker isucon-admin

isucon-admin: isucon-admin.encrypted
	sops -d isucon-admin.encrypted > isucon-admin.tmp
	mv isucon-admin.tmp isucon-admin
	chmod 400 isucon-admin
	rm -f isucon-admin.tmp
