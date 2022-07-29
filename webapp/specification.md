# エンドポイント仕様

## ホスト名

テナントごとのサブドメインにテナント名が入っている
- `{テナント名}.t.isucon.dev`
- `admin.t.isucon.dev` は SaaS 管理用の特別なホスト名

## 認証方式

### JWTセッションキー
外部サービス blackauth に ID を送るとJWTが発行される
  ブラックボックスなマイクロサービスは人間が動作検証する用で負荷走行には使用しない
    参加者ブラックボックスマイクロサービスのリバースエンジニアリングの方に走るのを防止するため

### JWT形式

- alg: RS256
- typ: JWT
- sub: ログインエンドポイントで渡したplayerの`name`
- aud: 発行元のtenantのname, admin roleでは`admin` という文字が入る
- role: `admin` `organizer` `player` いずれか
- exp: 24時間

## 請求額の仕様
終了した全ての大会について (大会にスコアを登録した参加者数 * 100 + スコア登録なしでランキングにアクセスした参加者 * 10) の総和 = 請求額(円)  
例: スコア登録参加者 20人, スコア登録なしランキング閲覧参加者が10人の場合,  20 * 100 + 10 * 10 = 2100円

## レスポンス基本フォーマット

### 成功時

`data` keyにはそれぞれのエンドポイントごとの「レスポンス」に設定された構造が入ります

```json
{
  "success": true,
  "data": object
}
```

### 失敗時

```json
{
  "success": false,
  "message": "error message"
}
```

### ResponseのHTTP Header

全APIに `Cache-Control: private` が設定されている必要があります

## SaaS管理者向けAPI

### POST `<admin endpoint>/api/admin/tenants/add`

