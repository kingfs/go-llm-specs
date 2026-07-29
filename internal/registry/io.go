package registry

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func Decode(data []byte) (Model, error) {
	var model Model
	if err := yaml.Unmarshal(data, &model); err != nil {
		return Model{}, err
	}
	return model, nil
}

func Encode(model Model) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(model); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Load(path string) (Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Model{}, err
	}
	model, err := Decode(data)
	if err != nil {
		return Model{}, fmt.Errorf("decode %s: %w", path, err)
	}
	model.FilePath = path
	return model, nil
}

func Save(path string, model Model) error {
	data, err := Encode(model)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return os.WriteFile(path, data, 0o644)
}
