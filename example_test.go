package llmspecs_test

import (
	"fmt"

	llmspecs "github.com/kingfs/go-llm-specs"
)

func ExampleGet() {
	// Get a model by alias
	if m, ok := llmspecs.Get("gpt4t"); ok {
		fmt.Printf("Model ID: %s\n", m.ID())
		fmt.Printf("Provider: %s\n", m.Provider())
	}
	// Output:
	// Model ID: openai/gpt-4-turbo
	// Provider: Openai
}

func ExampleQueryBuilder_List() {
	// Query models with Image support from Anthropic
	models := llmspecs.Query().
		Provider("Anthropic").
		Has(llmspecs.ModalityImageIn).
		List()

	for _, m := range models {
		fmt.Println(m.ID())
	}
	// (Output depends on registry content)
}
