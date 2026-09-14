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

	unbacked := base
	unbacked.Organizations = Organizations{HuggingFace: []string{"example-org"}}
	unbacked.Identity = Identity{RequireCorroboration: true}
	if err := unbacked.Validate(); err == nil {
		t.Fatal("corroboration without publisher strategy must fail validation")
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

func TestValidateOfficialAPI(t *testing.T) {
	base := Provider{
		SchemaVersion: CurrentSchemaVersion, ID: "example", Name: "Example",
		Official: Official{Homepage: "https://example.com/"},
	}

	noURL := base
	noURL.Identity = Identity{Strategy: IdentityStrategyPublisher, OfficialAPI: &OfficialAPI{}}
	if err := noURL.Validate(); err == nil {
		t.Fatal("official API without a URL must fail validation")
	}

	unknownAuth := base
	unknownAuth.Identity = Identity{Strategy: IdentityStrategyPublisher, OfficialAPI: &OfficialAPI{URL: "https://example.com/models", Auth: "basic"}}
	if err := unknownAuth.Validate(); err == nil {
		t.Fatal("unknown official API auth must fail validation")
	}

	queryWithoutParam := base
	queryWithoutParam.Identity = Identity{Strategy: IdentityStrategyPublisher, OfficialAPI: &OfficialAPI{URL: "https://example.com/models", Auth: OfficialAPIAuthQuery}}
	if err := queryWithoutParam.Validate(); err == nil {
		t.Fatal("query auth without a query parameter must fail validation")
	}

	valid := base
	valid.Identity = Identity{Strategy: IdentityStrategyPublisher, OfficialAPI: &OfficialAPI{
		URL: "https://example.com/models", Env: "EXAMPLE_API_KEY", Auth: OfficialAPIAuthQuery,
		QueryParam: "key", ItemsPath: "models", IDField: "name", IDPrefix: "models/",
	}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid official API config: %v", err)
	}
	if !valid.HasAuthoritativeSource() {
		t.Fatal("an official API is an authoritative source")
	}
}

func TestOfficialAPIDefaults(t *testing.T) {
	api := OfficialAPI{}
	if api.EffectiveAuth() != OfficialAPIAuthNone {
		t.Fatalf("default auth = %q", api.EffectiveAuth())
	}
	if api.EffectiveIDField() != "id" {
		t.Fatalf("default id field = %q", api.EffectiveIDField())
	}
}
