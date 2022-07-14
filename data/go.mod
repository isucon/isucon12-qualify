module github.com/isucon/isucon12-qualify/data

go 1.18

replace github.com/isucon/isucon12-qualify/webapp/go => ../webapp/go

require (
	github.com/go-sql-driver/mysql v1.6.0
	github.com/isucon/isucon12-qualify/webapp/go v0.0.0-00010101000000-000000000000
	github.com/jaswdr/faker v1.10.2
	github.com/jmoiron/sqlx v1.3.5
	github.com/mattn/go-sqlite3 v1.14.13
	github.com/samber/lo v1.21.0
)

require (
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	github.com/goccy/go-json v0.9.7 // indirect
	github.com/gofrs/flock v0.8.1 // indirect
	github.com/golang-jwt/jwt v3.2.2+incompatible // indirect
	github.com/labstack/echo/v4 v4.7.2 // indirect
	github.com/labstack/gommon v0.3.1 // indirect
	github.com/lestrrat-go/blackmagic v1.0.1 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/httprc v1.0.1 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/jwx/v2 v2.0.2 // indirect
	github.com/lestrrat-go/option v1.0.0 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/shogo82148/go-sql-proxy v0.6.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.1 // indirect
	golang.org/x/crypto v0.0.0-20220525230936-793ad666bf5e // indirect
	golang.org/x/exp v0.0.0-20220609121020-a51bd0440498 // indirect
	golang.org/x/net v0.0.0-20220607020251-c690dde0001d // indirect
	golang.org/x/sys v0.0.0-20220608164250-635b8c9b7f68 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/time v0.0.0-20220609170525-579cf78fd858 // indirect
)
