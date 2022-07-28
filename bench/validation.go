package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/failure"
	isuports "github.com/isucon/isucon12-qualify/webapp/go"
)

// failure.NewError で用いるエラーコード定義
const (
	ErrValidation failure.StringCode = "load-validation"
)

type ValidationError struct {
	Errors   []error
	Title    string
	Canceled bool
}

// error インターフェースを満たす Error メソッド
func (v ValidationError) Error() string {
	messages := []string{}

	for _, err := range v.Errors {
		if err != nil {
			messages = append(messages, fmt.Sprintf("%v", err))
		}
	}

	return fmt.Sprintf("error: %s ", v.Title) + strings.Join(messages, "\n")
}

// ValidationError が空かを判定
// NOTE: contextのキャンセルによって返されるValidationErrorは、IsEmptyはfalseでErrorsは空
func (v ValidationError) IsEmpty() bool {
	if v.Canceled {
		return false
	}
	return len(v.Errors) == 0
}

// レスポンスを検証するバリデータ関数の型
type ResponseValidator func(*Response) error

// Bodyを繰り返し取れるように先に読んでStringとして置いておく
type Response struct {
	Response *http.Response // Bodyは読み取り済み
	Body     string
}

func ReadResponse(res *http.Response) *Response {
	var data []byte
	if res.Body != nil {
		data, _ = io.ReadAll(res.Body)
		defer res.Body.Close()
	}
	return &Response{
		Body:     string(data),
		Response: res,
	}
}

// レスポンスを検証する関数
// 複数のバリデータ関数を受け取ってすべてでレスポンスを検証し、 ValidationError を返す
func ValidateResponse(title string, step *isucandar.BenchmarkStep, res *http.Response, err error, validators ...ResponseValidator) ValidationError {
	return ValidateResponseWithMsg(title, step, res, err, "", validators...)
}
func ValidateResponseWithMsg(title string, step *isucandar.BenchmarkStep, res *http.Response, err error, msg string, validators ...ResponseValidator) ValidationError {
	ve := ValidationError{
		Title: title,
	}
	defer func() {
		ve.Add(step)
	}()
	if err != nil {
		if failure.Is(err, context.DeadlineExceeded) || failure.Is(err, context.Canceled) {
			// ベンチが終了したタイミングのerrは無視してよい
			ve.Canceled = true
			return ve
		}
		// リクエストがエラーだったらそれ以上の検証はしない(できない)
		ve.Errors = append(ve.Errors, fmt.Errorf("%s %s", err, msg))
		ContestantLogger.Print(ve.Error())

		return ve
	} else {
		if Debug {
			AdminLogger.Printf("%s %s %d %s", res.Request.Method, res.Request.URL.Path, res.StatusCode, title)
		}
	}

	response := ReadResponse(res)
	for _, v := range validators {
		if err := v(response); err != nil {
			ve.Errors = append(ve.Errors, fmt.Errorf("%s %s", err, msg))
			break // 前から順に検証、失敗したらそれ以上の検証はしない
		}
	}

	if !ve.IsEmpty() {
		ContestantLogger.Print(ve.Error())
	}
	return ve
}

func WithCacheControlPrivate() ResponseValidator {
	return func(r *Response) error {
		if !strings.Contains(r.Response.Header.Get("Cache-Control"), "private") {
			return fmt.Errorf("Cache-Control: private が含まれていません")
		}
		return nil
	}
}

func WithBodySameFile(filePath string) ResponseValidator {
	return func(r *Response) error {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}

		if r.Body != string(data) {
			return fmt.Errorf("file body mismatch. local filepath: %s", filePath)
		}
		return nil
	}
}

// ステータスコードコードを検証するバリデータ関数を返す高階関数
// 例: ValidateResponse(res, WithStatusCode(200, 304))
func WithStatusCode(statusCodes ...int) ResponseValidator {
	return func(r *Response) error {
		for _, statusCode := range statusCodes {
			// 1個でも一致したらok
			if r.Response.StatusCode == statusCode {
				return nil
			}
		}
		// ステータスコードが一致しなければ HTTP メソッド、URL パス、期待したステータスコード、実際のステータスコードを持つ
		// エラーを返す
		return fmt.Errorf(
			"%s %s : expected(%v) != actual(%d)",
			r.Response.Request.Method,
			r.Response.Request.URL.Path,
			statusCodes,
			r.Response.StatusCode,
		)
	}
}

func (v ValidationError) Add(step *isucandar.BenchmarkStep) {
	for _, err := range v.Errors {
		if err != nil {
			// 中身が ValidationError なら展開
			if ve, ok := err.(ValidationError); ok {
				ve.Add(step)
			} else {
				e := failure.NewError(ErrValidation, err)
				step.AddError(e)
			}
		}
	}
}

type ResponseAPIBase struct {
	Status  bool   `json:"status"`
	Message string `json:"message,omitempty"`
}

func (r ResponseAPIBase) IsSuccess() bool {
	return r.Status
}

