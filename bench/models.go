package bench

import (
	"sync"

	"github.com/isucon/isucandar/agent"
)

// TODO: ユーザーの挙動みたいなのがここに入る、たぶん
type Tenant struct {
	// `id` BIGINT UNSIGNED NOT NULL,
	// `identifier` VARCHAR(191) NOT NULL,
	// `name` VARCHAR(191) NOT NULL,
	// `image` LONGBLOB NOT NULL,

	mu    sync.RWMutex
	Agent *agent.Agent
}
type Tenants []*Tenant

func (t *Tenant) GetAgent(opt Option) (*agent.Agent, error) {
	t.mu.RLock()
	ag := t.Agent
	t.mu.RUnlock()
	if ag != nil {
		return ag, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	ag, err := opt.NewAgent(false)
	if err != nil {
		return nil, err
	}
	t.Agent = ag
	return ag, nil
}

type Competition struct {
	// `id` INTEGER NOT NULL PRIMARY KEY,
	// `title` TEXT NOT NULL,
}
type Competitions []*Competition

type Competitor struct {
	// `id` INTEGER PRIMARY KEY,
	// `identifier` TEXT NOT NULL UNIQUE,
	// `name` TEXT NOT NULL,
}
type Competitors []*Competitor
