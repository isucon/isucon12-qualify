package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jaswdr/faker"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/samber/lo"

	isuports "github.com/isucon/isucon12-qualify/webapp/go"
)

var fake = faker.New()

var OutDir = "."                                 // テナントDBの出力先ディレクトリ
var DatabaseDSN string                           // MySQLのDSN
var Now = func() time.Time { return defaultNow } // ベンチから使うときは上書きできるようにしておく
var NowUnix = func() int64 { return Now().Unix() }
var Epoch = time.Date(2022, 05, 01, 0, 0, 0, 0, time.UTC) // サービス開始時点(IDの起点)
var EpochUnix = Epoch.Unix()
var defaultNow = time.Date(2022, 05, 31, 23, 59, 59, 0, time.UTC) // 初期データの終点
var playersNumByTenant = 200                                      // テナントごとのplayer数
var competitionsNumByTenant = 20                                  // テナントごとの大会数
var disqualifiedRate = 10                                         // player失格確率
var visitsByCompetition = 75                                      // 1大会のplayerごとの訪問数
var scoresByCompetition = 100                                     // 1大会のplayerごとのスコアを出した数
var maxID int64                                                   // webapp初期化時の起点ID
var hugeTenantScale = 25                                          // 1個だけある巨大テナント データサイズ倍数
var tenantID int64

// テナントIDは連番で生成
var GenTenantID = func() int64 {
	return atomic.AddInt64(&tenantID, 1)
}

var tenantDBSchemaFilePath = "../webapp/sql/tenant/10_schema.sql"
var adminDBSchemaFilePath = "../webapp/sql/admin/10_schema.sql"

type BenchmarkerSource struct {
	TenantName     string `json:"tenant_name"`
	CompetitionID  string `json:"competition_id"`
	IsFinished     bool   `json:"is_finished"`
	PlayerID       string `json:"player_id"`
	IsDisqualified bool   `json:"is_disqualified"`
}

type BenchmarkerTenantSource struct {
	TenantID     int64                     `json:"tenant_id"`
	TenantName   string                    `json:"tenant_name"`
	Billing      int64                     `json:"billing"`
	Competitions []*BenchmarkerCompeittion `json:"competitions"`
}

type BenchmarkerCompeittion struct {
	ID      string `json:"id"`
	Billing int64  `json:"billing`
}

func init() {
	os.Setenv("TZ", "UTC")
	diff := Now().Add(time.Second).Sub(Epoch)
	maxID = int64(diff.Seconds()) * 10000
}

func Run(tenantsNum int) error {
	v := os.Getenv("ISUPORTS_DATA_HUGE_TENANT_SCALE")
	if v != "" {
		var err error
		hugeTenantScale, err = strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("error failed strconv.Atoi: %s", err)
		}
	}

	log.Println("tenantsNum", tenantsNum)
	log.Println("hugeTenantScale", hugeTenantScale)
	log.Println("epoch", Epoch)

	db, err := adminDB()
	if err != nil {
		return err
	}
	if err := loadSchema(db, adminDBSchemaFilePath); err != nil {
		return err
	}
	defer db.Close()

	benchSrcs := make([]*BenchmarkerSource, 0)
	benchTenantSrcs := make([]*BenchmarkerTenantSource, 0)
	for i := 0; i < tenantsNum; i++ {
		log.Println("create tenant")
		tt := TenantTagGeneral
		switch i {
		case 0:
			tt = TenantTagFirst
		case 1:
			tt = TenantTagSecond
		}
		tenant := CreateTenant(tt)
		players := CreatePlayers(tenant)
		competitions := CreateCompetitions(tenant)
		playerScores, visitHistroies, billing, benchComps, b := CreatePlayerData(tenant, players, competitions)
		if err := storeTenant(tenant, players, competitions, playerScores); err != nil {
			return err
		}
		if err := storeAdmin(db, tenant, visitHistroies); err != nil {
			return err
		}
		samples := len(players)
		benchSrcs = append(benchSrcs, lo.Samples(b, samples)...)
		benchTenantSrcs = append(benchTenantSrcs, &BenchmarkerTenantSource{
			TenantID:     tenant.ID,
			TenantName:   tenant.Name,
			Billing:      billing,
			Competitions: benchComps,
		})
	}
	if err := storeMaxID(db); err != nil {
		return err
	}
	if f, err := os.Create("benchmarker.json"); err != nil {
		return err
	} else {
		json.NewEncoder(f).Encode(benchSrcs)
		f.Close()
	}
	if f, err := os.Create("benchmarker_tenant.json"); err != nil {
		return err
	} else {
		json.NewEncoder(f).Encode(benchTenantSrcs)
		f.Close()
	}
	return nil
}

