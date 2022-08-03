# ベンチマーカー

### localでの実行方法

```
go run cmd/bench/main.go -target-url https://t.isucon.dev
```

各種オプションは以下を参照してください。

```
Usage of bench:
  -data-dir string
        Data directory (default "data")
  -debug
        Debug mode
  -duration duration
        Benchmark duration (default 1m0s)
  -exit-error-on-fail
        Exit error on fail
  -initialize-request-timeout duration
        Initialize request timeout (default 30s)
  -prepare-only
        Prepare only
  -reproduce
        reproduce contest day mode. default: false
  -request-timeout duration
        Default request timeout (default 30s)
  -skip-prepare
        Skip prepare
  -strict-prepare
        strict prepare mode. default: true (default true)
  -target-addr string
        Benchmark target address e.g. host:port
  -target-url string
        Benchmark target URL (default "https://t.isucon.dev")
```

webappはhost名を見てテナントDBを振り分けるため、`--target-url`にはテナント名のsubdomainを加える前のURLを指定します。  
`--target-addr`には実際にリクエストするwebappのアドレスを指定します。

例: 10.0.0.1:443に向けてベンチマークを実行する場合

```
go run cmd/bench/main.go -target-url https://t.isucon.dev --target-addr 10.0.0.1:443
```

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

- AdminBillingScenario          : SaaS管理者
- PlayerScenario                : 参加者
- OrganizerNewTenantScenario    : 新規テナント
- PopularTenantScenario         : 既存テナント
- AdminBillingValidateScenario  : 管理者請求額整合性チェック
- TenantBillingValidateScenario : テナント請求額整合性チェック
- PlayerValidateScenario	      : プレイヤー整合性チェック
- ValidateScenario	            : 整合性チェック

### 構成

Scenario_xxx
- workerとして動作するもの
job_xxx
- Action+Validationのまとまり
- 終わるまでブロックする
- Scenarioの中で利用する


### Reproduce mode（コンテスト当日の再現について）

負荷走行の一部で本来意図していない挙動をしていた点について、競技終了後に修正を行いました。  

修正点
- `GET /api/player/competition/:competition_id/ranking`が空の内容を返した際に、再度リクエストする前にsleepが入るようになりました。

修正により、コンテスト当日と比べてベンチマークスコアが低くなることがあります。  
コンテスト当日のベンチマーカーの挙動を再現する場合は`-reproduce`をつけてベンチマーカーを実行してください。  
デフォルトは無効になっています。
