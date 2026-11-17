package core

import (
	"context"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/detect"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/gitx"
)

// ErrNoRepos is what DetectProtocol returns when the team file has no repo
// URL to try; compare with errors.Is.
var ErrNoRepos = apperr.New(apperr.Usage, "no repos in %s to detect the protocol with", brand.TeamFile)

// SetProtocol saves protocol, "ssh" or "https", to the personal file.
func (s *Service) SetProtocol(protocol string) error {
	if !config.ValidProtocol(protocol) {
		return apperr.New(apperr.Usage, "protocol must be %q or %q, not %q", giturl.SSH, giturl.HTTPS, protocol)
	}
	return s.savePersonal(s.home.Personal.WithProtocol(protocol))
}

// DetectProtocol tries every team-file repo URL and saves the first protocol
// that answers, replacing the one already set.
func (s *Service) DetectProtocol(ctx context.Context) (string, error) {
	if len(s.home.Team.Repos) == 0 {
		return "", ErrNoRepos
	}
	git, err := gitx.New(ctx)
	if err != nil {
		return "", err
	}
	return s.detectAndSave(ctx, git, repoURLs(s.home.Team.Repos))
}

// detectAndSave writes nothing when no URL answers.
func (s *Service) detectAndSave(ctx context.Context, git *gitx.Git, urls []string) (string, error) {
	protocol, err := detect.Protocol(ctx, git, urls)
	if err != nil {
		return "", err
	}
	if err := s.savePersonal(s.home.Personal.WithProtocol(protocol)); err != nil {
		return "", err
	}
	return protocol, nil
}

// SetAgents replaces the chosen agents in the personal file; nil clears them.
func (s *Service) SetAgents(agents []string) error {
	return s.savePersonal(s.home.Personal.WithAgents(agents))
}

func (s *Service) savePersonal(personal config.Personal) error {
	if err := config.WritePersonal(s.home.Root, personal); err != nil {
		return err
	}
	s.home.Personal = personal
	return nil
}

func (s *Service) saveTeam(team config.Team) error {
	if err := config.WriteTeam(s.home.Root, team); err != nil {
		return err
	}
	s.home.Team = team
	return nil
}
