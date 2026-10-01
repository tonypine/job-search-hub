package marketgaps

import (
	"testing"

	"github.com/google/uuid"
)

func TestRecurringTechnologiesTheKnowledgeBaseLacksAreGaps(t *testing.T) {
	jobs := make([]uuid.UUID, 5)
	for index := range jobs {
		jobs[index] = uuid.New()
	}
	goodFits := []GoodFit{
		{jobs[0], []string{"GraphQL", "React", "Node.JS", "AWS"}},
		{jobs[1], []string{"graphql", "React", "nodejs", "Kubernetes", "React Native"}},
		{jobs[2], []string{"GraphQL", "Node JS", "Kubernetes", "AWS", "React Native"}},
		{jobs[3], []string{"GraphQL", "GraphQL", "Kubernetes", "AWS", "react-native"}},
		{jobs[4], []string{"Python", "Python"}},
	}
	knowledgeBase := "Built the web app in React and Node.js. Shipped a React Native app (iOS, Android)."

	gaps := FindGaps(goodFits, knowledgeBase)
	var names []string
	for _, gap := range gaps {
		names = append(names, gap.Technology)
	}
	want := []string{"GraphQL", "AWS", "Kubernetes"}
	if len(names) != len(want) || names[0] != want[0] || names[1] != want[1] || names[2] != want[2] {
		t.Fatalf("gaps = %v, want %v: counted once per job, spelled as most postings do, the most asked first", names, want)
	}
	if len(gaps[0].JobIDs) != 4 {
		t.Errorf("GraphQL is asked for by %d jobs, want 4 (a repeat within a job counts once)", len(gaps[0].JobIDs))
	}
}
