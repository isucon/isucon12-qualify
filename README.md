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

## ISUCON12 予選当日との変更点

### Reproduce mode（コンテスト当日の再現について）

ベンチマーカーが負荷走行の一部で本来意図していない挙動をしていた点について、競技終了後に修正を行いました。
[#226](https://github.com/isucon/isucon12-qualify/pull/226)

修正点

- `GET /api/player/competition/:competition_id/ranking`が空の内容を返した際に、再度リクエストする前に sleep が入るようになりました。

修正により、コンテスト当日と比べてベンチマークスコアが低くなることがあります。
コンテスト当日のベンチマーカーの挙動を再現する場合は`-reproduce`をつけてベンチマーカーを実行してください。
デフォルトは無効になっています。

## JWT に利用する鍵について

ISUCON12 予選ではウェブアプリケーションのログインに JWT を利用しています。 JWT を生成・検証するための公開鍵・秘密鍵はそれぞれ以下に配置されています。

- 秘密鍵(すべて同じ内容です)
  - `bench/isuports.pem`
  - `blackauth/isuports.pem`
- 公開鍵
  - `webapp/public.pem`

## ISUCON12 予選のインスタンスタイプ

- 競技者 VM 3 台
  - InstanceType: c5.large (2vCPU, 4GiB Mem)
  - VolumeType: gp3 20GB
- ベンチマーカー VM 1 台
  - ECS Fargate (4vCPU, 8GB Mem)

## AWS 上での過去問環境の構築方法

### 用意された AMI を利用する場合

リージョン ap-northeast-1 AMI-ID ami-05c5b59deed48f66b で起動してください。
この AMI は予告なく利用できなくなる可能性があります。

### 自分で AMI をビルドする場合

上記 AMI が利用できなくなった場合は、[provisioning/](provisioning/)以下で`make build`を実行すると AMI をビルドできます。[packer](https://www.packer.io/) が必要です。(運営時に検証したバージョンは v1.8.0)

### AMI から EC2 を起動する場合の注意事項

- 起動に必要な EBS のサイズは最低 8GB ですが、ベンチマーク中にデータが増えると溢れる可能性があるため、大きめに設定することをお勧めします(競技環境では 20GiB)
- セキュリティグループは TCP 443, 22 を必要に応じて開放してください
- 適切なインスタンスプロファイルを設定することで、セッションマネージャーによる接続が可能です
- 起動時に指定したキーペアで `ubuntu` ユーザーで SSH 可能です
  - その後 `sudo su - isucon` で `isucon` ユーザーに切り替えてください
- ベンチマーカーは `/home/isucon/bench` 以下にビルド済みのバイナリがあります
  - 自分でビルドする場合は適宜 Go 1.18.x をインストールして、`/home/isucon/bench` 以下で `make` を実行してください

```console
$ cd /home/isucon/bench
$ ./bench                               # 127.0.0.1:443 に対して実行
% ./bench -target-addr 203.0.113.1:443  # IPアドレスを指定し別のホストに対して実行
```

### 用意された CloudFormation テンプレートを利用する場合

[cloudformation.yaml](cloudformation.yaml) を利用して、CloudFormation によって環境を起動できます。使用する AMI は上記「用意された AMI を利用する場合」と同じもののため、予告なく利用できなくなる可能性があります。その場合は上記「自分で AMI をビルドする場合」を参考にして AMI を作成し、その ID を使用するようにテンプレートを修正してください。

- 使用できるリージョンは ap-northeast-1 のみです
- EC2 (c5.large) が 3 台起動するため、課金額には注意して下さい
- EC2 用の keypair は自動作成され、Systems Manager パラメータストアの /ec2/keypair/ 以下に秘密鍵が保存されます

## 初期データについて

https://github.com/isucon/isucon12-qualify/releases 以下にビルド済みの初期データがあります。

## Links

- [ISUCON12 予選 レギュレーション](https://isucon.net/archives/56671734.html)
- [ISUCON12 予選 当日マニュアル](https://gist.github.com/mackee/4320c18919c8f6f1867849378a17e651)
- [ISUCON12 予選 解説(Node.js で SQLite のまま 10 万点行く方法)](https://isucon.net/archives/56842718.html)
- [ISUCON12 予選 問題の解説と講評](https://isucon.net/archives/56850281.html)

## 準備

### アクセスログ取得用

```
alp -f /var/log/nginx/access.log --aggregates=/api/player/player/[0-9a-z]*,/api/player/competition/[0-9a-z]*/ranking,/api/organizer/competition/[0-9a-z]*/score,/api/organizer/competition/[0-9a-z]*/finish,/api/organizer/player/[0-9a-z]*/disqualified
```
