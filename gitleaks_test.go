package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/franela/goblin"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/storage/memory"
)

const testWhitelistCommit = `
[[regexes]]
description = "AWS"
regex = '''AKIA[0-9A-Z]{16}'''

[whitelist]
commits = [
  "eaeffdc65b4c73ccb67e75d96bd8743be2c85973",
]
`
const testWhitelistFile = `
[[regexes]]
description = "AWS"
regex = '''AKIA[0-9A-Z]{16}'''

[whitelist]
files = [
  ".go",
]
`
const testWhitelistBranch = `
[[regexes]]
description = "AWS"
regex = '''AKIA[0-9A-Z]{16}'''

[whitelist]
branches = [
  "origin/master",
]
`

const testWhitelistRegex = `
[[regexes]]
description = "AWS"
regex = '''AKIA[0-9A-Z]{16}'''

[whitelist]
regexes= [
  "AKIA",
]
`

const testWhitelistRepo = `
[[regexes]]
description = "AWS"
regex = '''AKIA[0-9A-Z]{16}'''

[whitelist]
repos = [
  "gronit",
]
`

var benchmarkRepo *RepoDescriptor
var benchmarkLeaksRepo *RepoDescriptor

func getBenchmarkLeaksRepo() *RepoDescriptor {
	if benchmarkLeaksRepo != nil {
		return benchmarkLeaksRepo
	}
	leaksR, _ := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL: "https://github.com/gitleakstest/gronit.git",
	})
	benchmarkLeaksRepo = &RepoDescriptor{
		repository: leaksR,
	}
	return benchmarkLeaksRepo
}

func getBenchmarkRepo() *RepoDescriptor {
	if benchmarkRepo != nil {
		return benchmarkRepo
	}
	bmRepo, _ := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL: "https://github.com/apple/swift-package-manager.git",
	})
	benchmarkRepo = &RepoDescriptor{
		repository: bmRepo,
	}
	return benchmarkRepo
}

