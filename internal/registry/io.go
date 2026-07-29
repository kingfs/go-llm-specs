package registry

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func Decode(data []byte) (Model, error) {
	var model Model
	if err := yaml.Unmarshal(data, &model); err != nil {
		return Model{}, err
	}
	return model, nil
}

func Scan(root string) ([]Model, error) {
	var models []Model
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
			return nil
		}
		model, err := Load(path)
		if err != nil {
			return err
		}
		if model.ID != "" {
			models = append(models, model)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
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