var mu sync.Mutex
var idMap = map[int64]int64{}
var generatedMaxID int64

var GenID = func(ts int64) string {
	return genID(ts)
}

func genID(ts int64) string {
	mu.Lock()
	defer mu.Unlock()
	id := ts - EpochUnix
	if id <= 0 {
		panic(fmt.Sprintf("generatedMaxID is smaller than 0: ts=%d", ts))
	}
	var newID int64
	if _, exists := idMap[id]; !exists {
		idMap[id] = fake.Int64Between(0, 99)
		newID = id*10000 + idMap[id]
	} else if idMap[id] < 9999 {
		idMap[id]++
		newID = id*10000 + idMap[id]
	} else {
		log.Fatalf("too many id at %d", ts)
	}
	if newID > generatedMaxID {
		generatedMaxID = newID
	}
	if generatedMaxID >= maxID {
		panic("generatedMaxID must be smaller than maxID")
	}
	return fmt.Sprintf("%x", newID)
}

func loadSchema(db *sqlx.DB, schemaFile string) error {
	schema, err := os.ReadFile(schemaFile)
	if err != nil {
		return err
	}
	queries := strings.Split(string(schema), ";")
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to exec query: %s %s", err, query)
		}
	}
	return nil
}

func adminDB() (*sqlx.DB, error) {
	if DatabaseDSN == "" {
		config := mysql.NewConfig()
		config.Net = "tcp"
		config.Addr = "127.0.0.1:3306"
		config.User = "isucon"
		config.Passwd = "isucon"
		config.DBName = "isuports"
		config.InterpolateParams = true
		config.Loc = time.UTC
		DatabaseDSN = config.FormatDSN()
	}
	log.Println("DatabaseDSN", DatabaseDSN)
	return sqlx.Open("mysql", DatabaseDSN)
}

func storeAdmin(db *sqlx.DB, tenant *isuports.TenantRow, visitHistories []*isuports.VisitHistoryRow) error {
	log.Println("store admin", tenant.ID)
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err = tx.NamedExec(
		`INSERT INTO tenant (id, name, display_name, created_at, updated_at)
		 VALUES (:id, :name, :display_name, :created_at, :updated_at)`,
		tenant,
	); err != nil {
		return err
	}

	var from int
	for i := range visitHistories {
		if i > 0 && i%1000 == 0 || i == len(visitHistories)-1 {
			fmt.Fprint(os.Stderr, ".")
			if _, err := tx.NamedExec(
				`INSERT INTO visit_history (player_id, tenant_id, competition_id, created_at, updated_at)
				VALUES(:player_id, :tenant_id, :competition_id, :created_at, :updated_at)`,
				visitHistories[from:i],
			); err != nil {
				return err
			}
			from = i
		}
	}
	fmt.Fprintln(os.Stderr, "")
	return tx.Commit()
}

func storeMaxID(db *sqlx.DB) error {
	if _, err := db.Exec(`REPLACE INTO id_generator (id, stub) VALUES (?, ?)`, maxID, "a"); err != nil {
		return err
	}
	return nil
}

func storeTenant(tenant *isuports.TenantRow, players []*isuports.PlayerRow, competitions []*isuports.CompetitionRow, pss []*isuports.PlayerScoreRow) error {
	filename := filepath.Join(OutDir, fmt.Sprintf("%d.db", tenant.ID))
	log.Println("store tenant", tenant.ID, "to", filename)
	os.Remove(filename)
	db, err := sqlx.Open("sqlite3", fmt.Sprintf("file:%s?mode=rwc&_synchronous=OFF", filename))
	if err != nil {
		return err
	}
	if err := loadSchema(db, tenantDBSchemaFilePath); err != nil {
		return err
	}
	defer db.Close()

	mustTx := func() *sqlx.Tx {
		tx, err := db.Beginx()
		if err != nil {
			panic(err)
		}
		return tx
	}
	tx := mustTx()
	if _, err = tx.NamedExec(
		`INSERT INTO player (tenant_id, id, display_name, is_disqualified, created_at, updated_at)
		 VALUES (:tenant_id, :id, :display_name, :is_disqualified, :created_at, :updated_at)`,
		players,
	); err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()

	tx = mustTx()
	if _, err := tx.NamedExec(
		`INSERT INTO competition (tenant_id, id, title, finished_at, created_at, updated_at)
		VALUES(:tenant_id, :id, :title, :finished_at, :created_at, :updated_at)`,
		competitions,
	); err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()

	var from int
	for i := range pss {
		if i > 0 && i%1000 == 0 || i == len(pss)-1 {
			fmt.Fprint(os.Stderr, ".")
			tx = mustTx()
			if _, err := tx.NamedExec(
				`INSERT INTO player_score (tenant_id, id, player_id, competition_id, score, row_num, created_at, updated_at)
				VALUES(:tenant_id, :id, :player_id, :competition_id, :score, :row_num, :created_at, :updated_at)`,
				pss[from:i],
			); err != nil {
				tx.Rollback()
				return err
			}
			tx.Commit()
			from = i
		}
	}
	fmt.Fprintln(os.Stderr, "")
	return nil
}

