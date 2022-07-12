package bench

import (
	"time"

	"github.com/isucon/isucon12-qualify/data"
)

func InitializeData() {
	// データ生成器の時刻をベンチ実行時点にする
	data.Now = time.Now
	data.Epoch = time.Now()
	data.GenID = func(ts int64) string { return "" }             // ベンチ中はID生成はサーバでしかしない
	data.GenTenantID = func() int64 { return time.Now().Unix() } // ベンチ中はunix timeでダミーID生成(実際のIDはサーバ発番)
}