func (r ResponseAPIBase) ErrorMessage() string {
	return r.Message
}

type ResponseAPI interface {
	ResponseAPIBase |
		ResponseAPITenantsAdd | ResponseAPITenantsBilling | ResponseAPIPlayersAdd | ResponseAPIPlayersList |
		ResponseAPIPlayerDisqualified | ResponseAPICompetitionsAdd | ResponseAPIBilling |
		ResponseAPIPlayer | ResponseAPICompetitionRanking | ResponseAPICompetitions |
		ResponseAPIInitialize | ResponseAPICompetitionRankingFinish | ResponseAPICompetitionResult
	IsSuccess() bool
	ErrorMessage() string
}

func WithSuccessResponse[T ResponseAPI](validates ...func(res T) error) ResponseValidator {
	return func(r *Response) error {
		var v T
		if err := json.NewDecoder(strings.NewReader(r.Body)).Decode(&v); err != nil {
			if failure.Is(err, context.DeadlineExceeded) || failure.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("JSONのdecodeに失敗しました %s %s status %d", r.Response.Request.Method, r.Response.Request.URL.Path, r.Response.StatusCode)
		}
		if !v.IsSuccess() {
			return fmt.Errorf("成功したAPIレスポンスの.statusはtrueである必要があります %s %s status %d", r.Response.Request.Method, r.Response.Request.URL.Path, r.Response.StatusCode)
		}
		for _, validate := range validates {
			if err := validate(v); err != nil {
				b, _ := json.Marshal(v)
				AdminLogger.Println(string(b))
				return fmt.Errorf("%s %s %s", r.Response.Request.Method, r.Response.Request.URL.Path, err.Error())
			}
		}
		return nil
	}
}

func WithContentType(wantContentType string) ResponseValidator {
	return func(r *Response) error {
		ct := r.Response.Header.Get("Content-Type")
		if !strings.Contains(ct, wantContentType) {
			return fmt.Errorf("Content-Typeが違います (want:%+v got:%+v) %s %s status:%d body:%s", wantContentType, ct, r.Response.Request.Method, r.Response.Request.URL.Path, r.Response.StatusCode, r.Body)
		}
		return nil
	}
}

func WithErrorResponse[T ResponseAPI]() ResponseValidator {
	return func(r *Response) error {
		var v T
		if err := json.NewDecoder(strings.NewReader(r.Body)).Decode(&v); err != nil {
			if failure.Is(err, context.DeadlineExceeded) || failure.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("JSONのdecodeに失敗しました %s %s status:%d body:%s", r.Response.Request.Method, r.Response.Request.URL.Path, r.Response.StatusCode, r.Body)
		}
		if v.IsSuccess() {
			return fmt.Errorf("失敗したAPIレスポンスの.statusはfalseである必要があります %s %s %d", r.Response.Request.Method, r.Response.Request.URL.Path, r.Response.StatusCode)
		}
		if v.ErrorMessage() == "" {
			return fmt.Errorf("失敗したAPIレスポンスの.errorにはエラーメッセージが必要です %s %s %d", r.Response.Request.Method, r.Response.Request.URL.Path, r.Response.StatusCode)
		}
		return nil
	}
}

type ResponseAPITenantsAdd struct {
	ResponseAPIBase
	Data isuports.TenantsAddHandlerResult `json:"data"`
}
type ResponseAPITenantsBilling struct {
	ResponseAPIBase
	Data isuports.TenantsBillingHandlerResult `json:"data"`
}
type ResponseAPIPlayersAdd struct {
	ResponseAPIBase
	Data isuports.PlayersAddHandlerResult `json:"data"`
}
type ResponseAPIPlayersList struct {
	ResponseAPIBase
	Data isuports.PlayersListHandlerResult `json:"data"`
}
type ResponseAPIPlayerDisqualified struct {
	ResponseAPIBase
	Data isuports.PlayerDisqualifiedHandlerResult `json:"data"`
}
type ResponseAPICompetitionsAdd struct {
	ResponseAPIBase
	Data isuports.CompetitionsAddHandlerResult `json:"data"`
}
type ResponseAPIBilling struct {
	ResponseAPIBase
	Data isuports.BillingHandlerResult `json:"data"`
}
type ResponseAPIPlayer struct {
	ResponseAPIBase
	Data isuports.PlayerHandlerResult `json:"data"`
}
type ResponseAPICompetitionRanking struct {
	ResponseAPIBase
	Data isuports.CompetitionRankingHandlerResult `json:"data"`
}
type ResponseAPICompetitions struct {
	ResponseAPIBase
	Data isuports.CompetitionsHandlerResult `json:"data"`
}
type ResponseAPIInitialize struct {
	ResponseAPIBase
	Data isuports.InitializeHandlerResult `json:"data"`
}
type ResponseAPICompetitionRankingFinish struct {
	ResponseAPIBase
}
type ResponseAPICompetitionResult struct {
	ResponseAPIBase
	Data isuports.ScoreHandlerResult `json:"data"`
}
