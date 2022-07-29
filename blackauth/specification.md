# ブラックボックスマイクロサービス

### POST `<tenant endpoint>/auth/login/player`

人間がテナント内参加者ユーザの認証を行うためのエンドポイント  
署名されたJWTがセッションキーとして発行される

仕様
- リクエスト
  - name
    - 参加者の識別子 JWTのsubに入る
- レスポンス
  - Set-CookieにJWTが入る
    - 名前 `isuports_session`

### POST `<tenant endpoint>/auth/login/organizer`

人間がテナント内主催者ユーザの認証を行うためのエンドポイント  
署名されたJWTがセッションキーとして発行される

仕様
- リクエスト
  - name
    - organizerの識別子 JWTのsubに入る
- レスポンス
  - Set-Cookieni JWTが入る
    - 名前 `isuports_session`

### POST `<admin endpoint>/auth/login/admin`

人間がSaaS管理者の認証を行うためのエンドポイント  
署名されたJWTがセッションキーとして発行される

仕様
- リクエスト
  - なし
- レスポンス
  - Set-CookieにJWTが入る
  - 名前 `isuports_session`

### POST `(<admin endpoint>|<tenant endpoint>)/auth/logout`

人間がログアウトするためのエンドポイント  
Set-CookieでJWTをクリアするだけなので、全JWT共通でこれを使う

仕様
- リクエスト
  - なし
- レスポンス
  - Set-Cookieで1時間前にExpireしているクッキーを吐く
  - 名前 `isuports_session`