テナント初期化エンドポイント
仕様
- リクエスト `application/x-www-form-urlencoded`
  - テナント識別子 `name` `^[a-z][a-z0-9-]{0,61}[a-z0-9]$`
    - 英数小文字
    - 先頭はアルファベット(数字はだめ
    - 最初と最後には `-` はだめ
    - 2〜63文字まで
    - テナント名 `display_name`
  - レスポンス `application/json`
    - `tenant`
      - `name` テナント識別子 リクエストの`name` がそのまま入る
      - `display_name` テナント名 リクエストの`display_name` がそのまま入る

初期実装
- 管理DB
  - テナント識別子のバリデーション、重複チェック
  - Tenant IDの発番
  - 主催者ユーザーの作成
- テナント作成シェルスクリプトの実行
- sqliteコマンドでデータベースを初期化する

###  GET `<admin endpoint>/api/admin/tenants/billing`

テナントごとの請求ダッシュボード
仕様
- リクエスト query string
  - `before`
    - 型: ID, optional
    - 指定されたテナントIDより小さいテナントの請求一覧を返す
- レスポンス `application/json`
  - `tenants` 配列 最大10件
  - `id` テナントID
    - 次のページをリクエストする場合はレスポンス中の最後の`id` を`before`引数に指定する
  -`name` テナント名
  - `display_name` テナント表示名
  - `billing_yen` テナントの総請求額 finishを呼んでない大会は加算しない

## 主催者向けAPI

### POST `<tenant endpoint>/api/organizer/players/add`

参加者の追加  
仕様
- リクエスト `application/x-www-form-urlencoded`
  - `display_name[]` 参加者の名前 複数指定可能
- レスポンス `application/json`
  - `players` 配列
    - `id` PlayerのID
    - `display_id` 参加者の識別子
    - `is_disqualified` 失格かどうか (追加直後なので必ずfalse)

### POST `<tenant endpoint>/api/organizer/player/:player_id/disqualified`

参加者を失格させる  
失格になった参加者がAPIリクエストすると403で失敗する  
スコア登録時にはplayerIDが含まれていても問題ない  
一度失格になった参加者を失格から回復することはできない  

仕様
- リクエスト パスに含まれる
  - `player_id`失格にする参加者のid
- レスポンス `application/json`
  - `id` 参加者のID
  - `display_name` 参加者の表示名
  - `is_disqualified` 失格かどうか (常に`true`)

### POST `<tenant endpoint>/api/organizer/competitions/add`

大会を作成する  
仕様
- リクエスト `application/x-www-form-urlencoded`
  - `title` 大会名
- レスポンス `application/json`
  - `competition`
    - `id` 大会ID
    - `title` 大会名
    - `is_finished` 終了しているかどうか

### POST `<tenant endpoint>/api/organizer/competition/:competition_id/finish`

大会を終了する  
大会終了後は新たな結果の入稿が出来ない  
それ以降のランキングや課金額には変動がないので結果をキャッシュ可能になる  

仕様
- リクエスト パスに含まれる
  - `competition_id`
- レスポンス
  - なし

### POST `<tenant endpoint>/api/organizer/competition/:competition_id/score`

大会結果CSVを入稿する  
最後に入稿されたCSVのみが反映される  
大会が終了するまでは何度でもリクエストできる

仕様
- リクエスト `multipart/form-data`
  - `scores`
    - CSVの内容
      - `player_id`  参加者の識別子
      - `score` 得点
  - `player_id`の重複は許容する
    - それぞれの `player_id` について、CSV上で最後に出現した行がランキングに採用される
- レスポンス `application/json`
  - `rows` 入稿したCSVの、ヘッダ行(1行)を除外した行数
  - 大会が終了していたらスコアを反映せずに400を返す

### GET `<tenant endpoint>/api/organizer/billing`

テナントの請求金額を返す  

仕様
- リクエスト
  - 無し
- レスポンス `application/json`
  - `reports` 配列
    - `competition_id`
    - `competition_title`
      - 以下の要素については、終了した大会のみ正しい数値を返す必要がある。開催中の大会については0を返してよい
      - `player_count` スコアを登録した参加者数
      - `visitor_count` ランキングを閲覧した(スコアを登録していない)参加者数
      - `billing_player_yen` スコアを登録した参加者数 * 100 (請求金額内訳)
      - `billing_visitor_yen` ランキングを閲覧した(スコアを登録していない)参加者数 * 10 (請求金額内訳)
      - `billing_yen` 大会ごとの請求額 (`billing_player_yen + billing_visitor_yen`)

### GET `<tenant endpoint>/api/organizer/competitions`

テナント内大会の一覧を返す  

仕様
- リクエスト
  - なし
- レスポンス `application/json`
  - `competitions` 配列
    - `id`
    - `title`
    - `is_finished` 大会が終了済かどうか

## 参加者向けAPI

### GET `<tenant endpoint>/api/player/player/:player_id`

参加者の情報と戦績(スコアを登録した全ての大会ごとのスコア)を取得する

仕様
- リクエスト パスに含まれる
  - `player_id` 参加者の識別子
- レスポンス `application/json`
  - `player`
    - `id` 参加者のID
    - `display_name` 参加者の表示名
    - `is_disqualified` 失格かどうか
  - `scores` 配列
    - `competition_title` 大会のタイトル
    - `score` この参加者が登録したスコア

### GET `<tenant endpoint>/api/player/competition/:competition_id/ranking`

大会内のランキングを返す

仕様
- リクエスト
  - `competition_id` パスに含まれる
  - `rank_after` query string
    - 型: int, optional
    - この順位より大きい順位の参加者のリストを出す
    - ページングに使用
- レスポンス `application/json`
  - `ranks` 配列 最大100。参加者ごとに登録されたスコアのうち一番大きい値で求められる
    - `rank` 順位。スコアが同一の場合は入稿したCSV上で先に出現したほうが上位(小さい値)になる。つまり`rank`が同一になることはない
    - `score`
    - `player_id` 参加者の識別子
    - `player_display_name` 参加者の表示名

### GET `<tenant endpoint>/api/player/competitions`

テナント内大会の一覧を返す

仕様
- リクエスト
  - なし
- レスポンス `application/json`
  - `competitions` 配列
    - `id`
    - `title`
    - `is_finished` 大会が終了しているかどうか

## 共通API

### GET `<tenant endpoint/admin endpoint>/api/me`

現在接続中のテナントや、認証に用いているユーザ情報を返す

仕様
- リクエスト
  - なし
- レスポンス `application/json`
  - `tenant`
    - `name` テナント名
    - `display_name` テナント表示名
  - `me` 認証に用いている参加者情報
    - `id`
    - `display_name`
    - `is_disqualified`
    - `role` ロール
      - `none` `admin` `organizer` `player` のいずれかが入る
      - いずれのroleでもログインしていない場合は `none`
    - `logged_in` ログインしているかどうか

## ベンチマーカー向けAPI

### POST `<admin endpoint>/initialize`

ベンチマーク実行前に必要な初期化プロセスを実行する

仕様
- リクエスト
  - なし
- レスポンス `application/json`
  - `lang` 実装言語 自己申告
