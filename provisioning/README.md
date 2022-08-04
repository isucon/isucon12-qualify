# 環境セットアップガイド

ISUCON環境のセットアップを行うには以下のツールが必要です。

- [GitHub CLI](https://cli.github.com)
- [Packer](https://www.packer.io)
- make
- [AWS CLI](https://docs.aws.amazon.com/ja_jp/cli/latest/userguide/getting-started-install.html)
  - ご自身のAWS環境内にてAMIを作れる程度の権限を持つロールでログインしておく必要があります

provisioningディレクトリ内で`make`を実行すれば、AMIが作成されて利用できるようになります。
