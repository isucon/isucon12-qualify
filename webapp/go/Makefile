# [make deploy]で起動
deploy:
    echo "deploying..."
	scp * isucon@<IP>:/home/isucon/webapp/go
	ssh isucon@<IP> sudo docker restart webapp-webapp-1 