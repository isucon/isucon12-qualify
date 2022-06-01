package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/isucon/isucon12-qualify/bench"
	isuports "github.com/isucon/isucon12-qualify/webapp/go"
	"github.com/jaswdr/faker"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	_ "github.com/samber/lo"
)

var fake = faker.New()
var now = time.Now()
var epoch = time.Date(2022, 06, 01, 0, 0, 0, 0, time.UTC)
var playersNumByTenant = 1000
var competitionsNumByTenant = 100
var disqualifiedRate = 10
var totalTenants = int64(10)
var idByTenant = map[int64]int64{}
var tenantDBSchemaFilePath = "../webapp/sql/tenant/10_schema.sql"

func init() {
	os.Setenv("TZ", "UTC")
}

func main() {
	for i := int64(0); i <= totalTenants; i++ {
		log.Println("create tenant", i)
		tenant := createTenant(i)
		players := createPlayers(tenant)
		competitions := createCompetitions(tenant)
		playerScores := createPlayerScores(tenant, players, competitions)
		if err := storeTenant(tenant, players, competitions, playerScores); err != nil {
			panic(err)
		}
	}
}

var mu sync.Mutex
var idMap = map[int64]int64{}

func genID(ts time.Time) int64 {
	mu.Lock()
	defer mu.Unlock()
	diff := ts.Sub(epoch)
	id := int64(diff.Seconds())
	if idMap[id] == 0 {
		idMap[id] = 1
		return id*1000 + idMap[id]
	} else if idMap[id] < 999 {
		idMap[id]++
		return id*1000 + idMap[id]
	} else {
		panic(fmt.Sprintf("too many id at %s", ts))
	}
}

func storeTenant(tenant *isuports.TenantRow, players []*isuports.PlayerRow, competitions []*isuports.CompetitionRow, pss []*isuports.PlayerScoreRow) error {
	log.Println("store tenant", tenant.ID)
	cmd := exec.Command("sh", "-c", fmt.Sprintf("sqlite3 %s.db < %s", tenant.Name, tenantDBSchemaFilePath))
	if err := cmd.Run(); err != nil {
		return err
	}
	db, err := sqlx.Open("sqlite3", fmt.Sprintf("file:%s.db?mode=rw", tenant.Name))
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
		`INSERT INTO player (id, name, display_name, is_disqualified, created_at, updated_at)
		 VALUES (:id, :name, :display_name, :is_disqualified, :created_at, :updated_at)`,
		players,
	); err != nil {
		return err
	}
	if _, err := tx.NamedExec(
		`INSERT INTO competition (id, title, finished_at, created_at, updated_at)
		VALUES(:id, :title, :finished_at, :created_at, :updated_at)`,
		competitions,
	); err != nil {
		return err
	}
	for i, _ := range pss {
		if i > 0 && i%100 == 0 {
			if _, err := tx.NamedExec(
				`INSERT INTO player_score (id, player_id, competition_id, score, created_at, updated_at)
				VALUES(:id, :player_id, :competition_id, :score, :created_at, :updated_at)`,
				pss[i-100:i],
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func createTenant(id int64) *isuports.TenantRow {
	idByTenant[id] = 1
	name := fmt.Sprintf("tenant-%06d", id)
	created := fake.Time().TimeBetween(epoch, now.Add(-24*time.Hour))
	tenant := isuports.TenantRow{
		ID:          id,
		Name:        name,
		DisplayName: fake.Company().Name(),
		CreatedAt:   created,
		UpdatedAt:   fake.Time().TimeBetween(created, now),
	}
	return &tenant
}

func createPlayers(tenant *isuports.TenantRow) []*isuports.PlayerRow {
	playersNum := fake.IntBetween(playersNumByTenant/10, playersNumByTenant)
	players := make([]*isuports.PlayerRow, 0, playersNum)
	for i := 0; i < playersNum; i++ {
		players = append(players, createPlayer(tenant))
	}
	sort.SliceStable(players, func(i int, j int) bool {
		return players[i].CreatedAt.Before(players[j].CreatedAt)
	})
	return players
}

func createPlayer(tenant *isuports.TenantRow) *isuports.PlayerRow {
	created := fake.Time().TimeBetween(tenant.CreatedAt, now)
	player := isuports.PlayerRow{
		ID:             genID(created),
		Name:           bench.RandomString(fake.IntBetween(8, 16)),
		DisplayName:    fake.Person().Name(),
		IsDisqualified: rand.Intn(100) < disqualifiedRate,
		CreatedAt:      created,
		UpdatedAt:      fake.Time().TimeBetween(created, now),
	}
	return &player
}

func createCompetitions(tenant *isuports.TenantRow) []*isuports.CompetitionRow {
	num := fake.IntBetween(competitionsNumByTenant/10, competitionsNumByTenant)
	rows := make([]*isuports.CompetitionRow, 0, num)
	for i := 0; i < num; i++ {
		rows = append(rows, createCompetition(tenant))
	}
	sort.SliceStable(rows, func(i int, j int) bool {
		return rows[i].CreatedAt.Before(rows[j].CreatedAt)
	})
	return rows
}

func createCompetition(tenant *isuports.TenantRow) *isuports.CompetitionRow {
	created := fake.Time().TimeBetween(tenant.CreatedAt, now)
	isFinished := rand.Intn(100) < 50
	competition := isuports.CompetitionRow{
		ID:        genID(created),
		Title:     fake.Company().Name(),
		CreatedAt: created,
	}
	if isFinished {
		competition.FinishedAt = sql.NullTime{
			Time:  fake.Time().TimeBetween(created, now),
			Valid: true,
		}
		competition.UpdatedAt = competition.FinishedAt.Time
	} else {
		competition.UpdatedAt = fake.Time().TimeBetween(created, now)
	}
	return &competition
}

func createPlayerScores(tenant *isuports.TenantRow, players []*isuports.PlayerRow, competitions []*isuports.CompetitionRow) []*isuports.PlayerScoreRow {
	res := make([]*isuports.PlayerScoreRow, 0, len(players)*len(competitions))
	for _, c := range competitions {
		for _, p := range players {
			if c.FinishedAt.Valid && p.CreatedAt.After(c.FinishedAt.Time) {
				// 大会が終わったあとに登録したplayerはデータがない
				continue
			}
			var end time.Time
			if c.FinishedAt.Valid {
				end = c.FinishedAt.Time
			} else {
				end = now
			}
			created := fake.Time().TimeBetween(c.CreatedAt, end)
			res = append(res, &isuports.PlayerScoreRow{
				ID:            genID(created),
				PlayerID:      p.ID,
				CompetitionID: c.ID,
				Score:         fake.Int64Between(0, 1000000),
				CreatedAt:     created,
				UpdatedAt:     fake.Time().TimeBetween(created, end),
			})
		}
	}
	return res
}
