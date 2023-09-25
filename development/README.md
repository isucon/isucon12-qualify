# 注意

**このディレクトリの内容は、ISUCON12開発中に利用していたものです。現在はメンテナンスを行っていません。
参考のために残していますが、このディレクトリを利用した場合に発生した問題については回答できません。**

## 移植用開発環境

### 起動方法

以下の手順で行います。

1. GitHub release から初期データをダウンロード
   - `gh` コマンドが必要です https://github.com/cli/cli
2. Docker Compose (v2) で起動
   - `up/go` の `go` の部分は各言語実装によって書き換えてください。
3. 起動した環境に初期データをコピー

```console
$ make initial_data
$ make up/go
$ make install_initial_data
```

### ベンチ実行方法

```console
$ make run-bench
```

整合性チェックだけ走らせる(負荷を掛けない)場合は

```conosole
$ make run-bench-noload
```

### 初期データへのリセット方法

基本的にbenchmarkerが最初に行う `POST /initialize` によって復旧されますが、なんらかの原因で壊してしまったと思われる場合は以下でリセットできます。

```console
$ make install-initialdata
$ make init-mysql
```

GitHub Releaseの初期データが変わった場合は以下のようにします。

- `make down/go` して落とす
- `make clean`
- `docker volume ls` で `development-*` という volumen を見つけて全て削除
- 起動方法 の手順でやり直し

### アプリケーションログの閲覧方法

`go` の部分は各言語実装によって書き換えてください。

```console
$ make logs-webapp/go
```
