package main

import (
	"fmt"

	"github.com/kingfs/Argus"
)

func main() {
	fmt.Println("Total models:", argus.Total())

	fmt.Println("LLM metadata registry examples:")

	// 1. Get by alias
	modelName := "qwen3-embedding-0.6b"
	m, ok := argus.Get(modelName)
	if ok {
		fmt.Printf("[Alias Match] Found model: %s\n", m.Name())
		fmt.Printf("Description: %s\n", m.Description())
		fmt.Printf("Description CN: %s\n", m.DescriptionCN())
		fmt.Printf("Features: %s\n", m.Features().String())
	} else {
		fmt.Printf("Model %s not found!\n", modelName)
	}

	// 2. Query with multiple capabilities
	fmt.Println("\nQuerying Anthropic models with Vision and Function Calling:")
	results := argus.Query().
		Provider("Anthropic").
		Has(argus.ModalityImageIn).
		Has(argus.CapFunctionCall).
		List()

	for _, model := range results {
		fmt.Printf("- %s (context length: %d)\n",
			model.ID(), model.ContextLength())
	}

	// 3. Fuzzy search
	fmt.Println("\nFuzzy searching for 'gpt-4':")
	searchResults := argus.Search("gpt-4", 100)
	for i, res := range searchResults {
		fmt.Printf("%d. %s [%s]\n", i+1, res.Name(), res.ID())
	}

	// 4. Batch get
	fmt.Println("\nBatch retrieving models (gpt4t, qwen3-32b, qwen3-reranker-0.6b, qwen3-embedding-0.6b, non-existent):")
	batch := argus.GetMany([]string{"gpt4t", "qwen3-32b", "qwen3-reranker-0.6b", "qwen3-embedding-0.6b", "non-existent"})
	for _, m := range batch {
		fmt.Printf("- Found: %s (%s)\n", m.Name(), m.ID())
	}
}
