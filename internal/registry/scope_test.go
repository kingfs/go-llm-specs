package registry

import "testing"

func hfModel(pipeline, modelType string, tags ...string) Model {
	return Model{
		ID:       "publisher/model",
		Name:     "Model",
		Provider: "Publisher",
		Upstream: UpstreamMetadata{HuggingFace: &HuggingFaceMetadata{
			ID: "Publisher/Model", PipelineTag: pipeline, ModelType: modelType, Tags: tags,
		}},
	}
}

func TestScopeReasonExcludesDomainModels(t *testing.T) {
	cases := []struct {
		name  string
		model Model
		want  string
	}{
		{
			name: "protein language model behind a text-generation tag",
			model: Model{
				ID: "microsoft/dayhoff-170m-grs-ss-14000", Name: "Dayhoff-170M-GRS-SS-14000",
				Provider: "Microsoft", Developer: "microsoft",
				Upstream: UpstreamMetadata{HuggingFace: &HuggingFaceMetadata{
					ID: "microsoft/Dayhoff-170M-GRS-SS-14000", PipelineTag: "text-generation",
					ModelType: "jamba", Tags: []string{"protein-generation", "endpoints_compatible"},
				}},
			},
			want: "out-of-scope-domain:protein",
		},
		{
			name: "medical vision-language model",
			model: Model{
				ID: "google/medgemma-1.5-4b-it", Name: "MedGemma 1.5 4B",
				Upstream: UpstreamMetadata{HuggingFace: &HuggingFaceMetadata{
					ID: "google/medgemma-1.5-4b-it", PipelineTag: "image-text-to-text", ModelType: "gemma3",
					Tags: []string{"medical", "clinical-reasoning", "radiology"},
				}},
			},
			want: "out-of-scope-domain:biomedicine",
		},
		{
			name: "domain wording only in the description",
			model: Model{
				ID: "microsoft/designer-1", Name: "Designer 1",
				Description: "A model for protein sequence design and mutation effect prediction.",
				Upstream: UpstreamMetadata{HuggingFace: &HuggingFaceMetadata{
					ID: "microsoft/designer-1", PipelineTag: "text-generation",
				}},
			},
			want: "out-of-scope-domain:prose",
		},
		{
			name:  "vision classification pipeline",
			model: hfModel("image-classification", "resnet"),
			want:  "out-of-scope-pipeline:image-classification",
		},
		{
			name:  "speech encoder backbone",
			model: hfModel("feature-extraction", "wavlm", "speech"),
			want:  "out-of-scope-backbone:wavlm",
		},
		{
			name:  "3d generation pipeline",
			model: hfModel("text-to-3d", "trellis_text"),
			want:  "out-of-scope-pipeline:text-to-3d",
		},
		{
			name:  "robot policy pipeline",
			model: hfModel("robotics", "magma"),
			want:  "out-of-scope-pipeline:robotics",
		},
		{
			name: "speech backbone declared through architecture",
			model: Model{
				ID: "microsoft/unispeech-sat-base", Name: "unispeech-sat-base",
				Upstream: UpstreamMetadata{HuggingFace: &HuggingFaceMetadata{
					ID: "microsoft/unispeech-sat-base", PipelineTag: "feature-extraction", ModelType: "unispeech_sat",
				}},
			},
			want: "out-of-scope-backbone:unispeech_sat",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ScopeReason(tc.model); got != tc.want {
				t.Fatalf("ScopeReason(%s) = %q, want %q", tc.model.ID, got, tc.want)
			}
			if !IsOutOfScope(tc.model) {
				t.Fatalf("IsOutOfScope(%s) = false, want true", tc.model.ID)
			}
			if kind := ClassifyKind(tc.model); kind != KindOutOfScope {
				t.Fatalf("ClassifyKind(%s) = %q, want %q", tc.model.ID, kind, KindOutOfScope)
			}
			if IsCompiledKind(KindOutOfScope) {
				t.Fatal("out-of-scope must not be a compiled kind")
			}
		})
	}
}

