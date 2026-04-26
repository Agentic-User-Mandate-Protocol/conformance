package conformance

import "testing"

func TestRunSuite(t *testing.T) {
	report, err := RunSuite("../../fixtures", nil)
	if err != nil {
		t.Fatalf("RunSuite returned error: %v", err)
	}
	if report.Total() != 29 {
		t.Fatalf("Total() = %d, want 29", report.Total())
	}
	if report.Failed() != 0 {
		t.Fatalf("Failed() = %d, want 0", report.Failed())
	}
}
