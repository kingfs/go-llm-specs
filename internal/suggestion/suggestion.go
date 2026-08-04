package suggestion

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const CurrentSchemaVersion = 1

type Document struct {
	SchemaVersion int       `json:"schema_version"`
	Kind          string    `json:"kind"`
	ModelID       string    `json:"model_id"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	Source        Source    `json:"source"`
	Generator     Generator `json:"generator"`
	Claims        []Claim   `json:"claims"`
}

type Source struct {
	URL      string `json:"url"`
	Revision string `json:"revision,omitempty"`
	SHA256   string `json:"sha256"`
}

type Generator struct {
	Model   string `json:"model"`
	WireAPI string `json:"wire_api"`
}

type Claim struct {
	Field      string          `json:"field"`
	Value      json.RawMessage `json:"value"`
	Evidence   string          `json:"evidence"`
	Section    string          `json:"section,omitempty"`
	Confidence string          `json:"confidence"`
}

func (d Document) Validate() error {
	if d.SchemaVersion != CurrentSchemaVersion || d.ModelID == "" || d.Kind == "" {
		return fmt.Errorf("invalid suggestion identity")
	}
	if d.Status != "pending" && d.Status != "accepted" && d.Status != "partially_accepted" && d.Status != "rejected" {
		return fmt.Errorf("invalid suggestion status %q", d.Status)
	}
	for i, claim := range d.Claims {
		if claim.Field == "" || len(claim.Value) == 0 || claim.Evidence == "" {
			return fmt.Errorf("claim %d is missing field, value, or evidence", i)
		}
		if !json.Valid(claim.Value) {
			return fmt.Errorf("claim %d has invalid JSON value", i)
		}
		if claim.Confidence != "high" && claim.Confidence != "medium" && claim.Confidence != "low" {
			return fmt.Errorf("claim %d has invalid confidence %q", i, claim.Confidence)
		}
		if err := ValidateClaim(claim); err != nil {
			return fmt.Errorf("claim %d: %w", i, err)
		}
	}
	return nil
}

// ValidateClaim validates the field-specific value type of one AI claim.
func ValidateClaim(claim Claim) error {
	switch claim.Field {
	case "description", "reasoning.parser", "codex.shell_type", "codex.apply_patch_tool_type":
		var value string
		if json.Unmarshal(claim.Value, &value) != nil || value == "" {
			return fmt.Errorf("%s must be a non-empty string", claim.Field)
		}
	case "context_length", "max_output":
		var value int
		if json.Unmarshal(claim.Value, &value) != nil || value <= 0 {
			return fmt.Errorf("%s must be a positive integer", claim.Field)
		}
	case "reasoning.supported", "codex.enabled", "codex.supports_parallel_tool_calls":
		var value bool
		if json.Unmarshal(claim.Value, &value) != nil {
			return fmt.Errorf("%s must be a boolean", claim.Field)
		}
	case "features", "serving.vllm_args", "serving.sglang_args", "codex.slugs", "codex.input_modalities":
		var values []string
		if json.Unmarshal(claim.Value, &values) != nil {
			return fmt.Errorf("%s must be a string array", claim.Field)
		}
	default:
		return fmt.Errorf("unsupported field %q", claim.Field)
	}
	return nil
}

func Save(path string, document Document) error {
	if err := document.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func Load(path string) (Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	var document Document
	if err := json.Unmarshal(data, &document); err != nil {
		return Document{}, err
	}
	return document, document.Validate()
}
