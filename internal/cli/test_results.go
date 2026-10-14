package cli

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const maxListedFailures = 20

// testSummary aggregates the JUnit XML results of one Gradle test run.
type testSummary struct {
	Tests    int
	Skipped  int
	Failures []string
}

type junitSuite struct {
	Cases []junitCase `xml:"testcase"`
}

type junitCase struct {
	ClassName string    `xml:"classname,attr"`
	Name      string    `xml:"name,attr"`
	Failure   *struct{} `xml:"failure"`
	Error     *struct{} `xml:"error"`
	Skipped   *struct{} `xml:"skipped"`
}

// summarizeTestResults reads every TEST-*.xml directly inside dirs modified at
// or after since. It returns found=false when no such file exists.
func summarizeTestResults(dirs []string, since time.Time) (summary testSummary, found bool, err error) {
	var paths []string
	for _, dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, "TEST-*.xml"))
		if err != nil {
			return summary, false, err
		}
		sort.Strings(matches)
		paths = append(paths, matches...)
	}

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return summary, false, err
		}
		if info.ModTime().Before(since) {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return summary, false, err
		}
		var suite junitSuite
		if err := xml.Unmarshal(data, &suite); err != nil {
			return summary, false, fmt.Errorf("parsing %s: %w", path, err)
		}

		found = true
		for _, c := range suite.Cases {
			summary.Tests++
			if c.Failure != nil || c.Error != nil {
				summary.Failures = append(summary.Failures, c.ClassName+"#"+c.Name)
			} else if c.Skipped != nil {
				summary.Skipped++
			}
		}
	}
	return summary, found, nil
}

// printTestSummary reports the results a Gradle test task wrote during the run
// that began at start. Liferay's Gradle plugin redirects results to
// <module>/test-results/<liferayDir>; a standard Gradle build (such as a
// Workspace) uses <module>/build/test-results/<task>.
func printTestSummary(label, modulePath, task, liferayDir string, start time.Time) {
	dirs := []string{
		filepath.Join(modulePath, "test-results", liferayDir),
		filepath.Join(modulePath, "build", "test-results", task),
	}
	summary, found, err := summarizeTestResults(dirs, start)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] unable to read test results: %v\n", label, err)
		return
	}
	if !found {
		fmt.Fprintf(os.Stderr, "[%s] no test results were produced (does --tests match anything, or was the task up to date?)\n", label)
		return
	}

	fmt.Fprintf(os.Stderr, "[%s] %d tests, %d failed, %d skipped\n", label, summary.Tests, len(summary.Failures), summary.Skipped)
	for i, failure := range summary.Failures {
		if i == maxListedFailures {
			fmt.Fprintf(os.Stderr, "  ... and %d more\n", len(summary.Failures)-maxListedFailures)
			break
		}
		fmt.Fprintf(os.Stderr, "  FAILED %s\n", failure)
	}
}
