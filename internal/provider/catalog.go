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
}

const (
	IdentityStrategyPublisher  = "publisher"
	IdentityStrategyAggregator = "aggregator"
)

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
	return len(p.Organizations.HuggingFace) > 0 || len(p.Organizations.ModelScope) > 0 || p.Official.API != ""
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
