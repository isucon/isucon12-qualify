package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"strconv"
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

var Now = func() time.Time { return defaultNow }                  // ベンチから使うときは上書きできるようにしておく
var Epoch = time.Date(2022, 05, 01, 0, 0, 0, 0, time.UTC)         // サービス開始時点(IDの起点)
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
			return fmt.Errorf("error failed strconv.Atoi", err)
		}
	}

	log.Println("tenantsNum", tenantsNum)
	log.Println("hugeTenantScale", hugeTenantScale)
	log.Println("epoch", Epoch)

	cmd := exec.Command("sh", "-c", fmt.Sprintf("mysql -uisucon -pisucon --host 127.0.0.1 isuports < %s", adminDBSchemaFilePath))
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Println(string(out))
		return err
	}

	db, err := adminDB()
	if err != nil {
		return err
	}
	defer db.Close()
	benchSrcs := make([]*BenchmarkerSource, 0)
	for i := 0; i < tenantsNum; i++ {
		log.Println("create tenant")
		tenant := CreateTenant(i == 0)
		players := CreatePlayers(tenant)
		competitions := CreateCompetitions(tenant)
		playerScores, visitHistroies, b := CreatePlayerData(tenant, players, competitions)
		if err := storeTenant(tenant, players, competitions, playerScores); err != nil {
			return err
		}
		if err := storeAdmin(db, tenant, visitHistroies); err != nil {
			return err
		}
		samples := len(players)
		benchSrcs = append(benchSrcs, lo.Samples(b, samples)...)
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
	return nil
}

var mu sync.Mutex
var idMap = map[int64]int64{}
var generatedMaxID int64

var GenID = func(ts time.Time) int64 {
	return genID(ts)
}

func genID(ts time.Time) int64 {
	mu.Lock()
	defer mu.Unlock()
	diff := ts.Sub(Epoch)
	id := int64(diff.Seconds())
	var newID int64
	if _, exists := idMap[id]; !exists {
		idMap[id] = fake.Int64Between(0, 99)
		newID = id*10000 + idMap[id]
	} else if idMap[id] < 9999 {
		idMap[id]++
		newID = id*10000 + idMap[id]
	} else {
		log.Fatalf("too many id at %s", ts)
	}
	if newID > generatedMaxID {
		generatedMaxID = newID
	}
	if generatedMaxID >= maxID {
		panic("generatedMaxID must be smaller than maxID")
	}
	return newID
}

func adminDB() (*sqlx.DB, error) {
	config := mysql.NewConfig()
	config.Net = "tcp"
	config.Addr = "127.0.0.1:3306"
	config.User = "isucon"
	config.Passwd = "isucon"
	config.DBName = "isuports"
	config.ParseTime = true
	config.Loc = time.UTC

	return sqlx.Open("mysql", config.FormatDSN())
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
	return tx.Commit()
}

func storeMaxID(db *sqlx.DB) error {
	if _, err := db.Exec(`REPLACE INTO id_generator (id, stub) VALUES (?, ?)`, maxID, "a"); err != nil {
		return err
	}
	return nil
}

func storeTenant(tenant *isuports.TenantRow, players []*isuports.PlayerRow, competitions []*isuports.CompetitionRow, pss []*isuports.PlayerScoreRow) error {
	log.Println("store tenant", tenant.ID)
	os.Remove(tenant.Name + ".db")
	cmd := exec.Command("sh", "-c", fmt.Sprintf("sqlite3 %d.db < %s", tenant.ID, tenantDBSchemaFilePath))
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Println(string(out))
		return err
	}
	db, err := sqlx.Open("sqlite3", fmt.Sprintf("file:%d.db?mode=rw&_journal_mode=OFF", tenant.ID))
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err = tx.NamedExec(
		`INSERT INTO player (tenant_id, id, display_name, is_disqualified, created_at, updated_at)
		 VALUES (:tenant_id, :id, :display_name, :is_disqualified, :created_at, :updated_at)`,
		players,
	); err != nil {
		return err
	}
	if _, err := tx.NamedExec(
		`INSERT INTO competition (tenant_id, id, title, finished_at, created_at, updated_at)
		VALUES(:tenant_id, :id, :title, :finished_at, :created_at, :updated_at)`,
		competitions,
	); err != nil {
		return err
	}
	var from int
	for i := range pss {
		if i > 0 && i%1000 == 0 || i == len(pss)-1 {
			if _, err := tx.NamedExec(
				`INSERT INTO player_score (tenant_id, id, player_id, competition_id, score, row_number, created_at, updated_at)
				VALUES(:tenant_id, :id, :player_id, :competition_id, :score, :row_number, :created_at, :updated_at)`,
				pss[from:i],
			); err != nil {
				return err
			}
			from = i
		}
	}
	return tx.Commit()
}

