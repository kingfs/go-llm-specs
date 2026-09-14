package provider

import "testing"

func TestValidateIdentityStrategy(t *testing.T) {
	base := Provider{
		SchemaVersion: CurrentSchemaVersion,
		ID:            "example",
		Name:          "Example",
		Official:      Official{Homepage: "https://example.com/"},
	}

	publisher := base
	publisher.Identity = Identity{Strategy: IdentityStrategyPublisher}
	if err := publisher.Validate(); err == nil {
		t.Fatal("publisher strategy without an authoritative source must fail validation")
	}

	publisher.Organizations = Organizations{HuggingFace: []string{"example-org"}}
	if err := publisher.Validate(); err != nil {
		t.Fatalf("publisher strategy with a Hugging Face organization: %v", err)
	}

	apiOnly := base
	apiOnly.Official.API = "https://api.example.com/v1/models"
	apiOnly.Identity = Identity{Strategy: IdentityStrategyPublisher}
	if err := apiOnly.Validate(); err != nil {
		t.Fatalf("publisher strategy with an official API: %v", err)
	}

	unknown := base
	unknown.Identity = Identity{Strategy: "typo"}
	if err := unknown.Validate(); err == nil {
		t.Fatal("unknown identity strategy must fail validation")
	}

	aggregator := base
	aggregator.Identity = Identity{Strategy: IdentityStrategyAggregator}
	if err := aggregator.Validate(); err != nil {
		t.Fatalf("aggregator strategy: %v", err)
	}
}

func TestIdentityEffectiveStrategy(t *testing.T) {
	if got := (Identity{}).EffectiveStrategy(); got != IdentityStrategyAggregator {
		t.Fatalf("default strategy = %q, want %q", got, IdentityStrategyAggregator)
	}
	if got := (Identity{Strategy: IdentityStrategyPublisher}).EffectiveStrategy(); got != IdentityStrategyPublisher {
		t.Fatalf("declared strategy = %q", got)
	}
}
