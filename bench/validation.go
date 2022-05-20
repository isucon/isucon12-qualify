package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/failure"
)

// failure.NewError で用いるエラーコード定義
const (
	ErrInvalidStatusCode   failure.StringCode = "status-code"
	ErrInvalidCacheControl failure.StringCode = "cache-control"
	ErrInvalidJSON         failure.StringCode = "broken-json"
	ErrInvalidPath         failure.StringCode = "path"
	ErrFailed              failure.StringCode = "failed"
	ErrValidation          failure.StringCode = "validation"
)

type ValidationError struct {
	Errors []error
	Title  string
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
func (v ValidationError) IsEmpty() bool {
	return len(v.Errors) == 0
}

// レスポンスを検証するバリデータ関数の型
type ResponseValidator func(*http.Response) error

// レスポンスを検証する関数
// 複数のバリデータ関数を受け取ってすべてでレスポンスを検証し、 ValidationError を返す
func ValidateResponse(title string, step *isucandar.BenchmarkStep, res *http.Response, err error, validators ...ResponseValidator) ValidationError {
	ve := ValidationError{}
	ve.Title = title
	defer func() {
		ve.Add(step)
	}()
	if err != nil {
		if failure.Is(err, context.DeadlineExceeded) || failure.Is(err, context.Canceled) {
			// ベンチが終了したタイミングのerrは無視してよい
			return ve
		}
		// リクエストがエラーだったらそれ以上の検証はしない(できない)
		ve.Errors = append(ve.Errors, failure.NewError(ErrInvalidRequest, err))
		ContestantLogger.Print(ve.Error())
		return ve
	} else {
		if Debug {
			AdminLogger.Printf("%s %s %d %s", res.Request.Method, res.Request.URL.Path, res.StatusCode, title)
		}
		defer res.Body.Close()
	}
	for _, v := range validators {
		if err := v(res); err != nil {
			ve.Errors = append(ve.Errors, failure.NewError(ErrValidation, err))
			break // 前から順に検証、失敗したらそれ以上の検証はしない
		}
	}
	if !ve.IsEmpty() {
		ContestantLogger.Print(ve.Error())
	}
	return ve
}

func WithCacheControlPrivate() ResponseValidator {
	return func(r *http.Response) error {
		if !strings.Contains(r.Header.Get("Cache-Control"), "private") {
			return failure.NewError(ErrInvalidCacheControl, fmt.Errorf("Cache-Control: private が含まれていません"))
		}
		return nil
	}
}

// ステータスコードコードを検証するバリデータ関数を返す高階関数
// 例: ValidateResponse(res, WithStatusCode(200, 304))
func WithStatusCode(statusCodes ...int) ResponseValidator {
	return func(r *http.Response) error {
		for _, statusCode := range statusCodes {
			// 1個でも一致したらok
			if r.StatusCode == statusCode {
				return nil
			}
		}
		// ステータスコードが一致しなければ HTTP メソッド、URL パス、期待したステータスコード、実際のステータスコードを持つ
		// エラーを返す
		return failure.NewError(
			ErrInvalidStatusCode,
			fmt.Errorf(
				"%s %s : expected(%v) != actual(%d)",
				r.Request.Method,
				r.Request.URL.Path,
				statusCodes,
				r.StatusCode,
			),
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
				step.AddError(err)
			}
		}
	}
}

type ResponseAPIBase struct {
	Result bool   `json:"result"`
	Status int    `json:"status"`
	Error  string `json:"error"`
}

func (r ResponseAPIBase) IsSuccess() bool {
	return r.Result
}

func (r ResponseAPIBase) ErrorMessage() string {
	return r.Error
}

type ResponseAPI interface {
	ResponseAPIBase
	IsSuccess() bool
	ErrorMessage() string
}

func WithSuccessResponse[T ResponseAPI](validates ...func(res T) error) ResponseValidator {
	return func(r *http.Response) error {
		var v T
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			if failure.Is(err, context.DeadlineExceeded) || failure.Is(err, context.Canceled) {
				return nil
			}
			return failure.NewError(
				ErrInvalidJSON,
				fmt.Errorf("JSONのdecodeに失敗しました %s %s %s status %d", err, r.Request.Method, r.Request.URL.Path, r.StatusCode),
			)
		}
		if !v.IsSuccess() {
			return failure.NewError(
				ErrFailed,
				fmt.Errorf("成功したAPIレスポンスの.resultはtrueである必要があります %s %s status %d", r.Request.Method, r.Request.URL.Path, r.StatusCode),
			)
		}
		for _, validate := range validates {
			if err := validate(v); err != nil {
				b, _ := json.Marshal(v)
				AdminLogger.Println(string(b))
				return failure.NewError(
					ErrFailed,
					fmt.Errorf("%s %s %s", r.Request.Method, r.Request.URL.Path, err.Error()),
				)
			}
		}
		return nil
	}
}

func WithErrorResponse[T ResponseAPI]() ResponseValidator {
	return func(r *http.Response) error {
		var v T
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			if failure.Is(err, context.DeadlineExceeded) || failure.Is(err, context.Canceled) {
				return nil
			}
			return failure.NewError(
				ErrInvalidJSON,
				fmt.Errorf("JSONのdecodeに失敗しました %s %s status %d", r.Request.Method, r.Request.URL.Path, r.StatusCode),
			)
		}
		if v.IsSuccess() {
			return failure.NewError(
				ErrFailed,
				fmt.Errorf("失敗したAPIレスポンスの.resultはfalseである必要があります %s %s %d", r.Request.Method, r.Request.URL.Path, r.StatusCode),
			)
		}
		if v.ErrorMessage() == "" {
			return failure.NewError(
				ErrFailed,
				fmt.Errorf("失敗したAPIレスポンスの.errorにはエラーメッセージが必要です %s %s %d", r.Request.Method, r.Request.URL.Path, r.StatusCode),
			)
		}
		return nil
	}
}
