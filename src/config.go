package gitleaks

import (
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

type entropyRange struct {
	v1 float64
	v2 float64
}

type Regex struct {
	description string
	regex       *regexp.Regexp
}

// TomlConfig is used for loading gitleaks configs from a toml file
type TomlConfig struct {
	Regexes []struct {
		Description string
		Regex       string
	}
	Entropy struct {
		LineRegexes []string
	}
	Whitelist struct {
		Files   []string
		Regexes []string
		Commits []string
		Repos   []string
	}
	Misc struct {
		Entropy []string
	}
}

// Config contains gitleaks config
type Config struct {
	Regexes   []Regex
	WhiteList struct {
		regexes []*regexp.Regexp
		files   []*regexp.Regexp
		commits map[string]bool
		repos   []*regexp.Regexp
	}
	Entropy struct {
		entropyRanges []*entropyRange
		regexes       []*regexp.Regexp
	}
	sshAuth *ssh.PublicKeys
}

// loadToml loads of the toml config containing regexes and whitelists.
// This function will first look if the configPath is set and load the config
// from that file. Otherwise will then look for the path set by the GITHLEAKS_CONIFG
// env var. If that is not set, then gitleaks will continue with the default configs
// specified by the const var at the top `defaultConfig`
func newConfig() (*Config, error) {
	var (
		tomlConfig TomlConfig
		configPath string
		config     Config
	)

	if opts.ConfigPath != "" {
		configPath = opts.ConfigPath
		_, err := os.Stat(configPath)
		if err != nil {
			return nil, fmt.Errorf("no gitleaks config at %s", configPath)
		}
	} else {
		configPath = os.Getenv("GITLEAKS_CONFIG")
	}

	if configPath != "" {
		if _, err := toml.DecodeFile(configPath, &tomlConfig); err != nil {
			return nil, fmt.Errorf("problem loading config: %v", err)
		}
	} else {
		_, err := toml.Decode(defaultConfig, &tomlConfig)
		if err != nil {
			return nil, fmt.Errorf("problem loading default config: %v", err)
		}
	}

	sshAuth, err := getSSHAuth()
	if err != nil {
		return nil, err
	}
	config.sshAuth = sshAuth

	err = config.update(tomlConfig)
	if err != nil {
		return nil, err
	}
	return &config, err
}

// updateConfig will update a the global config values
func (config *Config) update(tomlConfig TomlConfig) error {
	if len(tomlConfig.Misc.Entropy) != 0 {
		err := config.updateEntropyRanges(tomlConfig.Misc.Entropy)
		if err != nil {
			return err
		}
	}

	for _, regex := range tomlConfig.Entropy.LineRegexes {
		config.Entropy.regexes = append(config.Entropy.regexes, regexp.MustCompile(regex))
	}

	if singleSearchRegex != nil {
		config.Regexes = append(config.Regexes, Regex{
			description: "single search",
			regex:       singleSearchRegex,
		})
	} else {
		for _, regex := range tomlConfig.Regexes {
			config.Regexes = append(config.Regexes, Regex{
				description: regex.Description,
				regex:       regexp.MustCompile(regex.Regex),
			})
		}
	}

	config.WhiteList.commits = make(map[string]bool)
	for _, commit := range tomlConfig.Whitelist.Commits {
		config.WhiteList.commits[commit] = true
	}
	for _, regex := range tomlConfig.Whitelist.Files {
		config.WhiteList.files = append(config.WhiteList.files, regexp.MustCompile(regex))
	}
	for _, regex := range tomlConfig.Whitelist.Regexes {
		config.WhiteList.regexes = append(config.WhiteList.regexes, regexp.MustCompile(regex))
	}
	for _, regex := range tomlConfig.Whitelist.Repos {
		config.WhiteList.repos = append(config.WhiteList.repos, regexp.MustCompile(regex))
	}

	return nil
}

// entropyRanges hydrates entropyRanges which allows for fine tuning entropy checking
func (config *Config) updateEntropyRanges(entropyLimitStr []string) error {
	for _, span := range entropyLimitStr {
		split := strings.Split(span, "-")
		v1, err := strconv.ParseFloat(split[0], 64)
		if err != nil {
			return err
		}
		v2, err := strconv.ParseFloat(split[1], 64)
		if err != nil {
			return err
		}
		if v1 > v2 {
			return fmt.Errorf("entropy range must be ascending")
		}
		r := &entropyRange{
			v1: v1,
			v2: v2,
		}
		if r.v1 > 8.0 || r.v1 < 0.0 || r.v2 > 8.0 || r.v2 < 0.0 {
			return fmt.Errorf("invalid entropy ranges, must be within 0.0-8.0")
		}
		config.Entropy.entropyRanges = append(config.Entropy.entropyRanges, r)
	}
	return nil
}

// externalConfig will attempt to load a pinned ".gitleaks.toml" configuration file
// from a remote or local repo. Use the --repo-config option to trigger this.
func (config *Config) updateFromRepo(repo *RepoInfo) error {
	var tomlConfig TomlConfig
	wt, err := repo.repository.Worktree()
	if err != nil {
		return err
	}
	f, err := wt.Filesystem.Open(".gitleaks.toml")
	if err != nil {
		return err
	}
	if _, err := toml.DecodeReader(f, &config); err != nil {
		return fmt.Errorf("problem loading config: %v", err)
	}
	f.Close()
	if err != nil {
		return err
	}
	return config.update(tomlConfig)
}

// getSSHAuth return an ssh auth use by go-git to clone repos behind authentication.
// If --ssh-key is set then it will attempt to load the key from that path. If not,
// gitleaks will use the default $HOME/.ssh/id_rsa key
func getSSHAuth() (*ssh.PublicKeys, error) {
	var (
		sshKeyPath string
	)
	if opts.SSHKey != "" {
		sshKeyPath = opts.SSHKey
	} else {
		// try grabbing default
		c, err := user.Current()
		if err != nil {
			return nil, nil
		}
		sshKeyPath = fmt.Sprintf("%s/.ssh/id_rsa", c.HomeDir)
	}
	sshAuth, err := ssh.NewPublicKeysFromFile("git", sshKeyPath, "")
	if err != nil {
		if strings.HasPrefix(opts.Repo, "git") {
			// if you are attempting to clone a git repo via ssh and supply a bad ssh key,
			// the clone will fail.
			return nil, fmt.Errorf("unable to generate ssh key: %v", err)
		}
	}
	return sshAuth, nil
}
