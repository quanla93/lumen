// Package config loads an optional YAML config file for the agent and
// folds its values into the process environment so the existing
// envcfg-based reader sees them like any other env var.
//
// Design goal: YAML is a *syntax* convenience, not a parallel config
// system. Every key maps to a `LUMEN_AGENT_*` (or `LUMEN_HUB_*`) env
// var; once LoadFile runs, the rest of the agent reads its
// configuration the same way it always has.
//
// Precedence:
//
//	process env  >  yaml file  >  .env file  >  hardcoded defaults
//
// "Process env wins" is enforced by LoadFile only setting a variable
// when it isn't already in the environment. The same non-clobber rule
// applies to godotenv's Load(), so a homelab op can ship one
// /etc/lumen/agent.yaml across the fleet and override per-box with
// `Environment=` lines in the systemd unit without touching the file.
//
// Missing file is not an error — env-only setups keep working with
// zero config. A malformed file IS an error and exits early; silently
// running with half-loaded config is worse than a noisy startup.
package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// File mirrors the subset of LUMEN_* env vars that an agent operator
// would reasonably keep in a config file. Fields are intentionally
// flat — nested groups (`buffer.path`) save a few characters per key
// but cost readability when grep-ing for "where is buffer_path set".
type File struct {
	HubURL       string `yaml:"hub_url"`
	Token        string `yaml:"token"`
	Host         string `yaml:"host"`
	Interval     string `yaml:"interval"`
	DiskPath     string `yaml:"disk_path"`
	DockerSocket string `yaml:"docker_socket"`
	BufferPath   string `yaml:"buffer_path"`
	BufferMaxAge string `yaml:"buffer_max_age"`
	BufferDrain  string `yaml:"buffer_drain"`
}

// envMap is the source of truth for which YAML key maps to which env
// var. Keeping it in one place means a new field in File only needs
// one line here and one entry in the docs.
func (f *File) envMap() []struct {
	val, env string
} {
	return []struct{ val, env string }{
		{f.HubURL, "LUMEN_HUB_URL"},
		{f.Token, "LUMEN_AGENT_TOKEN"},
		{f.Host, "LUMEN_AGENT_HOST"},
		{f.Interval, "LUMEN_AGENT_INTERVAL"},
		{f.DiskPath, "LUMEN_AGENT_DISK_PATH"},
		{f.DockerSocket, "LUMEN_AGENT_DOCKER_SOCKET"},
		{f.BufferPath, "LUMEN_AGENT_BUFFER_PATH"},
		{f.BufferMaxAge, "LUMEN_AGENT_BUFFER_MAX_AGE"},
		{f.BufferDrain, "LUMEN_AGENT_BUFFER_DRAIN"},
	}
}

// LoadResult tells the caller what happened so it can be logged.
// Empty Path means no file was loaded (missing or never specified).
type LoadResult struct {
	Path    string   // file actually loaded, "" if none
	Applied []string // env vars set from the file (others were already in env)
	Skipped []string // env vars present in YAML but already in env (yaml ignored)
}

// LoadFile reads path, parses YAML, and applies non-empty fields to
// the process environment when the corresponding env var is not
// already set. A missing file is not an error.
func LoadFile(path string) (LoadResult, error) {
	var res LoadResult
	if path == "" {
		return res, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("read %s: %w", path, err)
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return res, fmt.Errorf("parse %s: %w", path, err)
	}
	res.Path = path
	for _, p := range f.envMap() {
		if p.val == "" {
			continue
		}
		if _, exists := os.LookupEnv(p.env); exists {
			res.Skipped = append(res.Skipped, p.env)
			continue
		}
		if err := os.Setenv(p.env, p.val); err != nil {
			return res, fmt.Errorf("setenv %s: %w", p.env, err)
		}
		res.Applied = append(res.Applied, p.env)
	}
	return res, nil
}