func TestGetRepo(t *testing.T) {
	var err error
	dir, err = ioutil.TempDir("", "gitleaksTestRepo")
	defer os.RemoveAll(dir)
	if err != nil {
		panic(err)
	}
	_, err = git.PlainClone(dir, false, &git.CloneOptions{
		URL: "https://github.com/gitleakstest/gronit",
	})

	if err != nil {
		panic(err)
	}

	var tests = []struct {
		testOpts       Options
		description    string
		expectedErrMsg string
	}{
		{
			testOpts: Options{
				Repo: "https://github.com/gitleakstest/gronit",
			},
			description:    "test plain clone remote repo",
			expectedErrMsg: "",
		},
		{
			testOpts: Options{
				Repo: "https://github.com/gitleakstest/gronit",
				Disk: true,
			},
			description:    "test on disk clone remote repo",
			expectedErrMsg: "",
		},
		{
			testOpts: Options{
				RepoPath: dir,
			},
			description:    "test local clone repo",
			expectedErrMsg: "",
		},
		{
			testOpts: Options{
				Repo: "https://github.com/gitleakstest/nope",
			},
			description:    "test no repo",
			expectedErrMsg: "authentication required",
		},
		{
			testOpts: Options{
				Repo:           "https://github.com/gitleakstest/private",
				IncludePrivate: true,
			},
			description:    "test private repo",
			expectedErrMsg: "invalid auth method",
		},
		{
			testOpts: Options{
				Repo:           "https://github.com/gitleakstest/private",
				IncludePrivate: true,
				Disk:           true,
			},
			description:    "test private repo",
			expectedErrMsg: "invalid auth method",
		},
	}
	g := goblin.Goblin(t)
	for _, test := range tests {
		g.Describe("TestGetRepo", func() {
			g.It(test.description, func() {
				opts = test.testOpts
				_, err := cloneRepo()
				if err != nil {
					g.Assert(err.Error()).Equal(test.expectedErrMsg)
				}
			})
		})
	}
}
func TestRun(t *testing.T) {
	var err error
	configsDir := testTomlLoader()

	dir, err = ioutil.TempDir("", "gitleaksTestOwner")
	defer os.RemoveAll(dir)
	if err != nil {
		panic(err)
	}
	git.PlainClone(dir+"/gronit", false, &git.CloneOptions{
		URL: "https://github.com/gitleakstest/gronit",
	})
	git.PlainClone(dir+"/h1domains", false, &git.CloneOptions{
		URL: "https://github.com/gitleakstest/h1domains",
	})
	var tests = []struct {
		testOpts       Options
		description    string
		expectedErrMsg string
		whiteListRepos []string
		numLeaks       int
		configPath     string
	}{
		{
			testOpts: Options{
				GithubUser: "gitleakstest",
			},
			description:    "test github user",
			numLeaks:       2,
			expectedErrMsg: "",
		},
		{
			testOpts: Options{
				GithubUser: "gitleakstest",
				Disk:       true,
			},
			description:    "test github user on disk ",
			numLeaks:       2,
			expectedErrMsg: "",
		},
		{
			testOpts: Options{
				GithubOrg: "gitleakstestorg",
			},
			description:    "test github org",
			numLeaks:       2,
			expectedErrMsg: "",
		},
		{
			testOpts: Options{
				GithubOrg: "gitleakstestorg",
				Disk:      true,
			},
			description:    "test org on disk",
			numLeaks:       2,
			expectedErrMsg: "",
		},
		{
			testOpts: Options{
				OwnerPath: dir,
			},
			description:    "test owner path",
			numLeaks:       2,
			expectedErrMsg: "",
		},
		{
			testOpts: Options{
				GithubOrg:      "gitleakstestorg",
				IncludePrivate: true,
				SSHKey:         "reallyreallyreallyreallywrongpath",
			},
			description:    "test private org no ssh",
			numLeaks:       0,
			expectedErrMsg: "unable to generate ssh key: open reallyreallyreallyreallywrongpath: no such file or directory",
		},
		{
			testOpts: Options{
				Repo: "https://github.com/gitleakstest/gronit.git",
			},
			description:    "test leak",
			numLeaks:       2,
			expectedErrMsg: "",
		},
		{
			testOpts: Options{
				Repo: "https://github.com/gitleakstest/h1domains.git",
			},
			description:    "test clean",
			numLeaks:       0,
			expectedErrMsg: "",
		},
		{
			testOpts: Options{
				Repo: "https://github.com/gitleakstest/empty.git",
			},
			description:    "test empty",
			numLeaks:       0,
			expectedErrMsg: "reference not found",
		},
		{
			testOpts: Options{
				GithubOrg: "gitleakstestorg",
			},
			description:    "test github org, whitelist repo",
			numLeaks:       0,
			expectedErrMsg: "",
			configPath:     path.Join(configsDir, "repo"),
		},
		{
			testOpts: Options{
				GithubOrg:    "gitleakstestorg",
				ExcludeForks: true,
			},
			description:    "test github org, exclude forks",
			numLeaks:       0,
			expectedErrMsg: "",
		},
	}
	g := goblin.Goblin(t)
	for _, test := range tests {
		g.Describe("TestRun", func() {
			g.It(test.description, func() {
				if test.configPath != "" {
					os.Setenv("GITLEAKS_CONFIG", test.configPath)
				}
				opts = test.testOpts
				leaks, err := run()
				if err != nil {
					g.Assert(err.Error()).Equal(test.expectedErrMsg)
				}
				g.Assert(len(leaks)).Equal(test.numLeaks)
			})
		})
	}
}

func TestWriteReport(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "reportDir")
	reportJSON := path.Join(tmpDir, "report.json")
	reportCSV := path.Join(tmpDir, "report.csv")
	defer os.RemoveAll(tmpDir)
	leaks := []Leak{
		{
			Line:     "eat",
			Commit:   "your",
			Offender: "veggies",
			Type:     "and",
			Message:  "get",
			Author:   "some",
			File:     "sleep",
			Branch:   "thxu",
		},
	}

	var tests = []struct {
		leaks       []Leak
		reportFile  string
		fileName    string
		description string
		testOpts    Options
	}{
		{
			leaks:       leaks,
			reportFile:  reportJSON,
			fileName:    "report.json",
			description: "can we write a file",
			testOpts: Options{
				Report: reportJSON,
			},
		},
		{
			leaks:       leaks,
			reportFile:  reportCSV,
			fileName:    "report.csv",
			description: "can we write a file",
			testOpts: Options{
				Report: reportCSV,
				CSV:    true,
			},
		},
	}
	g := goblin.Goblin(t)
	for _, test := range tests {
		g.Describe("TestWriteReport", func() {
			g.It(test.description, func() {
				opts = test.testOpts
				writeReport(test.leaks)
				f, _ := os.Stat(test.reportFile)
				g.Assert(f.Name()).Equal(test.fileName)
			})
		})
	}

}