type TenantTag int

const (
	TenantTagGeneral TenantTag = iota
	TenantTagFirst
	TenantTagSecond
)

func CreateTenant(tag TenantTag) *isuports.TenantRow {
	id := GenTenantID()
	var created time.Time
	var name, displayName string
	switch tag {
	// 動作検証のための特別なテナント
	case TenantTagFirst:
		created = Epoch
		name, displayName = "isucon", "ISUコングロマリット"
	case TenantTagSecond:
		created = Epoch.Add(time.Minute * 5)
		name, displayName = "kayac", "KAYAC"
	case TenantTagGeneral:
		created = fake.Time().TimeBetween(
			Epoch.Add(time.Duration(id)*time.Hour*3),
			Epoch.Add(time.Duration(id+1)*time.Hour*3),
		)
		name = fmt.Sprintf("%s-%d", fake.Internet().Slug(), id)
		displayName = FakeTenantName()
	}
	tenant := isuports.TenantRow{
		ID:          id,
		Name:        name,
		DisplayName: displayName,
		CreatedAt:   created.Unix(),
		UpdatedAt:   fake.Time().TimeBetween(created, Now()).Unix(),
	}
	return &tenant
}

func CreatePlayers(tenant *isuports.TenantRow) []*isuports.PlayerRow {
	playersNum := fake.IntBetween(playersNumByTenant/10, playersNumByTenant)
	if tenant.ID == 1 {
		playersNum = playersNumByTenant * hugeTenantScale
	}
	log.Printf("create %d players for tenant %s", playersNum, tenant.Name)
	players := make([]*isuports.PlayerRow, 0, playersNum)
	for i := 0; i < playersNum; i++ {
		players = append(players, CreatePlayer(tenant))
	}
	if tenant.ID <= 2 { // id 1, 2 は特別に固定のplayerを作る
		players = append(players, CreateFixedPlayer(tenant))
	}
	sort.SliceStable(players, func(i int, j int) bool {
		return players[i].CreatedAt < players[j].CreatedAt
	})
	return players
}

func CreatePlayer(tenant *isuports.TenantRow) *isuports.PlayerRow {
	created := fake.Int64Between(tenant.CreatedAt, NowUnix())
	player := isuports.PlayerRow{
		TenantID:       tenant.ID,
		ID:             GenID(created),
		DisplayName:    fake.Person().Name(),
		IsDisqualified: rand.Intn(100) < disqualifiedRate,
		CreatedAt:      created,
		UpdatedAt:      fake.Int64Between(created, NowUnix()),
	}
	return &player
}

var fixedPlayerDisplayName = map[int64]string{
	1: "ISUコンボイ",
	2: "fujiwara組",
}

func CreateFixedPlayer(tenant *isuports.TenantRow) *isuports.PlayerRow {
	created := tenant.CreatedAt
	player := isuports.PlayerRow{
		TenantID:       tenant.ID,
		ID:             "000" + fmt.Sprintf("%x", tenant.ID),
		DisplayName:    fixedPlayerDisplayName[tenant.ID],
		IsDisqualified: false,
		CreatedAt:      created,
		UpdatedAt:      created,
	}
	return &player
}

func CreateCompetitions(tenant *isuports.TenantRow) []*isuports.CompetitionRow {
	var num int
	if tenant.ID == 1 {
		num = competitionsNumByTenant
	} else {
		num = fake.IntBetween(competitionsNumByTenant/10, competitionsNumByTenant)
	}
	log.Printf("create %d competitions for tenant %s", num, tenant.Name)
	rows := make([]*isuports.CompetitionRow, 0, num)
	for i := 0; i < num; i++ {
		rows = append(rows, CreateCompetition(tenant))
	}
	sort.SliceStable(rows, func(i int, j int) bool {
		return rows[i].CreatedAt < rows[j].CreatedAt
	})
	return rows
}

