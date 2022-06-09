package bench

import (
	"time"

	"github.com/isucon/isucon12-qualify/data"
)

func InitializeData() {
	// データ生成器の時刻をベンチ実行時点にする
	data.Now = time.Now
	data.Epoch = time.Now()
}