func testTomlLoader() string {
	tmpDir, _ := ioutil.TempDir("", "whiteListConfigs")
	ioutil.WriteFile(path.Join(tmpDir, "regex"), []byte(testWhitelistRegex), 0644)
	ioutil.WriteFile(path.Join(tmpDir, "branch"), []byte(testWhitelistBranch), 0644)
	ioutil.WriteFile(path.Join(tmpDir, "commit"), []byte(testWhitelistCommit), 0644)
	ioutil.WriteFile(path.Join(tmpDir, "file"), []byte(testWhitelistFile), 0644)
	ioutil.WriteFile(path.Join(tmpDir, "repo"), []byte(testWhitelistRepo), 0644)
	return tmpDir
}

func TestAuditRepo(t *testing.T) {
	var leaks []Leak
	err := loadToml()
	configsDir := testTomlLoader()
	defer os.RemoveAll(configsDir)

	if err != nil {
		panic(err)
	}
	leaksR, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL: "https://github.com/gitleakstest/gronit.git",
	})
	if err != nil {
		panic(err)
	}
	leaksRepo := &RepoDescriptor{
		repository: leaksR,
		name:       "gronit",
	}

	cleanR, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL: "https://github.com/gitleakstest/h1domains.git",
	})
	if err != nil {
		panic(err)
	}
	cleanRepo := &RepoDescriptor{
		repository: cleanR,
		name:       "h1domains",
	}

	var tests = []struct {
		testOpts          Options
		description       string
		expectedErrMsg    string
		numLeaks          int
		repo              *RepoDescriptor
		whiteListFiles    []*regexp.Regexp
		whiteListCommits  map[string]bool
		whiteListBranches []string
		whiteListRepos    []string
		whiteListRegexes  []*regexp.Regexp
		configPath        string
	}{
		{
			repo:        leaksRepo,
			description: "two leaks present",
			numLeaks:    2,
		},
		{
			repo:        leaksRepo,
			description: "two leaks present limit goroutines",
			numLeaks:    2,
			testOpts: Options{
				MaxGoRoutines: 4,
			},
		},
		{
			repo:        leaksRepo,
			description: "audit specific bad branch",
			numLeaks:    2,
			testOpts: Options{
				Branch: "master",
			},
		},
		{
			repo:        leaksRepo,
			description: "audit specific good branch",
			numLeaks:    0,
			testOpts: Options{
				Branch: "dev",
			},
		},
		{
			repo:        leaksRepo,
			description: "audit all branch",
			numLeaks:    6,
			testOpts: Options{
				AuditAllRefs: true,
			},
		},
		{
			repo:        leaksRepo,
			description: "audit all branch whitelist 1",
			numLeaks:    4,
			testOpts: Options{
				AuditAllRefs: true,
			},
			whiteListBranches: []string{
				"origin/master",
			},
		},
		{
			repo:        leaksRepo,
			description: "two leaks present whitelist AWS.. no leaks",
			whiteListRegexes: []*regexp.Regexp{
				regexp.MustCompile("AKIA"),
			},
			numLeaks: 0,
		},
		{
			repo:        leaksRepo,
			description: "two leaks present limit goroutines",
			numLeaks:    2,
		},
		{
			repo:        cleanRepo,
			description: "no leaks present",
			numLeaks:    0,
		},
		{
			repo:        leaksRepo,
			description: "two leaks present whitelist go files",
			whiteListFiles: []*regexp.Regexp{
				regexp.MustCompile(".go"),
			},
			numLeaks: 0,
		},
		{
			// note this double counts the first commit since we are whitelisting
			// a "bad" first commit
			repo:        leaksRepo,
			description: "two leaks present whitelist bad commit",
			whiteListCommits: map[string]bool{
				"eaeffdc65b4c73ccb67e75d96bd8743be2c85973": true,
			},
			numLeaks: 2,
		},
		{
			repo:        leaksRepo,
			description: "redact",
			testOpts: Options{
				Redact: true,
			},
			numLeaks: 2,
		},
		{
			repo:        leaksRepo,
			description: "toml whitelist regex",
			configPath:  path.Join(configsDir, "regex"),
			numLeaks:    0,
		},
		{
			repo:        leaksRepo,
			description: "toml whitelist branch",
			configPath:  path.Join(configsDir, "branch"),
			testOpts: Options{
				AuditAllRefs: true,
			},
			numLeaks: 4,
		},
		{
			repo:        leaksRepo,
			description: "toml whitelist file",
			configPath:  path.Join(configsDir, "file"),
			numLeaks:    0,
		},
		{
			// note this double counts the first commit since we are whitelisting
			// a "bad" first commit
			repo:        leaksRepo,
			description: "toml whitelist commit",
			configPath:  path.Join(configsDir, "commit"),
			numLeaks:    2,
		},
		{
			repo:        leaksRepo,
			description: "audit whitelist repo",
			numLeaks:    0,
			whiteListRepos: []string{
				"gronit",
			},
		},
		{
			repo:        leaksRepo,
			description: "toml whitelist repo",
			numLeaks:    0,
			configPath:  path.Join(configsDir, "repo"),
		},
	}

	whiteListCommits = make(map[string]bool)
	g := goblin.Goblin(t)
	for _, test := range tests {
		g.Describe("TestAuditRepo", func() {
			g.It(test.description, func() {
				opts = test.testOpts
				// settin da globs
				if test.whiteListFiles != nil {
					whiteListFiles = test.whiteListFiles
				} else {
					whiteListFiles = nil
				}
				if test.whiteListCommits != nil {
					whiteListCommits = test.whiteListCommits
				} else {
					whiteListCommits = nil
				}
				if test.whiteListBranches != nil {
					whiteListBranches = test.whiteListBranches
				} else {
					whiteListBranches = nil
				}
				if test.whiteListRegexes != nil {
					whiteListRegexes = test.whiteListRegexes
				} else {
					whiteListRegexes = nil
				}
				if test.whiteListRepos != nil {
					whiteListRepos = test.whiteListRepos
				} else {
					whiteListRepos = nil
				}

				// config paths
				if test.configPath != "" {
					os.Setenv("GITLEAKS_CONFIG", test.configPath)
					loadToml()
				}

				leaks, err = auditGitRepo(test.repo)

				if opts.Redact {
					g.Assert(leaks[0].Offender).Equal("REDACTED")
				}
				g.Assert(len(leaks)).Equal(test.numLeaks)
			})
		})
	}
}

