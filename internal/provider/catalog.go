package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const CurrentSchemaVersion = 1

// Provider describes a model publisher and deterministic places where its
// releases can be discovered. Provider files are human-maintained facts.
type Provider struct {
	SchemaVersion int           `yaml:"schema_version" json:"schema_version"`
	ID            string        `yaml:"id" json:"id"`
	Name          string        `yaml:"name" json:"name"`
	Kind          string        `yaml:"kind,omitempty" json:"kind,omitempty"`
	Aliases       []string      `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Official      Official      `yaml:"official" json:"official"`
	Organizations Organizations `yaml:"organizations,omitempty" json:"organizations,omitempty"`
	Discovery     Discovery     `yaml:"discovery,omitempty" json:"discovery,omitempty"`
	Identity      Identity      `yaml:"identity,omitempty" json:"identity,omitempty"`
}

// Identity declares who owns a provider's model identity. Strategy
// "publisher" means the provider's own official API or organization
// repositories are authoritative and OpenRouter is only a discovery/fallback
// feed. Strategy "aggregator" (the default) means OpenRouter is authoritative
// because the provider has no first-party machine-readable source.
type Identity struct {
	Strategy        string `yaml:"strategy,omitempty" json:"strategy,omitempty"`
	CanonicalPrefix string `yaml:"canonical_prefix,omitempty" json:"canonical_prefix,omitempty"`
	// RequireCorroboration holds newly discovered OpenRouter records as
	// lifecycle candidates until a first-party source confirms them. It is
	// opt-in because publishers that also ship closed models have no repository
	// to corroborate against.
	RequireCorroboration bool `yaml:"require_corroboration,omitempty" json:"require_corroboration,omitempty"`
	// OfficialAPI declares a first-party model list used to corroborate
	// identity. Acquisition records the official identifier and, when a link
	// template is given, the official link. Attribute enrichment stays with the
	// existing sources.
	OfficialAPI *OfficialAPI `yaml:"official_api,omitempty" json:"official_api,omitempty"`
}

// OfficialAPI describes a first-party model-list endpoint. The shape is
// declarative because closed vendors differ in authentication and payload:
//
//	openai:    bearer token,   items under "data",  id field "id"
//	anthropic: x-api-key,      items under "data",  id field "id"
//	google:    query key,      items under "models", id field "name"
//	xai:       bearer token,   items under "data",  id field "id"
type OfficialAPI struct {
	URL string `yaml:"url" json:"url"`
	// Env names the environment variable holding the API key. When it is empty
	// the endpoint is treated as public. A configured Env whose value is unset
	// makes acquisition skip the provider instead of failing.
	Env string `yaml:"env,omitempty" json:"env,omitempty"`
	// Auth is bearer, x-api-key, query or none (default none).
	Auth       string            `yaml:"auth,omitempty" json:"auth,omitempty"`
	QueryParam string            `yaml:"query_param,omitempty" json:"query_param,omitempty"`
	Headers    map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	// ItemsPath is the response field holding the model array; empty means the
	// response is the array itself.
	ItemsPath string `yaml:"items_path,omitempty" json:"items_path,omitempty"`
	// IDField is the field holding the model identifier (default "id").
	IDField string `yaml:"id_field,omitempty" json:"id_field,omitempty"`
	// IDPrefix is stripped from every official identifier (for example the
	// "models/" prefix returned by the Gemini API).
	IDPrefix string `yaml:"id_prefix,omitempty" json:"id_prefix,omitempty"`
	// LinkTemplate builds an official link from {id} when the vendor serves
	// per-model pages.
	LinkTemplate string `yaml:"link_template,omitempty" json:"link_template,omitempty"`
}

const (
	IdentityStrategyPublisher  = "publisher"
	IdentityStrategyAggregator = "aggregator"
)

// Official API authentication modes.
const (
	OfficialAPIAuthNone   = "none"
	OfficialAPIAuthBearer = "bearer"
	OfficialAPIAuthHeader = "x-api-key"
	OfficialAPIAuthQuery  = "query"
)

// EffectiveAuth returns the declared auth mode, defaulting to none.
func (a OfficialAPI) EffectiveAuth() string {
	if a.Auth == "" {
		return OfficialAPIAuthNone
	}
	return a.Auth
}

// EffectiveIDField returns the declared identifier field, defaulting to "id".
func (a OfficialAPI) EffectiveIDField() string {
	if a.IDField == "" {
		return "id"
	}
	return a.IDField
}

// EffectiveStrategy returns the declared strategy, defaulting to aggregator.
func (i Identity) EffectiveStrategy() string {
	if i.Strategy == "" {
		return IdentityStrategyAggregator
	}
	return i.Strategy
}

// HasAuthoritativeSource reports whether the provider declares a first-party
// place where model identities can be verified.
func (p Provider) HasAuthoritativeSource() bool {
	if len(p.Organizations.HuggingFace) > 0 || len(p.Organizations.ModelScope) > 0 || p.Official.API != "" {
		return true
	}
	return p.Identity.OfficialAPI != nil && strings.TrimSpace(p.Identity.OfficialAPI.URL) != ""
}

type Official struct {
	Homepage      string `yaml:"homepage" json:"homepage"`
	Documentation string `yaml:"documentation,omitempty" json:"documentation,omitempty"`
	ModelCatalog  string `yaml:"model_catalog,omitempty" json:"model_catalog,omitempty"`
	API           string `yaml:"api,omitempty" json:"api,omitempty"`
}

type Organizations struct {
	HuggingFace []string `yaml:"huggingface,omitempty" json:"huggingface,omitempty"`
	ModelScope  []string `yaml:"modelscope,omitempty" json:"modelscope,omitempty"`
	GitHub      []string `yaml:"github,omitempty" json:"github,omitempty"`
}

type Discovery struct {
	HuggingFace bool `yaml:"huggingface,omitempty" json:"huggingface,omitempty"`
}

func (p Provider) Validate() error {
	if p.SchemaVersion != CurrentSchemaVersion || p.ID == "" || p.Name == "" {
		return fmt.Errorf("invalid provider identity")
	}
	if p.Official.Homepage == "" {
		return fmt.Errorf("provider %s has no official homepage", p.ID)
	}
	if p.Discovery.HuggingFace && len(p.Organizations.HuggingFace) == 0 {
		return fmt.Errorf("provider %s enables Hugging Face discovery without an organization", p.ID)
	}
	switch p.Identity.Strategy {
	case "", IdentityStrategyPublisher, IdentityStrategyAggregator:
	default:
		return fmt.Errorf("provider %s has unknown identity strategy %q", p.ID, p.Identity.Strategy)
	}
	if p.Identity.Strategy == IdentityStrategyPublisher && !p.HasAuthoritativeSource() {
		return fmt.Errorf("provider %s has publisher identity without an authoritative source", p.ID)
	}
	if p.Identity.RequireCorroboration && p.Identity.Strategy != IdentityStrategyPublisher {
		return fmt.Errorf("provider %s requires identity corroboration without publisher strategy", p.ID)
	}
	if api := p.Identity.OfficialAPI; api != nil {
		if strings.TrimSpace(api.URL) == "" {
			return fmt.Errorf("provider %s declares an official API without a URL", p.ID)
		}
		switch api.EffectiveAuth() {
		case OfficialAPIAuthNone, OfficialAPIAuthBearer, OfficialAPIAuthHeader:
		case OfficialAPIAuthQuery:
			if strings.TrimSpace(api.QueryParam) == "" {
				return fmt.Errorf("provider %s uses query authentication without a query parameter", p.ID)
			}
		default:
			return fmt.Errorf("provider %s has unknown official API auth %q", p.ID, api.Auth)
		}
	}
	return nil
}

func Load(path string) (Provider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Provider{}, err
	}
	var p Provider
	if err := yaml.Unmarshal(data, &p); err != nil {
		return Provider{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return Provider{}, fmt.Errorf("validate %s: %w", path, err)
	}
	return p, nil
}

func Scan(root string) ([]Provider, error) {
	var providers []Provider
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
			return nil
		}
		p, err := Load(path)
		if err != nil {
			return err
		}
		providers = append(providers, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].ID < providers[j].ID })
	return providers, nil
}
