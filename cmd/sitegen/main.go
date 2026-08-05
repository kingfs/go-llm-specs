package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kingfs/go-llm-specs/internal/provider"
	"github.com/kingfs/go-llm-specs/internal/registry"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

//go:embed web/templates/*.html web/static/*
var webFiles embed.FS

const catalogSchemaVersion = 1

type siteCatalog struct {
	SchemaVersion int            `json:"schema_version"`
	Stats         siteStats      `json:"stats"`
	Providers     []siteProvider `json:"providers"`
	Models        []siteModel    `json:"models"`
}

type siteStats struct {
	Models     int `json:"models"`
	Providers  int `json:"providers"`
	Multimodal int `json:"multimodal"`
	ToolUse    int `json:"tool_use"`
	Reasoning  int `json:"reasoning"`
}

type siteProvider struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Homepage      string `json:"homepage,omitempty"`
	Documentation string `json:"documentation,omitempty"`
	ModelCatalog  string `json:"model_catalog,omitempty"`
}

type siteModel struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	NameCN        string            `json:"name_cn,omitempty"`
	Provider      string            `json:"provider"`
	ProviderID    string            `json:"provider_id"`
	Developer     string            `json:"developer,omitempty"`
	Lifecycle     string            `json:"lifecycle,omitempty"`
	Description   string            `json:"description,omitempty"`
	DescriptionCN string            `json:"description_cn,omitempty"`
	ContextLength int               `json:"context_length"`
	MaxOutput     int               `json:"max_output,omitempty"`
	Tags          []string          `json:"tags"`
	Aliases       []string          `json:"aliases,omitempty"`
	Links         map[string]string `json:"links,omitempty"`
}

type pageData struct {
	Title       string
	Description string
	Content     template.HTML
	Language    string
	Root        string
}

func main() {
	providersDir := flag.String("providers-dir", "providers", "publisher catalog directory")
	modelsDir := flag.String("models-dir", "models", "model registry directory")
	docsDir := flag.String("docs-dir", "docs", "Markdown documentation directory")
	outputDir := flag.String("output-dir", "site", "generated static site directory")
	flag.Parse()
	if err := generate(*providersDir, *modelsDir, *docsDir, *outputDir); err != nil {
		log.Fatal(err)
	}
}

func generate(providersDir, modelsDir, docsDir, outputDir string) error {
	providers, err := provider.Scan(providersDir)
	if err != nil {
		return err
	}
	models, err := registry.Scan(modelsDir)
	if err != nil {
		return err
	}
	catalog := buildCatalog(providers, models)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(catalog)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "catalog.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := renderTemplate("web/templates/index.html", filepath.Join(outputDir, "index.html"), pageData{}); err != nil {
		return err
	}
	if err := copyStatic(outputDir); err != nil {
		return err
	}
	if err := generateDocs(docsDir, outputDir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "404.html"), []byte(`<!doctype html><meta charset="utf-8"><title>Not found</title><p>Page not found. <a href="./">Return to the model catalog</a>.</p>`), 0o644); err != nil {
		return err
	}
	log.Printf("generated site with %d publishers and %d models", catalog.Stats.Providers, catalog.Stats.Models)
	return nil
}

func buildCatalog(providers []provider.Provider, models []registry.Model) siteCatalog {
	providerByKey := make(map[string]siteProvider)
	for _, p := range providers {
		sp := siteProvider{ID: p.ID, Name: p.Name, Homepage: p.Official.Homepage, Documentation: p.Official.Documentation, ModelCatalog: p.Official.ModelCatalog}
		providerByKey[normalizeKey(p.ID)] = sp
		providerByKey[normalizeKey(p.Name)] = sp
		for _, alias := range p.Aliases {
			providerByKey[normalizeKey(alias)] = sp
		}
	}

	result := siteCatalog{SchemaVersion: catalogSchemaVersion}
	seenProviders := make(map[string]siteProvider)
	for _, m := range models {
		sp, ok := providerByKey[normalizeKey(m.Provider)]
		if !ok {
			sp = siteProvider{ID: slugify(m.Provider), Name: m.Provider}
		}
		seenProviders[sp.ID] = sp
		tags := modelTags(m)
		result.Models = append(result.Models, siteModel{
			ID: m.ID, Name: displayName(m.Name, sp.Name), NameCN: m.NameCN,
			Provider: sp.Name, ProviderID: sp.ID, Developer: m.Developer, Lifecycle: m.Lifecycle,
			Description: m.Description, DescriptionCN: m.DescriptionCN, ContextLength: m.ContextLen,
			MaxOutput: m.MaxOutput, Tags: tags, Aliases: m.Aliases, Links: modelLinks(m.Links),
		})
		result.Stats.Models++
		if contains(tags, "multimodal") {
			result.Stats.Multimodal++
		}
		if contains(tags, "tool-use") {
			result.Stats.ToolUse++
		}
		if contains(tags, "reasoning") {
			result.Stats.Reasoning++
		}
	}
	for _, p := range seenProviders {
		result.Providers = append(result.Providers, p)
	}
	sort.Slice(result.Providers, func(i, j int) bool {
		return strings.ToLower(result.Providers[i].Name) < strings.ToLower(result.Providers[j].Name)
	})
	sort.Slice(result.Models, func(i, j int) bool {
		if result.Models[i].ProviderID == result.Models[j].ProviderID {
			return strings.ToLower(result.Models[i].Name) < strings.ToLower(result.Models[j].Name)
		}
		return strings.ToLower(result.Models[i].Provider) < strings.ToLower(result.Models[j].Provider)
	})
	result.Stats.Providers = len(result.Providers)
	return result
}

