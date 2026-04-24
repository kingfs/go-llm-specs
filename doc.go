// Package llmspecs provides a static LLM model metadata registry for Go
// applications.
//
// The registry is compiled into the package, so runtime lookup does not require
// network I/O. Applications can resolve model IDs and aliases, inspect provider
// metadata, render model cards, filter by capabilities such as image input or
// tool use, and search across model names, tags, summaries, and aliases.
package llmspecs
