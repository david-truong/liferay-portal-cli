package review

import (
	"encoding/json"
	"regexp"
	"strings"
)

var codeFencePattern = regexp.MustCompile("(?m)^```(?:json)?|```$")

func isFindingsArray(raw string) ([]map[string]interface{}, bool) {
	var arr []interface{}
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil, false
	}

	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, false
		}
		result = append(result, m)
	}
	return result, true
}

// extractJSONArray pulls a findings array out of a model's response. Prose
// before the array can itself contain bracket citations like "[401]", which
// parse as a valid (but wrong) JSON array — scan backward from each "]" and
// only accept a candidate that parses as a list of objects.
func extractJSONArray(text string) []map[string]interface{} {
	cleaned := strings.TrimSpace(codeFencePattern.ReplaceAllString(strings.TrimSpace(text), ""))

	if arr, ok := isFindingsArray(cleaned); ok {
		return arr
	}

	end := strings.LastIndex(cleaned, "]")
	for end != -1 {
		depth := 0
		start := -1
		for i := end; i >= 0; i-- {
			switch cleaned[i] {
			case ']':
				depth++
			case '[':
				depth--
				if depth == 0 {
					start = i
				}
			}
			if start != -1 {
				break
			}
		}

		if start != -1 {
			if arr, ok := isFindingsArray(cleaned[start : end+1]); ok {
				return arr
			}
		}

		if end == 0 {
			break
		}
		end = strings.LastIndex(cleaned[:end], "]")
	}

	return nil
}
