package conformance

type Case struct {
	ID          string         `json:"id"`
	Category    string         `json:"category"`
	Title       string         `json:"title"`
	Schema      string         `json:"schema"`
	Path        string         `json:"path"`
	Expect      string         `json:"expect"`
	BridgeType  string         `json:"bridge_type"`
	Mandate     string         `json:"mandate"`
	Action      string         `json:"action"`
	Context     map[string]any `json:"context"`
	ReasonCodes []string       `json:"reason_codes"`
}

type manifestFile struct {
	Suite    suiteInfo        `json:"aump_conformance"`
	Defaults manifestDefaults `json:"defaults"`
	Cases    []Case           `json:"cases"`
}

type suiteInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	SpecVersion string `json:"spec_version"`
}

type manifestDefaults struct {
	Now string `json:"now"`
}

type CaseResult struct {
	ID          string         `json:"id"`
	Category    string         `json:"category"`
	Title       string         `json:"title"`
	Passed      bool           `json:"passed"`
	Expected    any            `json:"expected"`
	Actual      any            `json:"actual"`
	Message     string         `json:"message"`
	ReasonCodes []string       `json:"reason_codes"`
	Paths       []string       `json:"paths"`
	Details     map[string]any `json:"details"`
}

type SuiteReport struct {
	Suite       string       `json:"suite"`
	Version     string       `json:"version"`
	SpecVersion string       `json:"spec_version"`
	Results     []CaseResult `json:"results"`
}

func (r SuiteReport) Total() int {
	return len(r.Results)
}

func (r SuiteReport) Passed() int {
	count := 0
	for _, result := range r.Results {
		if result.Passed {
			count++
		}
	}
	return count
}

func (r SuiteReport) Failed() int {
	return r.Total() - r.Passed()
}

func (r SuiteReport) JSON() map[string]any {
	return map[string]any{
		"suite":        r.Suite,
		"version":      r.Version,
		"spec_version": r.SpecVersion,
		"summary": map[string]int{
			"total":  r.Total(),
			"passed": r.Passed(),
			"failed": r.Failed(),
		},
		"results": r.Results,
	}
}
