# ベンチマーカー

### localでの回し方

```
./run_local.sh
```

各種オプションは

```
go run cmd/bench/main.go --help
```

を参照してください。

### 想定負荷の流れ

- SaaS管理者 (1 worker)
  - `/api/admin/tenants/billing`を最新から順番に取得する
  - すべて見終わったらテナントを1つ作成し、NewTenantScenarioを開始する

- Organizer
  - 以下を繰り返す
    - 大会を追加
      - playerを追加
      - 大会を開催する
        - 以下を並列で実行する
        - CSV入稿
        - 複数のplayerがrankingを取得する
      - 大会のfinish
      - rankingの確認
    - `/api/organizer/billing`を取得する

- Player
  - workerとして、離脱条件を満たさない限り以下を繰り返す
    - テナント内の大会の一覧を取得する
    - 大会のうちの一つのrankingを参照する
      - 1秒(厳密には1.2秒)レスポンスが帰ってこなければ離脱としてworkerを終了する
    - rankingの中からplayerを選び、`/api/player/player/:player_name`を参照する

## シナリオ一覧

- SaaS管理者: AdminBilling
- Player
- 新規テナント: OrganizerNewTenant
- 既存巨大テナント(id=1): PopularTenant(heavry)
- 既存テナント: PopularTenant
- 管理者請求額確認: AdminBillingValidate
- テナント請求額確認: TenantBillingValidate
- プレイヤー確認 PlayerValidate

### 構成

Scenario_xxx
- workerとして動作するもの
job_xxx
- Action+Validationのまとまり
- 終わるまでブロックする
- Scenarioの中で利用する

