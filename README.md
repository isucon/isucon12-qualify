

## Docker Compose v2 での実行方法

### 起動

Linux の場合は `DOCKER_BUILDKIT=1` を付けて実行してください。(Docker Desktop for Macでは不要)

```console
$ DOCKER_BUILDKIT=1 docker compose up --build
```

### 初期データ生成

benchコンテナに入って初期データ生成コマンドを実行します。

```console
$ docker compose exec bench bash

root@ubuntu:/home/isucon/bench# cd ../data
root@ubuntu:/home/isucon/data# make build-for-docker-compose
```

- 一度生成したデータは docker volume (initial_data) に残ります
- 削除する場合は `data` ディレクトリで `make clean` してください

### ベンチマーク実行

benchコンテナに入って実行します。

```console
$ docker compose exec bench bash

root@ubuntu:/home/isucon/bench# ./bench
[ADMIN] 05:23:14.682068 TargetURL: https://t.isucon.dev, TargetAddr: , InitializeRequestTimeout: 30s, InitializeRequestTimeout: 30s
(略)
```
