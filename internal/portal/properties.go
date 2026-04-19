package portal

import (
	"bufio"
	"os"
	"strings"
)

// ReadProperties parses a Java .properties file into a map.
// Handles line continuations (trailing \) and ignores comments.
func ReadProperties(filename string) (map[string]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	props := make(map[string]string)
	scanner := bufio.NewScanner(f)

	var pending strings.Builder
	for scanner.Scan() {
		raw := scanner.Text()

		if strings.HasSuffix(raw, "\\") {
			pending.WriteString(strings.TrimSuffix(raw, "\\"))
			continue
		}
		pending.WriteString(raw)
		line := strings.TrimSpace(pending.String())
		pending.Reset()

		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}

		idx := strings.IndexAny(line, "=:")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		props[key] = val
	}
	return props, scanner.Err()
}
