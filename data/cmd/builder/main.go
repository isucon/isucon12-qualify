package main

import (
	"flag"
	"log"
	"os"
	"strconv"

	"github.com/isucon/isucon12-qualify/data"
)

func main() {
	os.Exit(run())
}

func run() int {
	flag.StringVar(&data.OutDir, "out-dir", ".", "Output directory")
	flag.StringVar(&data.DatabaseDSN, "db-dsn", "", "")
	flag.Parse()
	if len(flag.Args()) != 1 {
		log.Println("Usage: builder <tenants_num>")
		return 1
	}
	tenantsNum, err := strconv.Atoi(flag.Args()[0])
	if err != nil {
		log.Println("Invalid tenants_num:", err)
		return 1
	}
	if err := data.Run(tenantsNum); err != nil {
		log.Println("ERROR:", err)
		return 1
	}
	return 0
}
