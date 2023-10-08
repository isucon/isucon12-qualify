# [make deploy]で起動
deploy:
    echo "deploying..."
	ssh isucon@52.196.244.84 sudo systemctl stop isuports.service
	scp * isucon@52.196.244.84:/home/isucon/webapp/go
	ssh isucon@52.196.244.84 sudo systemctl start isuports.service
	ssh isucon@52.196.244.84 sudo systemctl enable isuports.service