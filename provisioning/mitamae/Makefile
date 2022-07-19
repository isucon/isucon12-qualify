COMMAND := "cd /opt/mitamae && ./setup.sh && ./mitamae local roles/default.rb"

test:
	docker run -v `pwd`:/opt/mitamae -it ubuntu:22.04 sh -c $(COMMAND)
