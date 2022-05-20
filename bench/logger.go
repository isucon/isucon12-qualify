package bench

import (
	"log"
	"os"
)

var (
	// 選手向け情報を出力するロガー
	ContestantLogger = log.New(os.Stdout, "", log.Ltime|log.Lmicroseconds)
	// 大会運営向け情報を出力するロガー
	AdminLogger = log.New(os.Stderr, "[ADMIN] ", log.Ltime|log.Lmicroseconds)
)
