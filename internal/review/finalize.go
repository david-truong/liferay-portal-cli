package review

import (
	"fmt"
	"strings"
)

var chanceByViolationCount = map[int]int{0: 0, 1: 40, 2: 55, 3: 70, 4: 85}

// Result is the {"chance", "violations"} verdict poll-prs.sh posts to PRs.
type Result struct {
	Chance     int      `json:"chance"`
	Violations []string `json:"violations"`
}

func finalize(findings []Finding) Result {
	violations := make([]string, 0, len(findings))
	for _, f := range findings {
		description := strings.TrimSpace(f.Description)
		rule := strings.TrimSpace(f.Rule)
		if rule != "" {
			violations = append(violations, fmt.Sprintf("%s (%s)", description, rule))
		} else {
			violations = append(violations, description)
		}
	}

	chance, ok := chanceByViolationCount[len(violations)]
	if !ok {
		chance = 90
	}
	return Result{Chance: chance, Violations: violations}
}
