package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const testSuiteXML = `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.FooTest" tests="4" skipped="1" failures="1" errors="1">
  <testcase name="testOk" classname="com.example.FooTest" time="0.1"/>
  <testcase name="testFail" classname="com.example.FooTest" time="0.1">
    <failure message="boom" type="java.lang.AssertionError">trace</failure>
  </testcase>
  <testcase name="testError" classname="com.example.FooTest" time="0.1">
    <error message="npe" type="java.lang.NullPointerException">trace</error>
  </testcase>
  <testcase name="testSkip" classname="com.example.FooTest" time="0.0">
    <skipped/>
  </testcase>
</testsuite>
`

func writeResult(t *testing.T, dir, name, content string, modTime time.Time) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func TestSummarizeTestResults(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	writeResult(t, dir, "TEST-com.example.FooTest.xml", testSuiteXML, start.Add(time.Second))

	summary, found, err := summarizeTestResults([]string{dir}, start)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected results to be found")
	}
	if summary.Tests != 4 || summary.Skipped != 1 {
		t.Errorf("got %d tests, %d skipped; want 4, 1", summary.Tests, summary.Skipped)
	}
	want := []string{"com.example.FooTest#testFail", "com.example.FooTest#testError"}
	if !reflect.DeepEqual(summary.Failures, want) {
		t.Errorf("failures = %v, want %v", summary.Failures, want)
	}
}

func TestSummarizeTestResults_IgnoresStaleFiles(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	writeResult(t, dir, "TEST-com.example.FooTest.xml", testSuiteXML, start.Add(-time.Hour))

	_, found, err := summarizeTestResults([]string{dir}, start)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("stale result files must not count")
	}
}

func TestSummarizeTestResults_AggregatesDirs(t *testing.T) {
	root := t.TempDir()
	liferayDir := filepath.Join(root, "test-results", "unit", "test")
	gradleDir := filepath.Join(root, "build", "test-results", "test")
	binaryDir := filepath.Join(liferayDir, "binary")
	for _, d := range []string{liferayDir, gradleDir, binaryDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	writeResult(t, liferayDir, "TEST-a.xml", testSuiteXML, start.Add(time.Second))
	writeResult(t, gradleDir, "TEST-b.xml", testSuiteXML, start.Add(time.Second))
	writeResult(t, binaryDir, "TEST-c.xml", testSuiteXML, start.Add(time.Second))

	summary, found, err := summarizeTestResults([]string{liferayDir, gradleDir}, start)
	if err != nil {
		t.Fatal(err)
	}
	if !found || summary.Tests != 8 {
		t.Errorf("found=%v tests=%d; want true, 8 (binary dir must not be read)", found, summary.Tests)
	}
}

func TestSummarizeTestResults_MissingDir(t *testing.T) {
	_, found, err := summarizeTestResults([]string{filepath.Join(t.TempDir(), "missing")}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("missing directory should report no results")
	}
}

func TestSummarizeTestResults_MalformedXML(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	writeResult(t, dir, "TEST-bad.xml", "<testsuite><testcase", start.Add(time.Second))

	if _, _, err := summarizeTestResults([]string{dir}, start); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestTestGradleArgs(t *testing.T) {
	got := testGradleArgs("*FooTest")
	want := []string{"-Dtest.ignore.failures=false", "test", "--tests", "*FooTest"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("testGradleArgs = %v, want %v", got, want)
	}
}

func TestTestIntegrationGradleArgs(t *testing.T) {
	got := testIntegrationGradleArgs("*FooTest")
	want := []string{"-DtestIntegration.ignore.failures=false", "testIntegration", "--tests", "*FooTest"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("testIntegrationGradleArgs = %v, want %v", got, want)
	}
}
