package review

// Finding is one candidate code-review violation, mirroring the JSON shape
// the model is asked to emit: {"file", "anchor", "rule", "confidence", "description"}.
type Finding struct {
	File        string
	Anchor      string
	Rule        string
	Confidence  string
	Description string
}

func findingFromMap(m map[string]interface{}) Finding {
	return Finding{
		File:        stringField(m, "file"),
		Anchor:      stringField(m, "anchor"),
		Rule:        stringField(m, "rule"),
		Confidence:  stringField(m, "confidence"),
		Description: stringField(m, "description"),
	}
}

func stringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