func TestOptionGuard(t *testing.T) {
	var tests = []struct {
		testOpts            Options
		githubToken         bool
		description         string
		expectedErrMsg      string
		expectedErrMsgFuzzy string
	}{
		{
			testOpts:       Options{},
			description:    "default no opts",
			expectedErrMsg: "",
		},
		{
			testOpts: Options{
				IncludePrivate: true,
				GithubOrg:      "fakeOrg",
			},
			description:    "private org no githubtoken",
			expectedErrMsg: "user/organization private repos require env var GITHUB_TOKEN to be set",
			githubToken:    false,
		},
		{
			testOpts: Options{
				IncludePrivate: true,
				GithubUser:     "fakeUser",
			},
			description:    "private user no githubtoken",
			expectedErrMsg: "user/organization private repos require env var GITHUB_TOKEN to be set",
			githubToken:    false,
		},
		{
			testOpts: Options{
				IncludePrivate: true,
				GithubUser:     "fakeUser",
				GithubOrg:      "fakeOrg",
			},
			description:    "double owner",
			expectedErrMsg: "github user and organization set",
		},
		{
			testOpts: Options{
				IncludePrivate: true,
				GithubOrg:      "fakeOrg",
				OwnerPath:      "/dev/null",
			},
			description:    "local and remote target",
			expectedErrMsg: "github organization set and local owner path",
		},
		{
			testOpts: Options{
				IncludePrivate: true,
				GithubUser:     "fakeUser",
				OwnerPath:      "/dev/null",
			},
			description:    "local and remote target",
			expectedErrMsg: "github user set and local owner path",
		},
		{
			testOpts: Options{
				GithubUser:   "fakeUser",
				SingleSearch: "*/./....",
			},
			description:         "single search invalid regex gaurd",
			expectedErrMsgFuzzy: "unable to compile regex: */./...., ",
		},
		{
			testOpts: Options{
				GithubUser:   "fakeUser",
				SingleSearch: "mystring",
			},
			description:    "single search regex gaurd",
			expectedErrMsg: "",
		},
	}
	g := goblin.Goblin(t)
	for _, test := range tests {
		g.Describe("Test Option Gaurd", func() {
			g.It(test.description, func() {
				os.Clearenv()
				opts = test.testOpts
				if test.githubToken {
					os.Setenv("GITHUB_TOKEN", "fakeToken")
				}
				err := optsGuard()
				if err != nil {
					if test.expectedErrMsgFuzzy != "" {
						g.Assert(strings.Contains(err.Error(), test.expectedErrMsgFuzzy)).Equal(true)
					} else {
						g.Assert(err.Error()).Equal(test.expectedErrMsg)
					}
				} else {
					g.Assert("").Equal(test.expectedErrMsg)
				}

			})
		})
	}
}

