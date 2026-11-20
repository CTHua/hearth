package config

import (
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/version"
)

// oldestTeamSchema is the oldest team schema ParseTeam reads. Schema 1 is a
// subset of schema 2, so a file committed before an upgrade still parses.
const oldestTeamSchema = 1

// ParseTeam reads team-file content that is not on disk, such as the
// version in git's HEAD, with the checks Load applies to the team file for
// this build, except that it also accepts schema 1. The personal file is not
// involved.
func ParseTeam(data []byte) (Team, error) {
	doc, err := decode(brand.TeamFile, data)
	if err != nil {
		return Team{}, err
	}
	// Strict parsing assumes valid version keys, so they go first as in Load.
	if problems, _ := checkTeamVersions(doc, version.Current(), oldestTeamSchema); len(problems) > 0 {
		return Team{}, problems
	}
	team, problems := parseTeam(doc)
	if len(problems) > 0 {
		return Team{}, problems
	}
	return team, nil
}
