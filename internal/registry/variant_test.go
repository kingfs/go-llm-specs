package registry

import "testing"

func TestIsRoutingAlias(t *testing.T) {
	routing := []string{
		"~deepseek/deepseek-v4-flash-latest",
		"~openai/gpt-astra-latest",
	}
	for _, id := range routing {
		if !IsRoutingAlias(id) {
			t.Errorf("IsRoutingAlias(%q) = false, want true", id)
		}
	}
	models := []string{
		"",
		"deepseek/deepseek-v4-flash",
		"deepseek/deepseek-v4-flash-0731",
		"x-ai/grok-4.6",
		"google/gemma-3n-e2b-it",
		"openai/o3-pro",
		"openai/chatgpt-4o-latest",
		"openai/gpt-chat-latest",
	}
	for _, id := range models {
		if IsRoutingAlias(id) {
			t.Errorf("IsRoutingAlias(%q) = true, want false", id)
		}
	}
}

func TestIsDraftHead(t *testing.T) {
	drafts := []Model{
		{ID: "deepseek/deepseek-v4-flash-dspark"},
		{ID: "deepseek/deepseek-v4-pro-dspark"},
		{ID: "nvidia/kimi-k2.7-code-dflash"},
		{ID: "nvidia/kimi-k2.5-thinking-eagle3"},
		{ID: "nvidia/gpt-oss-120b-eagle3-v3"},
		{ID: "nvidia/nemotron-3-super-120b-a12b-bf16-mtpv2"},
		{ID: "nvidia/some-head", Description: "MTP head for speculative decoding. It is not a standalone language model."},
	}
	for _, m := range drafts {
		if !IsDraftHead(m) {
			t.Errorf("IsDraftHead(%q) = false, want true", m.ID)
		}
		if !IsServingVariant(m) {
			t.Errorf("IsServingVariant(%q) = false, want true", m.ID)
		}
	}
	standalone := []Model{
		{ID: "deepseek/deepseek-v4-flash"},
		{ID: "openai/o3-pro", Description: "uses more compute"},
		{ID: "x-ai/grok-4-fast", Description: "Identical capabilities with higher output speed"},
		{ID: "nvidia/nemotron-3-super-120b-a12b", Description: "supports speculative decoding at serving time"},
	}
	for _, m := range standalone {
		if IsDraftHead(m) {
			t.Errorf("IsDraftHead(%q) = true, want false", m.ID)
		}
		if IsServingVariant(m) {
			t.Errorf("IsServingVariant(%q) = true, want false", m.ID)
		}
	}
}
