package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

// LeakElem contains the line and commit of a leak
type LeakElem struct {
	Line     string `json:"line"`
	Commit   string `json:"commit"`
	Offender string `json:"string"`
	Reason   string `json:"reason"`
}

// start clones and determines if there are any leaks
func start(opts *Options) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	fmt.Printf("Cloning \x1b[37;1m%s\x1b[0m...\n", opts.RepoURL)
	err := exec.Command("git", "clone", opts.RepoURL).Run()
	if err != nil {
		log.Printf("failed to clone repo %v", err)
		return
	}
	fmt.Printf("Evaluating \x1b[37;1m%s\x1b[0m...\n", opts.RepoURL)
	repoName := getLocalRepoName(opts.RepoURL)
	if err = os.Chdir(repoName); err != nil {
		log.Fatal(err)
	}
	go func() {
		<-c
		cleanup(repoName)
		os.Exit(1)
	}()

	report := getLeaks(repoName, opts)
	if len(report) == 0 {
		fmt.Printf("No Leaks detected for \x1b[35;2m%s\x1b[0m...\n\n", opts.RepoURL)
	}
	cleanup(repoName)
	reportJSON, _ := json.MarshalIndent(report, "", "\t")
	err = ioutil.WriteFile(fmt.Sprintf("%s_leaks.json", repoName), reportJSON, 0644)
	if err != nil {
		log.Fatalf("Can't write to file: %s", err)
	}
}

// getLocalRepoName generates the name of the local clone folder based on the given URL
func getLocalRepoName(url string) string {
	splitSlashes := strings.Split(url, "/")
	name := splitSlashes[len(splitSlashes)-1]
	name = strings.TrimSuffix(name, ".git")
	splitColons := strings.Split(name, ":")
	name = splitColons[len(splitColons)-1]

	return name
}

// cleanup deletes the repo
func cleanup(repoName string) {
	if err := os.Chdir(appRoot); err != nil {
		log.Fatalf("failed cleaning up repo. Does the repo exist? %v", err)
	}
	err := exec.Command("rm", "-rf", repoName).Run()
	if err != nil {
		log.Fatal(err)
	}
}

// getLeaks will attempt to find gitleaks
func getLeaks(repoName string, opts *Options) []LeakElem {
	var (
		out               []byte
		err               error
		commitWG          sync.WaitGroup
		gitLeakReceiverWG sync.WaitGroup
		gitLeaks          = make(chan LeakElem)
		report            []LeakElem
	)
	semaphoreChan := make(chan struct{}, opts.Concurrency)

	go func(commitWG *sync.WaitGroup, gitLeakReceiverWG *sync.WaitGroup) {
		for gitLeak := range gitLeaks {
			b, err := json.MarshalIndent(gitLeak, "", "   ")
			if err != nil {
				fmt.Println("failed to output leak:", err)
			}
			fmt.Println(string(b))
			report = append(report, gitLeak)
			gitLeakReceiverWG.Done()
		}
	}(&commitWG, &gitLeakReceiverWG)

	out, err = exec.Command("git", "rev-list", "--all", "--remotes", "--topo-order").Output()
	if err != nil {
		log.Fatalf("error retrieving commits%v\n", err)
	}

	commits := bytes.Split(out, []byte("\n"))
	for _, currCommitB := range commits {
		currCommit := string(currCommitB)
		if currCommit == "" {
			continue
		}
		if currCommit == opts.SinceCommit {
			break
		}

		commitWG.Add(1)
		go func(currCommit string, repoName string, commitWG *sync.WaitGroup,
			gitLeakReceiverWG *sync.WaitGroup) {

			defer commitWG.Done()

			if err := os.Chdir(fmt.Sprintf("%s/%s", appRoot, repoName)); err != nil {
				log.Fatal(err)
			}

			commitCmp := fmt.Sprintf("%s^!", currCommit)
			semaphoreChan <- struct{}{}
			out, err := exec.Command("git", "diff", commitCmp).Output()
			<-semaphoreChan

			if err != nil {
				fmt.Printf("error retrieving diff for commit %s try turning concurrency factor down %v\n", currCommit, err)
				cleanup(repoName)
				return
			}

			leaks := doChecks(string(out), currCommit)
			if len(leaks) == 0 {
				return
			}
			for _, leak := range leaks {
				gitLeakReceiverWG.Add(1)
				gitLeaks <- leak
			}

		}(currCommit, repoName, &commitWG, &gitLeakReceiverWG)
	}

	commitWG.Wait()
	gitLeakReceiverWG.Wait()
	return report
}
