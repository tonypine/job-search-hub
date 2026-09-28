package linkedinexport

import (
	"errors"
	"io"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// ErrNotFollowsFile means the file lacks the columns of Company Follows.csv.
var ErrNotFollowsFile = errors.New(`not a LinkedIn Company Follows.csv: no "Organization" column`)

// ParseCompanyFollows reads Company Follows.csv.
func ParseCompanyFollows(file io.Reader) ([]store.NewCompanyFollow, error) {
	rows, columns, err := readTable(file, "Organization")
	if errors.Is(err, errMissingColumn) {
		return nil, ErrNotFollowsFile
	}
	if err != nil {
		return nil, err
	}
	var follows []store.NewCompanyFollow
	for _, row := range rows {
		field := columns.reader(row)
		follow := store.NewCompanyFollow{Organization: field("Organization")}
		if at, err := time.Parse(time.UnixDate, field("Followed On")); err == nil {
			follow.FollowedAt = &at
		}
		if follow.Organization != "" {
			follows = append(follows, follow)
		}
	}
	return follows, nil
}
