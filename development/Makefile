.PHONY: install-initialdata init-mysql run-bench run-bench-noload

up/%:
	docker compose -f docker-compose-$*.yml -f docker-compose-common.yml up --build -d

down/%:
	docker compose -f docker-compose-$*.yml -f docker-compose-common.yml down

logs-webapp/%:
	docker compose -f docker-compose-$*.yml -f docker-compose-common.yml logs -f webapp

install-initialdata:
	docker compose -f docker-compose-common.yml cp ../initial_data/ bench:/home/isucon/

init-mysql:
	docker compose -f docker-compose-common.yml exec --workdir /docker-entrypoint-initdb.d mysql \
		bash -c 'cat 10_schema.sql 90_data.sql | mysql -uisucon -pisucon isuports'

run-bench:
	docker compose -f docker-compose-common.yml exec bench ./run_local.sh

run-bench-noload:
	docker compose -f docker-compose-common.yml exec bench ./run_local.sh -prepare-only
