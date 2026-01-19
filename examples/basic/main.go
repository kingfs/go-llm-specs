package main

import (
	"fmt"

	llmspecs "github.com/kingfs/go-llm-specs"
)

func main() {
	fmt.Println("LLM metadata registry examples:")

	// 1. Get by alias
	m, ok := llmspecs.Get("opus-4.5")
	if ok {
		fmt.Printf("[Alias Match] Found model: %s\n", m.Name())
		fmt.Printf("Description: %s\n", m.DescriptionCN())
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
	searchResults := llmspecs.Search("gpt-4", 3)
	for i, res := range searchResults {
		fmt.Printf("%d. %s [%s]\n", i+1, res.Name(), res.ID())
	}
}
