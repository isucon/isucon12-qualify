# bench

# TODO

- 鍵がPEM版、webappはまだ多分jwk版なので動かないです


## How to run

前提

- repo root にいる状態
- docker-compose で起動している状態
  - nginx が port 80
  - mysql が port 13306

```console
$ gh release download dummydata/20220421_0056-prod # もしくはreleaseからisucon_listen80_dump_prod.tar.gzをダウンロード
$ tar xvf isucon_listen80_dump_prod.tar.gz
$ mysql -uroot -proot --host 127.0.0.1 --port 13306 < isucon_listen80_dump.sql
$ cd bench
$ make
$ ./bench -target-url http://localhost  # nginxのportを変えている場合はportを合わせる
```


# メモ

なんとかActionでリクエストを作って送って返ってきたresをValidateResponseで検証してるんだけど、この2つの関数に関係がないのでリクエスト開始から結果取得完了までの時刻(レスポンスタイム)を元になにかするのができない
n秒超えたらタイムアウトではないけど0点、みたいな調整がやりづらいのでそこだけ作りを変えたい気持ち
リクエストを送るctxをwrapして、そこでリクエスト送信時にメタデータを入れてvalidateにもctxを渡してそれをみれるようにするとか