func CreateCompetition(tenant *isuports.TenantRow) *isuports.CompetitionRow {
	created := fake.Int64Between(tenant.CreatedAt, NowUnix())
	isFinished := rand.Intn(100) < 50
	competition := isuports.CompetitionRow{
		TenantID:  tenant.ID,
		ID:        GenID(created),
		Title:     FakeCompetitionName(),
		CreatedAt: created,
	}
	if isFinished {
		competition.FinishedAt = sql.NullInt64{
			Valid: true,
			Int64: fake.Int64Between(created, NowUnix()),
		}
		competition.UpdatedAt = competition.FinishedAt.Int64
	} else {
		competition.UpdatedAt = fake.Int64Between(created, NowUnix())
	}
	return &competition
}

func CreatePlayerData(
	tenant *isuports.TenantRow,
	players []*isuports.PlayerRow,
	competitions []*isuports.CompetitionRow,
) (
	[]*isuports.PlayerScoreRow,
	[]*isuports.VisitHistoryRow,
	int64, // 対象テナントにいるplayerに関連するBillingYen
	[]*BenchmarkerCompeittion,
	[]*BenchmarkerSource,
) {
	scores := make([]*isuports.PlayerScoreRow, 0, len(players)*len(competitions))
	visits := make([]*isuports.VisitHistoryRow, 0, len(players)*len(competitions)*visitsByCompetition)
	bench := make([]*BenchmarkerSource, 0, len(players)*len(competitions))

	bcs := []*BenchmarkerCompeittion{}
	tenantBilling := 0

	for _, c := range competitions {
		competitionScores := make([]*isuports.PlayerScoreRow, 0, len(players)*100)
		competitionBilling := 0
		for _, p := range players {
			if c.FinishedAt.Valid && c.FinishedAt.Int64 < p.CreatedAt {
				// 大会が終わったあとに登録したplayerはデータがない
				continue
			}
			var end int64
			if c.FinishedAt.Valid {
				end = c.FinishedAt.Int64
			} else {
				end = NowUnix()
			}
			yen := 0
			created := fake.Int64Between(c.CreatedAt, end)
			lastVisitedAt := fake.Int64Between(created, end)
			for i := 0; i < fake.IntBetween(visitsByCompetition/10, visitsByCompetition); i++ {
				visitedAt := fake.Int64Between(created, lastVisitedAt)
				visits = append(visits, &isuports.VisitHistoryRow{
					TenantID:      tenant.ID,
					PlayerID:      p.ID,
					CompetitionID: c.ID,
					CreatedAt:     visitedAt,
					UpdatedAt:     visitedAt,
				})
				yen = 10
			}
			if rand.Intn(100) < 90 { // 大会参加率90%
				for i := 0; i < fake.IntBetween(scoresByCompetition/10, scoresByCompetition); i++ {
					created := fake.Int64Between(c.CreatedAt, end)
					competitionScores = append(competitionScores, &isuports.PlayerScoreRow{
						TenantID:      tenant.ID,
						ID:            GenID(created),
						PlayerID:      p.ID,
						CompetitionID: c.ID,
						Score:         CreateScore(),
						CreatedAt:     created,
						UpdatedAt:     created,
					})
					yen = 100
				}
			}
			bench = append(bench, &BenchmarkerSource{
				TenantName:     tenant.Name,
				CompetitionID:  c.ID,
				PlayerID:       p.ID,
				IsFinished:     c.FinishedAt.Valid,
				IsDisqualified: p.IsDisqualified,
			})
			// 大会が終了済みの場合のみ加算
			if c.FinishedAt.Valid {
				competitionBilling += yen
			}
		}
		sort.Slice(competitionScores, func(i, j int) bool {
			return competitionScores[i].CreatedAt < competitionScores[j].CreatedAt
		})
		for i := range competitionScores {
			competitionScores[i].RowNum = int64(i + 1)
		}
		scores = append(scores, competitionScores...)

		bcs = append(bcs, &BenchmarkerCompeittion{
			ID:      c.ID,
			Billing: int64(competitionBilling),
		})
		tenantBilling += competitionBilling
	}

	log.Printf("tenant:%v billing:%v", tenant.Name, tenantBilling)
	return scores, visits, int64(tenantBilling), bcs, bench
}

func CreateScore() int64 {
	return fake.Int64Between(0, 100) * fake.Int64Between(0, 100) * fake.Int64Between(0, 100)
}
