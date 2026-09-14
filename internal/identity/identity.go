// Package identity resolves where a model's canonical identity comes from and
// whether a first-party source corroborates it.
//
// Phase 2 uses this only for reporting. Later phases use the same resolution to
// decide whether a newly discovered OpenRouter record may become active.
package identity

import (
	"path/filepath"
	"strings"

	"github.com/kingfs/go-llm-specs/internal/provider"
	"github.com/kingfs/go-llm-specs/internal/registry"
)

// Identity sources, ordered from most to least authoritative.
const (
	SourceManual              = "manual"
	SourceOfficial            = "official"
	SourceOfficialHuggingFace = "official_huggingface"
	SourceOfficialModelScope  = "official_modelscope"
	SourceOpenRouter          = "openrouter"
)

// Resolved is the effective origin of a model identity.
type Resolved struct {
	Source   string `json:"source"`
	Verified bool   `json:"verified"`
}

// Resolve returns the effective identity of a model for its publisher.
//
// Precedence:
//  1. an explicit model identity block in YAML (human reviewed, highest);
//  2. a repository in one of the publisher's declared official organizations;
//  3. an official identifier or link;
//  4. OpenRouter, which is authoritative for aggregator publishers and
//     unverified for publisher-strategy publishers until corroborated.
func Resolve(m registry.Model, p provider.Provider) Resolved {
	if m.Identity != nil && m.Identity.Source != "" {
		return Resolved{Source: m.Identity.Source, Verified: m.Identity.Verified}
	}
	if identifierInOrgs(m.Identifiers.HuggingFace, p.Organizations.HuggingFace) {
		return Resolved{Source: SourceOfficialHuggingFace, Verified: true}
	}
	if identifierInOrgs(m.Identifiers.ModelScope, p.Organizations.ModelScope) {
		return Resolved{Source: SourceOfficialModelScope, Verified: true}
	}
	if hasOfficialIdentity(m) {
		return Resolved{Source: SourceOfficial, Verified: true}
	}
	return Resolved{
		Source:   SourceOpenRouter,
		Verified: p.Identity.EffectiveStrategy() != provider.IdentityStrategyPublisher,
	}
}

// ProviderFor resolves the publisher record a model belongs to. Model IDs do
// not always match the provider directory, so developer, directory and ID
// prefix are tried in order.
func ProviderFor(m registry.Model, providers []provider.Provider) (provider.Provider, bool) {
	candidates := []string{
		normalize(m.Developer),
		normalize(filepath.Base(filepath.Dir(m.FilePath))),
		normalize(strings.SplitN(m.ID, "/", 2)[0]),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		for _, p := range providers {
			if normalize(p.ID) == candidate || normalize(p.Name) == candidate {
				return p, true
			}
		}
	}
	return provider.Provider{}, false
}

func identifierInOrgs(identifiers, organizations []string) bool {
	for _, identifier := range identifiers {
		org := strings.TrimSpace(strings.SplitN(identifier, "/", 2)[0])
		if org == "" {
			continue
		}
		for _, declared := range organizations {
			if strings.EqualFold(org, strings.TrimSpace(declared)) {
				return true
			}
		}
	}
	return false
}

func hasOfficialIdentity(m registry.Model) bool {
	return len(m.Identifiers.Official) > 0 || strings.TrimSpace(m.Links.Official) != ""
}

func normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").Replace(value)
}
