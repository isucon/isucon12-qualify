package main

import (
	"database/sql"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/isucon/isucon12-qualify/bench"
	isuports "github.com/isucon/isucon12-qualify/webapp/go"
	"github.com/jaswdr/faker"
	_ "github.com/samber/lo"
)

var fake = faker.New()
var now = time.Now()
var epoch = now.Add(-365 * 24 * time.Hour) // 1 year
var playersNumByTenant = 1000
var competitionsNumByTenant = 100
var disqualifiedRate = 10
var totalTenants = int64(10)
var idByTenant = map[int64]int64{}

func main() {
	for i := int64(0); i <= totalTenants; i++ {
		tenant := createTenant(i)
		players := createPlayers(tenant)
		competitions := createCompetitions(tenant)
		for _, p := range players {
			fmt.Println(p.ID)
		}
		for _, c := range competitions {
			fmt.Println(c.ID)
		}
	}
}

var mu sync.Mutex

func genIDByTenant(tenant *isuports.TenantRow) int64 {
	mu.Lock()
	defer mu.Unlock()
	idByTenant[tenant.ID]++
	return idByTenant[tenant.ID]*totalTenants + tenant.ID
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
	for _, p := range players {
		p.ID = genIDByTenant(tenant)
	}
	return players
}

func createPlayer(tenant *isuports.TenantRow) *isuports.PlayerRow {
	created := fake.Time().TimeBetween(tenant.CreatedAt, now)
	player := isuports.PlayerRow{
		Name: fmt.Sprintf("%s_%d",
			bench.RandomString(fake.IntBetween(8, 16)),
		),
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
	for _, r := range rows {
		r.ID = genIDByTenant(tenant)
	}
	return rows
}

func createCompetition(tenant *isuports.TenantRow) *isuports.CompetitionRow {
	created := fake.Time().TimeBetween(tenant.CreatedAt, now)
	isFinished := rand.Intn(100) < 50
	competition := isuports.CompetitionRow{
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
