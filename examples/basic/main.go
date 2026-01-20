package main

import (
	"fmt"

	llmspecs "github.com/kingfs/go-llm-specs"
)

func main() {
	fmt.Println("Total models:", llmspecs.Total())

	fmt.Println("LLM metadata registry examples:")

	// 1. Get by alias
	m, ok := llmspecs.Get("qwen3-32b")
	if ok {
		fmt.Printf("[Alias Match] Found model: %s\n", m.Name())
		fmt.Printf("Description: %s\n", m.DescriptionCN())
		fmt.Printf("Features: %s\n", m.Features().String())
	}

	// 2. Query with multiple capabilities
	fmt.Println("\nQuerying Anthropic models with Vision and Function Calling:")
	results := llmspecs.Query().
		Provider("Anthropic").
		Has(llmspecs.ModalityImageIn).
		Has(llmspecs.CapFunctionCall).
		List()

	for _, model := range results {
		fmt.Printf("- %s (Prices: In $%f, Out $%f)\n",
			model.ID(), model.PriceInput(), model.PriceOutput())
	}

	// 3. Fuzzy search
	fmt.Println("\nFuzzy searching for 'gpt-4':")
	searchResults := llmspecs.Search("gpt-4", 100)
	for i, res := range searchResults {
		fmt.Printf("%d. %s [%s]\n", i+1, res.Name(), res.ID())
	}

	// 4. Batch get
	fmt.Println("\nBatch retrieving models (gpt4t, qwen3-32b, non-existent):")
	batch := llmspecs.GetMany([]string{"gpt4t", "qwen3-32b", "non-existent"})
	for _, m := range batch {
		fmt.Printf("- Found: %s (%s)\n", m.Name(), m.ID())
	}
}
