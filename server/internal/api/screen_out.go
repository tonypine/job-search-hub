package api

import (
	"encoding/json"
	"strings"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// screenOutAnswer is one reason a posting could screen the owner out at
// once, answered from the fit checks and the facts' evidence. An empty
// verdict is information the fit doesn't judge, such as the contract.
type screenOutAnswer struct {
	Name     string         `json:"name"`
	Verdict  jobfit.Verdict `json:"verdict,omitempty"`
	Answer   string         `json:"answer"`
	Evidence string         `json:"evidence,omitempty"`
}

// buildScreenOutAnswers answers whether they hire from Brazil, the level and
// the timezone from the fit, and the contract from the facts, each with the
// posting's words the facts rest on.
func buildScreenOutAnswers(details store.JobDetails, fit jobfit.Fit) []screenOutAnswer {
	facts := map[string]store.JobFactEntry{}
	if details.Facts != nil {
		for _, entry := range details.Facts.Entries {
			facts[entry.Key] = entry
		}
	}
	answerFromCheck := func(checkName, answerName string, factKeys ...string) screenOutAnswer {
		answer := screenOutAnswer{Name: answerName, Verdict: jobfit.VerdictUnclear, Answer: "the posting doesn't say"}
		for _, check := range fit.Checks {
			if check.Name == checkName {
				answer.Verdict, answer.Answer = check.Verdict, check.Reason
			}
		}
		answer.Evidence = getFirstEvidence(facts, factKeys...)
		return answer
	}
	return []screenOutAnswer{
		answerFromCheck("Where they hire", "Hires from Brazil", "location", "location_restriction"),
		answerFromCheck("Level", "Level", "seniority"),
		answerFromCheck("Timezone", "Timezone", "timezone_requirement"),
		{Name: "Contract", Answer: describeContract(facts), Evidence: getFirstEvidence(facts, "contract", "contract_type")},
	}
}

func getFirstEvidence(facts map[string]store.JobFactEntry, keys ...string) string {
	for _, key := range keys {
		if evidence := facts[key].Evidence; evidence != "" {
			return evidence
		}
	}
	return ""
}

// describeContract is the contract as the posting writes it, with the kinds
// it names when the facts carry them: "PJ (contractor)".
func describeContract(facts map[string]store.JobFactEntry) string {
	var asWritten string
	for _, key := range []string{"contract", "contract_type"} {
		if json.Unmarshal(facts[key].Value, &asWritten) == nil && asWritten != "" {
			break
		}
	}
	var kinds []string
	_ = json.Unmarshal(facts["contract.kinds"].Value, &kinds)
	switch {
	case asWritten == "" || strings.EqualFold(asWritten, "not stated"):
		if len(kinds) > 0 {
			return strings.Join(kinds, ", ")
		}
		return "the posting doesn't say"
	case len(kinds) > 0 && !strings.EqualFold(asWritten, strings.Join(kinds, ", ")):
		return asWritten + " (" + strings.Join(kinds, ", ") + ")"
	default:
		return asWritten
	}
}
