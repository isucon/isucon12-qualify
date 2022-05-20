package bench

import (
	"fmt"
	"io"
	"os"

	"encoding/json"
)

type Model interface {
	// TODO: dataset jsonをunmarshalする先の構造体をここに入れる
}

func LoadFromJSONFile[T Model](jsonFile string) ([]*T, error) {
	// 引数に渡されたファイルを開く
	file, err := os.Open(jsonFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	objects := make([]*T, 0, 10000) // 大きく確保しておく
	// JSON 形式としてデコード
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&objects); err != nil {
		if err != io.EOF {
			return nil, fmt.Errorf("failed to decode json: %w", err)
		}
	}
	return objects, nil
}