func CreateTenant(isFirst bool) *isuports.TenantRow {
	id := GenTenantID()
	var created time.Time
	var name, displayName string
	if isFirst {
		// これだけ特別
		created = Epoch
		name, displayName = "isucon", "ISUCONglomerate"
	} else {
		created = fake.Time().TimeBetween(
			Epoch.Add(time.Duration(id)*time.Hour*3),
			Epoch.Add(time.Duration(id+1)*time.Hour*3),
		)
		name = fmt.Sprintf("%s-%d", fake.Internet().Slug(), id)
		displayName = fake.Company().Name()
	}
	tenant := isuports.TenantRow{
		ID:          id,
		Name:        name,
		DisplayName: displayName,
		CreatedAt:   created,
		UpdatedAt:   fake.Time().TimeBetween(created, Now()),
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
	sort.SliceStable(players, func(i int, j int) bool {
		return players[i].CreatedAt.Before(players[j].CreatedAt)
	})
	return players
}

func CreatePlayer(tenant *isuports.TenantRow) *isuports.PlayerRow {
	created := fake.Time().TimeBetween(tenant.CreatedAt, Now())
	player := isuports.PlayerRow{
		TenantID:       tenant.ID,
		ID:             strconv.FormatInt(GenID(created), 10),
		DisplayName:    fake.Person().Name(),
		IsDisqualified: rand.Intn(100) < disqualifiedRate,
		CreatedAt:      created,
		UpdatedAt:      fake.Time().TimeBetween(created, Now()),
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
		return rows[i].CreatedAt.Before(rows[j].CreatedAt)
	})
	return rows
}

func CreateCompetition(tenant *isuports.TenantRow) *isuports.CompetitionRow {
	created := fake.Time().TimeBetween(tenant.CreatedAt, Now())
	isFinished := rand.Intn(100) < 50
	competition := isuports.CompetitionRow{
		TenantID:  tenant.ID,
		ID:        strconv.FormatInt(GenID(created), 10),
		Title:     fake.Music().Name(),
		CreatedAt: created,
	}
	if isFinished {
		competition.FinishedAt = sql.NullTime{
			Time:  fake.Time().TimeBetween(created, Now()),
			Valid: true,
		}
		competition.UpdatedAt = competition.FinishedAt.Time
	} else {
		competition.UpdatedAt = fake.Time().TimeBetween(created, Now())
	}
	return &competition
}

func CreatePlayerData(
	tenant *isuports.TenantRow,
	players []*isuports.PlayerRow,
	competitions []*isuports.CompetitionRow,
) ([]*isuports.PlayerScoreRow, []*isuports.VisitHistoryRow, []*BenchmarkerSource) {
	scores := make([]*isuports.PlayerScoreRow, 0, len(players)*len(competitions))
	visits := make([]*isuports.VisitHistoryRow, 0, len(players)*len(competitions)*visitsByCompetition)
	bench := make([]*BenchmarkerSource, 0, len(players)*len(competitions))
	for _, c := range competitions {
		competitionScores := make([]*isuports.PlayerScoreRow, 0, len(players)*100)
		for _, p := range players {
			if c.FinishedAt.Valid && p.CreatedAt.After(c.FinishedAt.Time) {
				// 大会が終わったあとに登録したplayerはデータがない
				continue
			}
			var end time.Time
			if c.FinishedAt.Valid {
				end = c.FinishedAt.Time
			} else {
				end = Now()
			}
			created := fake.Time().TimeBetween(c.CreatedAt, end)
			lastVisitedAt := fake.Time().TimeBetween(created, end)
			for i := 0; i < fake.IntBetween(visitsByCompetition/10, visitsByCompetition); i++ {
				visitedAt := fake.Time().TimeBetween(created, lastVisitedAt)
				visits = append(visits, &isuports.VisitHistoryRow{
					TenantID:      tenant.ID,
					PlayerID:      p.ID,
					CompetitionID: c.ID,
					CreatedAt:     visitedAt,
					UpdatedAt:     visitedAt,
				})
			}
			for i := 0; i < fake.IntBetween(scoresByCompetition-(scoresByCompetition/10), scoresByCompetition+(scoresByCompetition/10)); i++ {
				created := fake.Time().TimeBetween(c.CreatedAt, end)
				competitionScores = append(competitionScores, &isuports.PlayerScoreRow{
					TenantID:      tenant.ID,
					ID:            GenID(created),
					PlayerID:      p.ID,
					CompetitionID: c.ID,
					Score:         CreateScore(),
					CreatedAt:     created,
					UpdatedAt:     created,
				})
			}
			bench = append(bench, &BenchmarkerSource{
				TenantName:     tenant.Name,
				CompetitionID:  c.ID,
				PlayerID:       p.ID,
				IsFinished:     c.FinishedAt.Valid,
				IsDisqualified: p.IsDisqualified,
			})
		}
		sort.Slice(competitionScores, func(i, j int) bool {
			return competitionScores[i].CreatedAt.Before(competitionScores[j].CreatedAt)
		})
		for i := range competitionScores {
			competitionScores[i].RowNumber = int64(i + 1)
		}
		scores = append(scores, competitionScores...)
	}
	return scores, visits, bench
}

func CreateScore() int64 {
	return fake.Int64Between(0, 100) * fake.Int64Between(0, 100) * fake.Int64Between(0, 100)
}
