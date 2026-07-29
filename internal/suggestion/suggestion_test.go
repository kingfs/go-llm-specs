package suggestion

import (
	"encoding/json"
	"testing"
	"time"
)

func TestValidateRejectsWrongClaimType(t *testing.T) {
	document := Document{
		SchemaVersion: 1, Kind: "model_card", ModelID: "qwen/test", Status: "pending", CreatedAt: time.Now(),
		Claims: []Claim{{Field: "features", Value: json.RawMessage(`"tool use"`), Evidence: "supports tools", Confidence: "high"}},
	}
	if err := document.Validate(); err == nil {
		t.Fatal("expected wrong feature type to fail")
	}
}
