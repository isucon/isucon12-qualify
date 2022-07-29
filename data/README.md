# 初期データ生成器

## 使用方法

```
make build-for-docker-compose
```

## 生成される初期データと配置場所

- `{1..100}.db`
  - テナント毎のSQLite DBファイル
  - `../initial_data/`と`../webapp/tenant_db/`以下に配置する
- `benchmarker*.json`
  - ベンチマーカーが初期データの検証に利用する
  - `../bench/`以下に配置する
- `90_data.sql`
  - 初期データのAdmin DB(MySQL)のdumpファイル
  - `../initial_data/`と`../webapp/sql/admin/`以下に配置する
