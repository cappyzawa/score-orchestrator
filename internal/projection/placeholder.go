package projection

import (
	"encoding/json"
	"regexp"
)

var placeholderRe = regexp.MustCompile(`\$\{[^}]+\}`)

// HasUnresolvedPlaceholders parses JSON and returns:
// - true,nil  : at least one "${...}" found
// - false,nil : no placeholders
// - true,err  : JSON parse error (fail-safe => treat as unresolved)
func HasUnresolvedPlaceholders(raw []byte) (bool, error) {
	var v any
	if len(raw) == 0 {
		return false, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		// fail-safe: parsing error is treated as unresolved (block plan emission)
		return true, err
	}
	return scan(v), nil
}

func scan(v any) bool {
	switch x := v.(type) {
	case string:
		return placeholderRe.MatchString(x)
	case []any:
		for _, it := range x {
			if scan(it) {
				return true
			}
		}
	case map[string]any:
		for _, it := range x {
			if scan(it) {
				return true
			}
		}
	}
	return false
}
