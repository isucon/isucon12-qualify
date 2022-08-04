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
```

## JWTに利用する鍵について

ISUCON12 予選ではウェブアプリケーションのログインに JWT を利用しています。 JWT を生成・検証するための公開鍵・秘密鍵はそれぞれ以下に配置されています。

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

リージョン ap-northeast-1 AMI-ID ami-0ca04106bc403aeac で起動してください。
このAMIは予告なく利用できなくなる可能性があります。

- 起動に必要なEBSのサイズは最低8GBですが、ベンチマーク中にデータが増えると溢れる可能性があるため、大きめに設定することをお勧めします(競技環境では20GiB)
- セキュリティグループはTCP 443, 22を必要に応じて開放してください
- 適切なインスタンスプロファイルを設定することで、セッションマネージャーによる接続が可能です
- 起動時に指定したキーペアで `ubuntu` ユーザーでSSH可能です
  - その後 `sudo su - isucon` で `isucon` ユーザーに切り替えてください
- ベンチマーカーは `/home/isucon/bench` 以下にビルド済みのバイナリがあります
  - 自分でビルドする場合は適宜 Go 1.18.x をインストールして、`/home/isucon/bench` 以下で `make` を実行してください
```console
$ cd /home/isucon/bench
$ ./bench                               # 127.0.0.1:443 に対して実行
% ./bench -target-addr 203.0.113.1:443  # IPアドレスを指定し別のホストに対して実行
```

### 用意されたCloudFormationテンプレートを利用する場合

[cloudformation.yaml](cloudformation.yaml) を利用して、CloudFormationによって環境を起動できます。使用するAMIは上記「用意されたAMIを利用する場合」と同じもののため、予告なく利用できなくなる可能性があります。

- 使用できるリージョンは ap-northeast-1 のみです
- EC2 (c5.large) が3台起動するため、課金額には注意して下さい
- EC2用のkeypairは自動作成され、Systems Manager パラメータストアの /ec2/keypair/ 以下に秘密鍵が保存されます

## 初期データについて

https://github.com/isucon/isucon12-qualify/releases 以下にビルド済みの初期データがあります。

## Links

- [ISUCON12予選 レギュレーション](https://isucon.net/archives/56671734.html)
- [ISUCON12予選 当日マニュアル](https://gist.github.com/mackee/4320c18919c8f6f1867849378a17e651)
- [ISUCON12予選 解説(Node.jsでSQLiteのまま10万点行く方法)](https://isucon.net/archives/56842718.html)
- [ISUCON12予選 問題の解説と講評](https://isucon.net/archives/56850281.html)
