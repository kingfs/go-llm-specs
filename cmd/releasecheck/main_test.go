package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSnapshot = `package llmspecs

func init() {
	staticRegistry = map[string]*modelData{
		"openai/a": {
			IDVal:         "openai/a",
			NameVal:       "A",
			ProviderVal:   "OpenAI",
			DescVal:       "desc a",
			DescCNVal:     "描述 a",
			ContextLenVal: 128000,
			MaxOutputVal:  4096,
			FeaturesVal:   CapChat | ModalityTextIn | ModalityTextOut,
			AliasList:     []string{"a", "alias-a"},
		},
		"openai/b": {
			IDVal:         "openai/b",
			NameVal:       "B",
			ProviderVal:   "OpenAI",
			DescVal:       "desc b",
			DescCNVal:     "",
			ContextLenVal: 64000,
			MaxOutputVal:  2048,
			FeaturesVal:   0,
			AliasList:     []string{},
		},
	}
	aliasIndex = map[string]string{}
}
`

func TestParseSnapshot(t *testing.T) {
	models, err := parseSnapshot([]byte(testSnapshot))
	if err != nil {
		t.Fatal(err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	model := models["openai/a"]
	if model.Name != "A" {
		t.Fatalf("expected name A, got %q", model.Name)
	}
	if len(model.Features) != 3 {
		t.Fatalf("unexpected features: %#v", model.Features)
	}
	if len(model.Aliases) != 2 {
		t.Fatalf("unexpected aliases: %#v", model.Aliases)
	}
}

func TestCompareSnapshotsReleasesOnCapacityChanges(t *testing.T) {
	base := map[string]snapshotModel{
		"openai/a": {ID: "openai/a", Name: "A", Provider: "OpenAI", ContextLen: 1, MaxOutput: 2},
	}
	current := map[string]snapshotModel{
		"openai/a": {ID: "openai/a", Name: "A", Provider: "OpenAI", ContextLen: 10, MaxOutput: 20},
	}

	rep := compareSnapshots("v0.1.0", current, base)
	if !rep.ReleaseNeeded {
		t.Fatalf("expected release for capacity changes: %#v", rep)
	}
	if len(rep.UpdatedModels) != 1 {
		t.Fatalf("expected significant capacity diff, got %#v", rep.UpdatedModels)
	}
}

func TestCompareSnapshotsReleasesOnDescriptionCNChange(t *testing.T) {
	base := map[string]snapshotModel{
		"openai/a": {ID: "openai/a", Name: "A", Provider: "OpenAI", DescriptionCN: ""},
	}
	current := map[string]snapshotModel{
		"openai/a": {ID: "openai/a", Name: "A", Provider: "OpenAI", DescriptionCN: "中文"},
	}

	rep := compareSnapshots("v0.1.0", current, base)
	if !rep.ReleaseNeeded {
		t.Fatalf("expected release for translation change: %#v", rep)
	}
	if len(rep.UpdatedModels) != 1 || rep.UpdatedModels[0].ChangedFields[0] != "description_cn" {
		t.Fatalf("unexpected updated models: %#v", rep.UpdatedModels)
	}
}

func TestWriteGitHubOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "github_output.txt")
	rep := report{
		BaseTag:        "v0.1.0",
		NextTag:        "v0.1.1",
		ReleaseNeeded:  true,
		ReleaseReason:  "1 new model(s)",
		ReleaseSummary: "summary",
		ReleaseBody:    "body",
		AddedModels:    []string{"openai/a"},
	}

	if err := writeGitHubOutput(path, rep); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "release_needed=true") {
		t.Fatalf("unexpected github output: %s", string(content))
	}
}
