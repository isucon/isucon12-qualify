# ISUCON12 予選問題

## ディレクトリ構成

```
.
+- webapp     # 各言語の参考実装
+- bench      # ベンチマーカー
+- public     # Webフロント用の静的ファイル
+- frontend   # Webフロント
+- blackauth  # 動作確認をするためのJWTを返すAPI
+- nginx      # 初期状態のnginx設定ファイル
+- data       # 初期データ生成
+- probisioning # セットアップ用
`- infra        # セットアップ用
```

## JWTに利用する鍵について

ISUCON11 予選ではウェブアプリケーションのログインに JWT を利用しています。 JWT を生成・検証するための公開鍵・秘密鍵はそれぞれ以下に配置されています。

- 秘密鍵(すべて同じ内容です)
  - `bench/isuports.pem`
  - `blackauth/isuports.pem`
- 公開鍵
  - `webapp/public.pem`

## ISUCON12予選のインスタンスタイプ

- 競技者 VM 3台
  - InstanceType: c5.large (2vCPU, 4GiB Mem)
  - VolumeType: gp3 20GB
- ベンチマーカー VM 1台
  - ECS Fargate (4vCPU, 8GB Mem)

## AWS上での過去問環境の構築方法

### 用意されたAMIを利用する場合

TODO

### AMIを自前で用意する場合

TODO

## 初期データについて

https://github.com/isucon/isucon12-qualify/releases 以下にビルド済みの初期データがあります。  
利用方法は`webapp/README.md`を参照してください。

## Links

- [ISUCON12予選 レギュレーション](https://isucon.net/archives/56671734.html)
- [ISUCON12予選 当日マニュアル](https://gist.github.com/mackee/4320c18919c8f6f1867849378a17e651)
- [ISUCON12予選 解説(Node.jsでSQLiteのまま10万点行く方法)](https://isucon.net/archives/56842718.html)
