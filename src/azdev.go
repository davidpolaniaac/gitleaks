package gitleaks

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops"
	"github.com/microsoft/azure-devops-go-api/azuredevops/git"

	log "github.com/sirupsen/logrus"
	gogit "gopkg.in/src-d/go-git.v4"
)

// auditGitlabRepos kicks off audits if --gitlab-user or --gitlab-org options are set.
// Getting all repositories from the GitLab API and run audit. If an error occurs during an audit of a repo,
// that error is logged.
func auditAzureDevOpsRepos() (int, error) {
	var (
		tempDir string
		err     error
		leaks   []Leak
	)

	organizationUrl := "https://dev.azure.com/" + opts.AzdevOrg // todo: replace value with your organization url
	personalAccessToken := os.Getenv("AZURE_DEVOPS_TOKEN")      // todo: replace value with your PAT

	// Create a connection to your organization
	connection := azuredevops.NewPatConnection(organizationUrl, personalAccessToken)

	ctx := context.Background()

	gitClient, err := git.NewClient(ctx, connection)
	if err != nil {
		log.Fatal(err)
	}

	repos, err := gitClient.GetRepositories(ctx, git.GetRepositoriesArgs{})
	if err != nil {
		log.Fatal(err)
	}

	log.Debugf("found repositories: %d", len(*repos))

	if tempDir, err = createAzureDevOpsTempDir(); err != nil {
		log.Fatal("error creating temp directory: ", err)
	}

	for _, p := range *repos {
		repo, err := cloneAzureDevopsRepo(tempDir, &p)
		if err != nil {
			log.Warn(err)
			os.RemoveAll(fmt.Sprintf("%s/%s", tempDir, p.Id))
			continue
		}

		err = repo.audit()
		if err != nil {
			log.Warn(err)
			os.RemoveAll(fmt.Sprintf("%s/%s", tempDir, p.Id))
			continue
		}

		os.RemoveAll(fmt.Sprintf("%s/%s", tempDir, p.Id))

		repo.report()
		leaks = append(leaks, repo.leaks...)
	}

	if opts.Report != "" {
		err = writeReport(leaks)
		if err != nil {
			return NoLeaks, err
		}
	}

	return len(leaks), nil
}

func createAzureDevOpsTempDir() (string, error) {

	pathName := opts.AzdevOrg

	os.RemoveAll(fmt.Sprintf("%s/%s", dir, pathName))

	ownerDir, err := ioutil.TempDir(dir, pathName)
	if err != nil {
		return "", err
	}

	return ownerDir, nil
}

func cloneAzureDevopsRepo(tempDir string, p *git.GitRepository) (*Repo, error) {
	var (
		repo *gogit.Repository
		err  error
	)

	gitAzureDevOpsToken := os.Getenv("AZURE_DEVOPS_TOKEN")

	log.Infof("cloning: %s", *p.Name)
	cloneTarget := fmt.Sprintf("%s/%s", tempDir, *p.Id)
	auth := "https://" + "fakeUsername:" + gitAzureDevOpsToken + "@"
	repository := strings.Replace(*p.WebUrl, "https://", auth, 1)
	cmdOutput, err := exec.Command("git", "clone", repository, cloneTarget).Output()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s", cmdOutput)
	repo, err = gogit.PlainOpen(cloneTarget)

	if err != nil {
		return nil, err
	}

	return &Repo{
		repository: repo,
		name:       *p.Name,
	}, nil
}