func modelTags(m registry.Model) []string {
	mapping := map[string]string{
		"CapChat": "chat", "CapEmbedding": "embedding", "CapRerank": "rerank", "CapTTS": "tts", "CapASR": "asr",
		"CapFunctionCall": "tool-use", "CapJsonMode": "structured-output", "CapMultimodal": "multimodal",
		"ModalityImageIn": "vision", "ModalityImageOut": "image-output", "ModalityAudioIn": "audio-input",
		"ModalityAudioOut": "audio-output", "ModalityVideoIn": "video-input", "ModalityVideoOut": "video-output",
		"ModalityFileIn": "file-input", "ModalityFileOut": "file-output",
	}
	seen := make(map[string]bool)
	for _, feature := range m.Features {
		if tag := mapping[feature]; tag != "" {
			seen[tag] = true
		}
	}
	if seen["vision"] || seen["image-output"] || seen["audio-input"] || seen["audio-output"] || seen["video-input"] || seen["video-output"] || seen["file-input"] {
		seen["multimodal"] = true
	}
	if m.Reasoning != nil && m.Reasoning.Supported {
		seen["reasoning"] = true
	}
	for _, word := range []string{"preview", "experimental", "free", "fast", "mini", "nano", "turbo", "pro", "thinking"} {
		if strings.Contains(strings.ToLower(m.ID+" "+m.Name+" "+m.Lifecycle), word) {
			seen[word] = true
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

func modelLinks(l registry.ModelLinks) map[string]string {
	links := map[string]string{"official": l.Official, "documentation": l.Documentation, "model_card": l.ModelCard, "paper": l.Paper, "repository": l.Repository}
	for key, value := range links {
		if value == "" {
			delete(links, key)
		}
	}
	if len(links) == 0 {
		return nil
	}
	return links
}

func displayName(name, provider string) string {
	prefix := strings.ToLower(provider) + ":"
	if strings.HasPrefix(strings.ToLower(name), prefix) {
		return strings.TrimSpace(name[len(provider)+1:])
	}
	return name
}

func normalizeKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func renderTemplate(name, output string, data pageData) error {
	t, err := template.ParseFS(webFiles, name)
	if err != nil {
		return err
	}
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()
	return t.Execute(file, data)
}

func copyStatic(outputDir string) error {
	return fs.WalkDir(webFiles, "web/static", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := webFiles.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(outputDir, "assets", filepath.Base(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func generateDocs(docsDir, outputDir string) error {
	repoRoot := filepath.Dir(filepath.Clean(docsDir))
	sources := []struct{ path, slug, title string }{{filepath.Join(repoRoot, "README.md"), "about", "项目介绍"}, {filepath.Join(repoRoot, "README_EN.md"), "about-en", "About"}}
	entries, err := os.ReadDir(docsDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			slug := strings.ToLower(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
			sources = append(sources, struct{ path, slug, title string }{filepath.Join(docsDir, entry.Name()), slug, strings.ReplaceAll(slug, "_", " ")})
		}
	}
	md := goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithParserOptions(parser.WithAutoHeadingID()))
	for _, source := range sources {
		raw, err := os.ReadFile(source.path)
		if err != nil {
			return err
		}
		var out strings.Builder
		if err := md.Convert(raw, &out); err != nil {
			return err
		}
		html := rewriteDocLinks(out.String())
		dir := filepath.Join(outputDir, "docs", source.slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		data := pageData{Title: source.title, Description: "go-llm-specs documentation", Content: template.HTML(html), Language: "zh-CN", Root: "../../"}
		if source.slug == "about-en" {
			data.Language = "en"
		}
		if err := renderTemplate("web/templates/doc.html", filepath.Join(dir, "index.html"), data); err != nil {
			return fmt.Errorf("render %s: %w", source.path, err)
		}
	}
	return nil
}

func rewriteDocLinks(html string) string {
	replacements := map[string]string{
		`href="./README.md"`:                       `href="../about/"`,
		`href="./README_EN.md"`:                    `href="../about-en/"`,
		`href="./docs/DEVELOPMENT.md"`:             `href="../development/"`,
		`href="./docs/DESIGN.md"`:                  `href="../design/"`,
		`href="./docs/MODEL_CATALOG.md"`:           `href="../model_catalog/"`,
		`href="./docs/CODEX_METADATA_PIPELINE.md"`: `href="../codex_metadata_pipeline/"`,
	}
	for from, to := range replacements {
		html = strings.ReplaceAll(html, from, to)
	}
	return strings.ReplaceAll(html, `href="./`, `href="https://github.com/kingfs/go-llm-specs/blob/master/`)
}
