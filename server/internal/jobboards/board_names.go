package jobboards

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

// FetchBoardCompanyName returns the company name a board publishes, or ""
// when its provider publishes none: only Greenhouse names its boards.
func (verifier *Verifier) FetchBoardCompanyName(ctx context.Context, provider, boardToken string) (string, error) {
	if provider != Greenhouse {
		return "", nil
	}
	body, found, err := verifier.fetch(ctx, provider, verifier.GreenhouseAPIBase+"/v1/boards/"+url.PathEscape(boardToken))
	if err != nil || !found {
		return "", err
	}
	var board struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &board); err != nil {
		return "", err
	}
	return strings.TrimSpace(board.Name), nil
}