func TestScopeReasonKeepsGeneralModelsInScope(t *testing.T) {
	cases := []struct {
		name  string
		model Model
	}{
		{
			name: "text chat model",
			model: Model{ID: "microsoft/phi-4", Name: "Phi-4", Upstream: UpstreamMetadata{HuggingFace: &HuggingFaceMetadata{
				ID: "microsoft/phi-4", PipelineTag: "text-generation", ModelType: "phi3",
			}}},
		},
		{
			name: "conversational vision-language model",
			model: Model{ID: "microsoft/phi-4-reasoning-vision-15b", Name: "Phi-4-Reasoning-Vision",
				Upstream: UpstreamMetadata{HuggingFace: &HuggingFaceMetadata{
					ID: "microsoft/Phi-4-reasoning-vision-15B", PipelineTag: "image-text-to-text", ModelType: "phi4-siglip",
				}}},
		},
		{
			name: "bidirectional text embedding model",
			model: Model{ID: "ibm-granite/granite-embedding-311m-multilingual-r2", Name: "Granite Embedding",
				Features: []string{"CapEmbedding"},
				Upstream: UpstreamMetadata{HuggingFace: &HuggingFaceMetadata{
					ID: "ibm-granite/granite-embedding-311m-multilingual-r2", PipelineTag: "feature-extraction",
					ModelType: "modernbert", Tags: []string{"embeddings", "multilingual", "mteb"},
				}}},
		},
		{
			name: "video understanding model",
			model: Model{ID: "qwen/qwen3-vl-32b", Name: "Qwen3 VL 32B",
				Upstream: UpstreamMetadata{HuggingFace: &HuggingFaceMetadata{
					ID: "Qwen/Qwen3-VL-32B", PipelineTag: "video-text-to-text", ModelType: "qwen3_vl",
				}}},
		},
		{
			name: "reranker",
			model: Model{ID: "qwen/qwen3-reranker-0.6b", Name: "Qwen3 Reranker",
				Upstream: UpstreamMetadata{HuggingFace: &HuggingFaceMetadata{
					ID: "Qwen/Qwen3-Reranker-0.6B", PipelineTag: "text-ranking", ModelType: "qwen3",
				}}},
		},
		{
			name: "speech recognition model",
			model: Model{ID: "microsoft/vibevoice-asr", Name: "VibeVoice ASR",
				Upstream: UpstreamMetadata{HuggingFace: &HuggingFaceMetadata{
					ID: "microsoft/VibeVoice-ASR", PipelineTag: "automatic-speech-recognition", ModelType: "vibevoice",
				}}},
		},
		{
			name: "machine translation model",
			model: Model{ID: "tencent/hy-mt2-7b", Name: "Hunyuan MT2",
				Upstream: UpstreamMetadata{HuggingFace: &HuggingFaceMetadata{
					ID: "tencent/HY-MT2-7B", PipelineTag: "translation",
				}}},
		},
		{
			name: "benchmark prose mentioning biology and chemistry",
			model: Model{ID: "openai/o1", Name: "o1",
				Description: "Optimized for math, science, programming, and other STEM-related tasks. " +
					"Exhibits PhD-level accuracy on benchmarks in physics, chemistry, and biology."},
		},
		{
			name:  "openrouter record without a repository",
			model: Model{ID: "deepseek/deepseek-v4-pro", Name: "DeepSeek V4 Pro", Developer: "deepseek"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if reason := ScopeReason(tc.model); reason != "" {
				t.Fatalf("ScopeReason(%s) = %q, want in scope", tc.model.ID, reason)
			}
			if kind := ClassifyKind(tc.model); kind != KindModel {
				t.Fatalf("ClassifyKind(%s) = %q, want %q", tc.model.ID, kind, KindModel)
			}
			if !IsCompiledKind(KindModel) {
				t.Fatal("an ordinary model must be a compiled kind")
			}
		})
	}
}

func TestExplicitKindOverridesScopePatterns(t *testing.T) {
	// A reviewed record can pull a false positive back into scope, and an
	// explicit serving kind still wins over the domain patterns.
	model := Model{
		ID: "microsoft/dayhoff-170m-grs-ss-14000", Name: "Dayhoff", Kind: KindModel,
		Upstream: UpstreamMetadata{HuggingFace: &HuggingFaceMetadata{
			ID: "microsoft/Dayhoff-170M-GRS-SS-14000", PipelineTag: "text-generation",
			Tags: []string{"protein-generation"},
		}},
	}
	if reason := ScopeReason(model); reason != "" {
		t.Fatalf("explicit kind must override scope patterns, got %q", reason)
	}
	if kind := ClassifyKind(model); kind != KindModel {
		t.Fatalf("ClassifyKind = %q, want %q", kind, KindModel)
	}
}

func TestInScopePipelineAllowlist(t *testing.T) {
	for _, tag := range []string{"text-generation", "text-ranking", "feature-extraction", "automatic-speech-recognition", "text-to-speech", ""} {
		if !IsInScopePipeline(tag) {
			t.Fatalf("IsInScopePipeline(%q) = false, want true", tag)
		}
	}
	for _, tag := range []string{"image-classification", "text-to-3d", "robotics", "graph-ml", "fill-mask"} {
		if IsInScopePipeline(tag) {
			t.Fatalf("IsInScopePipeline(%q) = true, want false", tag)
		}
	}
}
