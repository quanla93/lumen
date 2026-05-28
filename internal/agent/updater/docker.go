package updater

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

type DockerOptions struct {
	Container string
	Image     string
	Apply     bool
}

type DockerPlan struct {
	Container     string
	Image         string
	Replacement   string
	RestartPolicy string
	User          string
	Env           []string
	Binds         []string
	Networks      []string
}

type inspectContainer struct {
	Config struct {
		Image  string            `json:"Image"`
		Env    []string          `json:"Env"`
		User   string            `json:"User"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	HostConfig struct {
		Binds         []string `json:"Binds"`
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Networks map[string]struct{} `json:"Networks"`
	} `json:"NetworkSettings"`
}

func DockerUpdate(opts DockerOptions) (DockerPlan, error) {
	if opts.Container == "" {
		return DockerPlan{}, errors.New("container name is required")
	}
	current, err := inspect(opts.Container)
	if err != nil {
		return DockerPlan{}, err
	}
	image := opts.Image
	if image == "" {
		image = current.Config.Image
	}
	plan := DockerPlan{
		Container:     opts.Container,
		Image:         image,
		Replacement:   opts.Container + "-next",
		RestartPolicy: current.HostConfig.RestartPolicy.Name,
		User:          current.Config.User,
		Env:           lumenEnv(current.Config.Env),
		Binds:         append([]string(nil), current.HostConfig.Binds...),
		Networks:      networkNames(current.NetworkSettings.Networks),
	}
	if !opts.Apply {
		return plan, nil
	}
	if err := docker("pull", image); err != nil {
		return plan, fmt.Errorf("pull %s: %w", image, err)
	}
	_ = docker("rm", "-f", plan.Replacement)
	runArgs := []string{"run", "-d", "--name", plan.Replacement}
	if plan.RestartPolicy != "" && plan.RestartPolicy != "no" {
		runArgs = append(runArgs, "--restart", plan.RestartPolicy)
	}
	if plan.User != "" {
		runArgs = append(runArgs, "--user", plan.User)
	}
	for _, env := range plan.Env {
		runArgs = append(runArgs, "-e", env)
	}
	for _, bind := range plan.Binds {
		runArgs = append(runArgs, "-v", bind)
	}
	if len(plan.Networks) == 1 && plan.Networks[0] != "bridge" {
		runArgs = append(runArgs, "--network", plan.Networks[0])
	}
	runArgs = append(runArgs, image)
	if err := docker(runArgs...); err != nil {
		_ = docker("rm", "-f", plan.Replacement)
		return plan, fmt.Errorf("start replacement: %w", err)
	}
	if err := docker("rm", "-f", opts.Container); err != nil {
		_ = docker("rm", "-f", plan.Replacement)
		return plan, fmt.Errorf("remove old container: %w", err)
	}
	if err := docker("rename", plan.Replacement, opts.Container); err != nil {
		return plan, fmt.Errorf("rename replacement: %w", err)
	}
	return plan, nil
}

func inspect(container string) (inspectContainer, error) {
	out, err := dockerOutput("inspect", container)
	if err != nil {
		return inspectContainer{}, fmt.Errorf("inspect %s: %w", container, err)
	}
	var items []inspectContainer
	if err := json.Unmarshal(out, &items); err != nil {
		return inspectContainer{}, fmt.Errorf("parse docker inspect: %w", err)
	}
	if len(items) == 0 {
		return inspectContainer{}, fmt.Errorf("container %s not found", container)
	}
	return items[0], nil
}

func lumenEnv(env []string) []string {
	var out []string
	for _, item := range env {
		if strings.HasPrefix(item, "LUMEN_") {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

func networkNames(networks map[string]struct{}) []string {
	out := make([]string, 0, len(networks))
	for name := range networks {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func docker(args ...string) error {
	_, err := dockerOutput(args...)
	return err
}

func dockerOutput(args ...string) ([]byte, error) {
	cmd := exec.Command("docker", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return out, fmt.Errorf("%v: %s", err, msg)
		}
		return out, err
	}
	return out, nil
}
