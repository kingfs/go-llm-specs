package llmspecs

import "testing"

func TestModelDataGetters(t *testing.T) {
	m := &modelData{
		IDVal:         "test/model",
		NameVal:       "Test Model",
		ProviderVal:   "TestProvider",
		DescVal:       "A test model",
		DescCNVal:     "测试模型",
		TagList:       []string{"coding", "tool-use"},
		ContextLenVal: 100,
		MaxOutputVal:  50,
		FeaturesVal:   ModalityTextIn,
		AliasList:     []string{"tm"},
	}

	if m.ID() != "test/model" {
		t.Error("Getter ID fail")
	}
	if m.Name() != "Test Model" {
		t.Error("Getter Name fail")
	}
	if m.Provider() != "TestProvider" {
		t.Error("Getter Provider fail")
	}
	if m.Description() != "A test model" {
		t.Error("Getter Description fail")
	}
	if m.DescriptionCN() != "测试模型" {
		t.Error("Getter DescriptionCN fail")
	}
	if m.Family() != "" {
		t.Error("Getter Family default fail")
	}
	if m.Series() != "" {
		t.Error("Getter Series default fail")
	}
	if m.Summary() != "" {
		t.Error("Getter Summary default fail")
	}
	if m.ContextLength() != 100 {
		t.Error("Getter ContextLength fail")
	}
	if m.MaxOutput() != 50 {
		t.Error("Getter MaxOutput fail")
	}
	if !m.HasCapability(ModalityTextIn) {
		t.Error("Getter HasCapability fail")
	}
	if m.Aliases()[0] != "tm" {
		t.Error("Getter Aliases fail")
	}
	if !m.HasTag("coder") {
		t.Error("Getter HasTag fail")
	}
	card := m.Card()
	if card.ID != "test/model" || card.Name != "Test Model" || card.Provider != "TestProvider" {
		t.Error("Getter Card fail")
	}
}
