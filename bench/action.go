package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/isucon/isucandar/agent"
)

var globalPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

func GetInitializeAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	b, reset, err := newRequestBody(struct{}{})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/initialize", b)
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func GetRootAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	req, err := ag.GET("/")
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func newRequestBody(obj any) (*bytes.Buffer, func(), error) {
	b := globalPool.Get().(*bytes.Buffer)
	reset := func() {
		b.Reset()
		globalPool.Put(b)
	}
	if err := json.NewEncoder(b).Encode(obj); err != nil {
		reset()
		return nil, nil, err
	}
	return b, reset, nil
}
