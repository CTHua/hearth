package config

import (
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/version"
)

// ParseTeam reads team-file content that is not on disk, such as the
// version in git's HEAD, with the checks Load applies to the team file for
// this build. The personal file is not involved.
func ParseTeam(data []byte) (Team, error) {
	doc, err := decode(brand.TeamFile, data)
	if err != nil {
		return Team{}, err
	}
	// Strict parsing assumes valid version keys, so they go first as in Load.
	if problems, _ := checkTeamVersions(doc, version.Current()); len(problems) > 0 {
		return Team{}, problems
	}
	team, problems := parseTeam(doc)
	if len(problems) > 0 {
		return Team{}, problems
	}
	return team, nil
}
