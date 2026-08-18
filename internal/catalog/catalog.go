package catalog

import (
	_ "embed"
	"encoding/json"
)

type Problem struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Topic      string `json:"topic"`
	Difficulty string `json:"difficulty"`
	Priority   string `json:"priority"`
}

//go:embed problems.json
var problemsJSON []byte

func Load() []Problem {
	var problems []Problem
	if err := json.Unmarshal(problemsJSON, &problems); err != nil {
		panic(err)
	}
	return problems
}
