package llmspecs

import "testing"

func TestModelDataGetters(t *testing.T) {
	m := &modelData{
		IDVal:         "test/model",
		NameVal:       "Test Model",
		ProviderVal:   "TestProvider",
		DescVal:       "A test model",
		DescCNVal:     "测试模型",
		ContextLenVal: 100,
		MaxOutputVal:  50,
		PriceInVal:    0.1,
		PriceOutVal:   0.2,
		Features:      ModalityTextIn,
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
	if m.ContextLength() != 100 {
		t.Error("Getter ContextLength fail")
	}
	if m.MaxOutput() != 50 {
		t.Error("Getter MaxOutput fail")
	}
	if m.PriceInput() != 0.1 {
		t.Error("Getter PriceInput fail")
	}
	if m.PriceOutput() != 0.2 {
		t.Error("Getter PriceOutput fail")
	}
	if !m.HasCapability(ModalityTextIn) {
		t.Error("Getter HasCapability fail")
	}
	if m.Aliases()[0] != "tm" {
		t.Error("Getter Aliases fail")
	}
}
