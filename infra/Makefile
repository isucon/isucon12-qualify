.PHONY: build clean
build: files.tar.gz isucon-admin.pub
	packer build qualify.pkr.hcl

files.tar.gz: sshd_config *.sh *.service *.pub
	tar cvzf files.tar.gz $^

clean:
	rm -f isucon-admin files.tar.gz

isucon-admin: isucon-admin.encrypted
	sops -d isucon-admin.encrypted > isucon-admin.tmp
	mv isucon-admin.tmp isucon-admin
	chmod 400 isucon-admin
	rm -f isucon-admin.tmp
