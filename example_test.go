package argus_test

import (
	"fmt"

	"github.com/kingfs/Argus"
)

func ExampleGet() {
	// Get a model by alias
	if m, ok := argus.Get("gpt4t"); ok {
		fmt.Printf("Model ID: %s\n", m.ID())
		fmt.Printf("Provider: %s\n", m.Provider())
	}
	// Output:
	// Model ID: openai/gpt-4-turbo
	// Provider: OpenAI
}

func ExampleQueryBuilder_List() {
	// Query models with Image support from Anthropic
	models := argus.Query().
		Provider("Anthropic").
		Has(argus.ModalityImageIn).
		List()

	for _, m := range models {
		fmt.Println(m.ID())
	}
	// (Output depends on registry content)
}
