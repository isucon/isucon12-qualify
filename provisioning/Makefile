.PHONY: build clean
build: mitamae.tar.gz
	packer build qualify.pkr.hcl

isucon-env-checker/isucon-env-checker: isucon-env-checker/*.go isucon-env-checker/go.*
	cd isucon-env-checker && GOOS=linux GOARCH=amd64 go build .

blackauth:
	cd ../blackauth && GOOS=linux GOARCH=amd64 go build .

clean:
	rm -rf isucon-env-checker/isucon-env-checker isucon-admin mitamae.tar.gz isucon12-qualify*

isucon-admin: isucon-admin.encrypted
	sops -d isucon-admin.encrypted > isucon-admin.tmp
	mv isucon-admin.tmp isucon-admin
	chmod 400 isucon-admin
	rm -f isucon-admin.tmp

initial_data.tar.gz:
	gh release list | awk '/Latest/{print $$3}' | xargs gh release download --dir .

isucon12-qualify:
	gh repo clone isucon/isucon12-qualify -- --depth=1 --branch=main

isucon12-qualify.tar.gz: isucon12-qualify
	cd isucon12-qualify && tar czf ../isucon12-qualify.tar.gz .

mitamae.tar.gz: isucon-env-checker/isucon-env-checker initial_data.tar.gz blackauth
	install isucon-env-checker/isucon-env-checker mitamae/cookbooks/envchecker/
	install blackauth mitamae/cookbooks/blackauth/
	tar czvf mitamae.tar.gz mitamae/
