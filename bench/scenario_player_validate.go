package bench

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
	"github.com/isucon/isucon12-qualify/data"
	isuports "github.com/isucon/isucon12-qualify/webapp/go"
)

type playerValidateScenarioWorker struct {
	worker *worker.Worker
}

func (playerValidateScenarioWorker) String() string {
	return "PlayerValidateScenarioWorker"
}
func (w *playerValidateScenarioWorker) Process(ctx context.Context) { w.worker.Process(ctx) }

// PlayerHandlerが不正な値を返さないかチェックする
func (sc *Scenario) PlayerValidateScenarioWorker(step *isucandar.BenchmarkStep, p int32) (Worker, error) {
	scTag := ScenarioTagPlayerValidate

	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.PlayerValidateScenario(ctx, step, scTag); err != nil {
			sc.ScenarioError(scTag, err)
			SleepWithCtx(ctx, SleepOnError)
		}
	},
		// 無限回繰り返す
		worker.WithInfinityLoop(),
		worker.WithMaxParallelism(1),
	)
	if err != nil {
		return nil, err
	}
	w.SetParallelism(p)
	return &playerValidateScenarioWorker{
		worker: w,
	}, nil
}

func (sc *Scenario) PlayerValidateScenario(ctx context.Context, step *isucandar.BenchmarkStep, scTag ScenarioTag) error {
	report := timeReporter(string(scTag))
	defer report()
	sc.ScenarioStart(scTag)

	// 初期データからテナントを選ぶ
	index := randomRange(ConstPlayerValidateScenarioIDRange)
	tenant := sc.InitialDataTenant[index]

	orgAc, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenant.TenantName, "organizer")
	if err != nil {
		return err
	}

	playerIDs := []string{}
	disqualifiedPlayerIDs := []string{}
	{
		res, err, txt := GetOrganizerPlayersListAction(ctx, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("テナントのプレイヤー一覧取得", step, res, err, msg, WithStatusCode(200),
			WithContentType("application/json"),
			WithSuccessResponse(func(r ResponseAPIPlayersList) error {
				for _, pl := range r.Data.Players {
					if pl.IsDisqualified {
						disqualifiedPlayerIDs = append(disqualifiedPlayerIDs, pl.ID)
					} else {
						playerIDs = append(playerIDs, pl.ID)
					}
				}
				if len(playerIDs) < 2 {
					return fmt.Errorf("テナント内のプレイヤーが不足しています。")
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETOrganizerPlayersList, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	// チェック用の大会を開催し、初期状態のスコアを入稿する
	comp := &CompetitionData{
		Title: data.FakeCompetitionName(),
	}

	{
		res, err, txt := PostOrganizerCompetitionsAddAction(ctx, comp.Title, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("新規大会追加", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionsAdd) error {
				comp.ID = r.Data.Competition.ID
				return nil
			}))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionsAdd, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddCriticalCount() // OrganizerAPI 更新系はCritical Error
			return v
		}
	}

	score := ScoreRows{}
	for i, id := range playerIDs {
		if 100 <= i {
			break
		}
		score = append(score, &ScoreRow{
			PlayerID: id,
			Score:    1000 + i,
		})
	}
	{
		csv := score.CSV()
		res, err, txt := PostOrganizerCompetitionScoreAction(ctx, comp.ID, []byte(csv), orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("大会結果CSV入稿", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionResult) error {
				_ = r
				if r.Data.Rows != int64(len(score)) {
					return fmt.Errorf("入稿したCSVの行数が正しくありません %d != %d", r.Data.Rows, len(score))
				}
				return nil
			}))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionScore, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddCriticalCount() // OrganizerAPI 更新系はCritical Error
			return v
		}
	}

	checkerPlayerID := playerIDs[0]
	disqualifyingdPlayerID := playerIDs[1]

	checkerPlayerAc, checkerPlayerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenant.TenantName, checkerPlayerID)
	if err != nil {
		return err
	}
	disqualifyingdPlayerAc, disqualifyingdPlayerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenant.TenantName, disqualifyingdPlayerID)
	if err != nil {
		return err
	}

	// GETPlayer
	//		IsDisqualifyが更新されていること
	//		Scores更新
	// GETPlayerRanking
	//		ranks
	//		score
	// GETPlayerCompetitions
	//		competitions
	//			IsFinished

	// cacheさせる
	{
		pid := checkerPlayerID
		res, err, txt := GetPlayerAction(ctx, pid, checkerPlayerAg)
		msg := fmt.Sprintf("%s %s", checkerPlayerAc, txt)
		v := ValidateResponseWithMsg("参加者と戦績情報取得", step, res, err, msg, WithStatusCode(200),
			WithContentType("application/json"),
			WithSuccessResponse(func(r ResponseAPIPlayer) error {
				if r.Data.Player.ID != pid {
					return fmt.Errorf("PlayerIDが違います (want: %s, got: %s)", pid, r.Data.Player.ID)
				}
				if false != r.Data.Player.IsDisqualified {
					return fmt.Errorf("失格状態が違います playerID: %s (want: %v, got: %v)", pid, false, r.Data.Player.IsDisqualified)
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerDetails, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}
	{
		pid := disqualifyingdPlayerID
		res, err, txt := GetPlayerAction(ctx, pid, checkerPlayerAg)
		msg := fmt.Sprintf("%s %s", checkerPlayerAc, txt)
		v := ValidateResponseWithMsg("参加者と戦績情報取得", step, res, err, msg, WithStatusCode(200),
			WithContentType("application/json"),
			WithSuccessResponse(func(r ResponseAPIPlayer) error {
				if r.Data.Player.ID != pid {
					return fmt.Errorf("PlayerIDが違います (want: %s, got: %s)", pid, r.Data.Player.ID)
				}
				if false != r.Data.Player.IsDisqualified {
					return fmt.Errorf("失格状態が違います playerID: %s (want: %v, got: %v)", pid, false, r.Data.Player.IsDisqualified)
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerDetails, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	var beforeRanks []isuports.CompetitionRank
	{
		res, err, txt := GetPlayerCompetitionRankingAction(ctx, comp.ID, "", checkerPlayerAg)
		msg := fmt.Sprintf("%s %s", checkerPlayerAc, txt)
		v := ValidateResponseWithMsg("大会ランキング確認", step, res, err, msg, WithStatusCode(200),
			WithContentType("application/json"),
			WithSuccessResponse(func(r ResponseAPICompetitionRanking) error {
				if false != r.Data.Competition.IsFinished {
					return fmt.Errorf("大会の開催状態が違います CompetitionID: %s (want: %v, got: %v)", comp.ID, false, r.Data.Competition.IsFinished)
				}
				beforeRanks = r.Data.Ranks
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerRanking, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}
	{
		res, err, txt := GetPlayerCompetitionsAction(ctx, checkerPlayerAg)
		msg := fmt.Sprintf("%s %s", checkerPlayerAc, txt)
		v := ValidateResponseWithMsg("大会一覧確認", step, res, err, msg, WithStatusCode(200),
			WithContentType("application/json"),
			WithSuccessResponse(func(r ResponseAPICompetitions) error {
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerCompetitions, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	// disqualify後に403になる
	{
		pid := disqualifyingdPlayerID
		res, err, txt := GetPlayerAction(ctx, pid, disqualifyingdPlayerAg)
		msg := fmt.Sprintf("%s %s", disqualifyingdPlayerAc, txt)
		v := ValidateResponseWithMsg("参加者と戦績情報取得", step, res, err, msg, WithStatusCode(200))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerDetails, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}
	{
		res, err, txt := GetPlayerCompetitionRankingAction(ctx, comp.ID, "", disqualifyingdPlayerAg)
		msg := fmt.Sprintf("%s %s", checkerPlayerAc, txt)
		v := ValidateResponseWithMsg("大会ランキング確認", step, res, err, msg, WithStatusCode(200))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerRanking, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}
	{
		res, err, txt := GetPlayerCompetitionsAction(ctx, disqualifyingdPlayerAg)
		msg := fmt.Sprintf("%s %s", disqualifyingdPlayerAc, txt)
		v := ValidateResponseWithMsg("大会一覧確認", step, res, err, msg, WithStatusCode(200))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerCompetitions, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	// 失格状態を更新する
	// チェックは3秒後
	{
		pid := disqualifyingdPlayerID
		res, err, txt := PostOrganizerApiPlayerDisqualifiedAction(ctx, pid, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("プレイヤーを失格にする", step, res, err, msg, WithStatusCode(200),
			WithContentType("application/json"),
			WithCacheControlPrivate(),
			WithSuccessResponse(func(r ResponseAPIPlayerDisqualified) error {
				if !r.Data.Player.IsDisqualified {
					return fmt.Errorf("プレイヤーが失格になっていません player.id: %s", r.Data.Player.ID)
				}
				if disqualifyingdPlayerID != r.Data.Player.ID {
					return fmt.Errorf("失格にしたプレイヤーが違います (want: %s, got: %s)", disqualifyingdPlayerID, r.Data.Player.ID)
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerPlayerDisqualified, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddCriticalCount()
			return v
		}
	}
	disqualifyTicker := time.After(time.Second * 3)

	// スコアを入稿する
	// 初期スコアの逆順
	score = ScoreRows{}
	for i, id := range playerIDs {
		if 100 <= i {
			break
		}
		score = append(score, &ScoreRow{
			PlayerID: id,
			Score:    1000 - i,
		})
	}
	{
		csv := score.CSV()
		res, err, txt := PostOrganizerCompetitionScoreAction(ctx, comp.ID, []byte(csv), orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("大会結果CSV入稿", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionResult) error {
				_ = r
				if r.Data.Rows != int64(len(score)) {
					return fmt.Errorf("入稿したCSVの行数が正しくありません %d != %d", r.Data.Rows, len(score))
				}
				return nil
			}))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionScore, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddCriticalCount() // OrganizerAPI 更新系はCritical Error
			return v
		}
	}

	// rankingが更新されていることを確認する
	{
		res, err, txt := GetPlayerCompetitionRankingAction(ctx, comp.ID, "", checkerPlayerAg)
		msg := fmt.Sprintf("%s %s", checkerPlayerAc, txt)
		v := ValidateResponseWithMsg("大会ランキング確認", step, res, err, msg, WithStatusCode(200),
			WithContentType("application/json"),
			WithSuccessResponse(func(r ResponseAPICompetitionRanking) error {
				if false != r.Data.Competition.IsFinished {
					return fmt.Errorf("大会の開催状態が違います CompetitionID: %s (want: %v, got: %v)", comp.ID, false, r.Data.Competition.IsFinished)
				}
				if len(score) != len(r.Data.Ranks) {
					return fmt.Errorf("大会のランキング数が違います CompetitionID: %s (want: %v, got: %v)", comp.ID, len(score), len(r.Data.Ranks))
				}
				if diff := cmp.Diff(beforeRanks, r.Data.Ranks); diff == "" {
					return fmt.Errorf("大会のランキングが更新されていません CompetitionID:%s", comp.ID)
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerRanking, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	// 大会を終了する
	{
		res, err, txt := PostOrganizerCompetitionFinishAction(ctx, comp.ID, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("大会終了", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRankingFinish) error {
				_ = r
				return nil
			}))

		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionFinish, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddCriticalCount() // OrganizerAPI 更新系はCritical Error
			return v
		}
	}

	// 大会が終了されていることを確認する
	// NOTE: finishの3秒猶予はbillingにのみなのでそれ以外は即時反映されている必要がある
	{
		res, err, txt := GetPlayerCompetitionsAction(ctx, checkerPlayerAg)
		msg := fmt.Sprintf("%s %s", checkerPlayerAc, txt)
		v := ValidateResponseWithMsg("大会一覧確認", step, res, err, msg, WithStatusCode(200),
			WithContentType("application/json"),
			WithSuccessResponse(func(r ResponseAPICompetitions) error {
				// TODO
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerCompetitions, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	// 失格操作は3秒の猶予がある
	select {
	case <-ctx.Done():
	case <-disqualifyTicker:
	}
	// 失格が反映されていることをチェック
	{
		pid := disqualifyingdPlayerID
		res, err, txt := GetPlayerAction(ctx, pid, checkerPlayerAg)
		msg := fmt.Sprintf("%s %s", checkerPlayerAc, txt)
		v := ValidateResponseWithMsg("参加者と戦績情報取得", step, res, err, msg, WithStatusCode(200),
			WithContentType("application/json"),
			WithSuccessResponse(func(r ResponseAPIPlayer) error {
				if r.Data.Player.ID != pid {
					return fmt.Errorf("PlayerIDが違います (want: %s, got: %s)", pid, r.Data.Player.ID)
				}
				if true != r.Data.Player.IsDisqualified {
					return fmt.Errorf("失格状態が違います playerID: %s (want: %v, got: %v)", pid, true, r.Data.Player.IsDisqualified)
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerDetails, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}
	{
		pid := disqualifyingdPlayerID
		res, err, txt := GetPlayerAction(ctx, pid, disqualifyingdPlayerAg)
		msg := fmt.Sprintf("%s %s", disqualifyingdPlayerAc, txt)
		v := ValidateResponseWithMsg("参加者と戦績情報取得", step, res, err, msg, WithStatusCode(403))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerDetails, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddCriticalCount()
			return v
		}
	}
	{
		res, err, txt := GetPlayerCompetitionRankingAction(ctx, comp.ID, "", disqualifyingdPlayerAg)
		msg := fmt.Sprintf("%s %s", checkerPlayerAc, txt)
		v := ValidateResponseWithMsg("大会ランキング確認", step, res, err, msg, WithStatusCode(403))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerRanking, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddCriticalCount()
			return v
		}
	}
	{
		res, err, txt := GetPlayerCompetitionsAction(ctx, disqualifyingdPlayerAg)
		msg := fmt.Sprintf("%s %s", disqualifyingdPlayerAc, txt)
		v := ValidateResponseWithMsg("大会一覧確認", step, res, err, msg, WithStatusCode(403))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerCompetitions, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddCriticalCount()
			return v
		}
	}

	return nil
}
