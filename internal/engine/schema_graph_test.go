package engine

import "testing"

func TestPluralizeIrregular(t *testing.T) {
	tests := map[string]string{
		"person":   "people",
		"child":    "children",
		"man":      "men",
		"woman":    "women",
		"sheep":    "sheep",
		"fish":     "fish",
		"category": "categories",
		"bus":      "buses",
		"pet":      "pets",
	}
	for input, expected := range tests {
		got := pluralize(input)
		if got != expected {
			t.Errorf("pluralize(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestSingularizeIrregular(t *testing.T) {
	tests := map[string]string{
		"people":     "person",
		"children":   "child",
		"men":        "man",
		"women":      "woman",
		"sheep":      "sheep",
		"fish":       "fish",
		"categories": "category",
		"buses":      "bus",
		"pets":       "pet",
	}
	for input, expected := range tests {
		got := singularize(input)
		if got != expected {
			t.Errorf("singularize(%q) = %q, want %q", input, got, expected)
		}
	}
}
