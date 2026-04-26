package conformance

import (
	"encoding/xml"
	"fmt"
	"strings"
)

func RenderText(report SuiteReport) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("%s v%s (spec %s)\n", report.Suite, report.Version, report.SpecVersion))
	builder.WriteString(fmt.Sprintf("%d/%d passed\n", report.Passed(), report.Total()))
	for _, result := range report.Results {
		status := "PASS"
		if !result.Passed {
			status = "FAIL"
		}
		builder.WriteString(fmt.Sprintf("%s %s - %s\n", status, result.ID, result.Title))
		if !result.Passed && result.Message != "" {
			builder.WriteString("  " + result.Message + "\n")
		}
		if !result.Passed {
			builder.WriteString(fmt.Sprintf("  expected: %v\n", result.Expected))
			builder.WriteString(fmt.Sprintf("  actual: %v\n", result.Actual))
		}
	}
	return builder.String()
}

func RenderJSON(report SuiteReport) (string, error) {
	return dumpJSON(report.JSON())
}

func RenderJUnit(report SuiteReport) (string, error) {
	type failure struct {
		Message string `xml:"message,attr"`
		Text    string `xml:",chardata"`
	}
	type testcase struct {
		Classname string   `xml:"classname,attr"`
		Name      string   `xml:"name,attr"`
		Failure   *failure `xml:"failure,omitempty"`
	}
	type testsuite struct {
		XMLName  xml.Name   `xml:"testsuite"`
		Name     string     `xml:"name,attr"`
		Tests    int        `xml:"tests,attr"`
		Failures int        `xml:"failures,attr"`
		Cases    []testcase `xml:"testcase"`
	}

	suite := testsuite{Name: report.Suite, Tests: report.Total(), Failures: report.Failed()}
	for _, result := range report.Results {
		tc := testcase{Classname: result.Category, Name: result.ID}
		if !result.Passed {
			payload, err := dumpJSON(result)
			if err != nil {
				return "", err
			}
			message := result.Message
			if message == "" {
				message = "conformance case failed"
			}
			tc.Failure = &failure{Message: message, Text: payload}
		}
		suite.Cases = append(suite.Cases, tc)
	}

	data, err := xml.Marshal(suite)
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}