func TestLoadToml(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "gitleaksTestConfigDir")
	defer os.RemoveAll(tmpDir)
	err := ioutil.WriteFile(path.Join(tmpDir, "gitleaksConfig"), []byte(defaultConfig), 0644)
	if err != nil {
		panic(err)
	}

	configPath := path.Join(tmpDir, "gitleaksConfig")
	noConfigPath := path.Join(tmpDir, "gitleaksConfigNope")

	var tests = []struct {
		testOpts       Options
		description    string
		configPath     string
		expectedErrMsg string
		singleSearch   bool
	}{
		{
			testOpts: Options{
				ConfigPath: configPath,
			},
			description: "path to config",
		},
		{
			testOpts:     Options{},
			description:  "env var path to no config",
			singleSearch: true,
		},
		{
			testOpts: Options{
				ConfigPath: noConfigPath,
			},
			description:    "no path to config",
			expectedErrMsg: fmt.Sprintf("no gitleaks config at %s", noConfigPath),
		},
		{
			testOpts:       Options{},
			description:    "env var path to config",
			configPath:     configPath,
			expectedErrMsg: "",
		},
		{
			testOpts:       Options{},
			description:    "env var path to no config",
			configPath:     noConfigPath,
			expectedErrMsg: fmt.Sprintf("problem loading config: open %s: no such file or directory", noConfigPath),
		},
	}

	g := goblin.Goblin(t)
	for _, test := range tests {
		g.Describe("TestLoadToml", func() {
			g.It(test.description, func() {
				opts = test.testOpts
				if test.singleSearch {
					singleSearchRegex = regexp.MustCompile("test")
				} else {
					singleSearchRegex = nil
				}
				if test.configPath != "" {
					os.Setenv("GITLEAKS_CONFIG", test.configPath)
				} else {
					os.Clearenv()
				}
				err := loadToml()
				if err != nil {
					g.Assert(err.Error()).Equal(test.expectedErrMsg)
				} else {
					g.Assert("").Equal(test.expectedErrMsg)
				}
			})
		})
	}
}

func BenchmarkAuditRepo1Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 1
	benchmarkRepo = getBenchmarkRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}

func BenchmarkAuditRepo2Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 2
	benchmarkRepo = getBenchmarkRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}

func BenchmarkAuditRepo4Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 4
	benchmarkRepo = getBenchmarkRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}

func BenchmarkAuditRepo8Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 8
	benchmarkRepo = getBenchmarkRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}

func BenchmarkAuditRepo10Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 10
	benchmarkRepo = getBenchmarkRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}

func BenchmarkAuditRepo100Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 100
	benchmarkRepo = getBenchmarkRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}

func BenchmarkAuditRepo1000Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 1000
	benchmarkRepo = getBenchmarkRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}
func BenchmarkAuditRepo10000Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 10000
	benchmarkRepo = getBenchmarkRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}
func BenchmarkAuditRepo100000Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 100000
	benchmarkRepo = getBenchmarkRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}
func BenchmarkAuditLeakRepo1Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 1
	benchmarkLeaksRepo = getBenchmarkLeaksRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}

func BenchmarkAuditLeakRepo2Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 2
	benchmarkLeaksRepo = getBenchmarkLeaksRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}

func BenchmarkAuditLeakRepo4Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 4
	benchmarkLeaksRepo = getBenchmarkLeaksRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}

func BenchmarkAuditLeakRepo8Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 8
	benchmarkLeaksRepo = getBenchmarkLeaksRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}

func BenchmarkAuditLeakRepo10Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 10
	benchmarkLeaksRepo = getBenchmarkLeaksRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}
func BenchmarkAuditLeakRepo100Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 100
	benchmarkLeaksRepo = getBenchmarkLeaksRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}
func BenchmarkAuditLeakRepo1000Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 1000
	benchmarkLeaksRepo = getBenchmarkLeaksRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}

func BenchmarkAuditLeakRepo10000Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 10000
	benchmarkLeaksRepo = getBenchmarkLeaksRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}

func BenchmarkAuditLeakRepo100000Proc(b *testing.B) {
	loadToml()
	opts.MaxGoRoutines = 100000
	benchmarkLeaksRepo = getBenchmarkLeaksRepo()
	for n := 0; n < b.N; n++ {
		auditGitRepo(benchmarkRepo)
	}
}
