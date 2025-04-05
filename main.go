package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/v70/github"
	"golang.org/x/oauth2"
)

type userRepoLink struct {
	User    string
	Repo    string
	Checked bool
}

type config struct {
	RepoMap map[string]userRepoLink
	Secret  string
}

func main() {
	err := os.Mkdir("out", 0750)
	if err != nil {
		fmt.Printf("Error: creating directory: %v\n", err)
	}
	// Create the config, maybe later load from file

	repoMap, err := readLinkFile()
	if err != nil {
		fmt.Printf("cant load link map using empty map: %v", err)
	}

	secret, err := os.ReadFile("secrets")
	if err != nil {
		fmt.Printf("failed to read secrets: %v\n", err)
	}
	secretString := string(secret)
	secretString = strings.Trim(secretString, " ")
	secretString = strings.Trim(secretString, "\n")
	config := config{
		RepoMap: repoMap,
		Secret:  secretString,
	}

	context := context.Background()
	// Create oauth2 client so we can authenticate with github
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: config.Secret})
	tc := oauth2.NewClient(context, ts)

	// Create a github api client

	client := github.NewClient(tc)

	// InitiateRepo
	intialRepo := "https://github.com/sindresorhus/awesome"

	initialUserRepo, err := getUserAndRepo(intialRepo)
	if err != nil {
		fmt.Printf("Error initializaing program: %v\n", err)
	}
	initialContent, err := config.getReadme(context, client, initialUserRepo.User, initialUserRepo.Repo)
	if err != nil {
		fmt.Printf("failed to get readme: %v", err)
		return
	}
	config.saveLinksAndTextFromReadme(initialContent)
	// work through links until it throws an error this will happen when no new links are to be found
	endProcess := false
	for !endProcess {
		workingList, err := config.userRepoList()
		if err != nil {
			fmt.Printf("error creating workingList: %v\n", err)
			endProcess = true
			continue
		}
		err = config.processList(workingList, context, client)
		if err != nil {
			if strings.Contains(err.Error(), "403") {
				fmt.Printf("error processing list: %v\n", err)
				endProcess = true
			}
		}
	}
	err = config.saveLinkFile()
	if err != nil {
		fmt.Printf("failed to save linkfile:%v\n", err)
	}
}

func readLinkFile() (map[string]userRepoLink, error) {
	m := make(map[string]userRepoLink)
	_, err := os.Stat("/home/obegarde/workspace/github.com/obegarde/gh-readme-downloader/out/linkFile")
	if err != nil {
		return m, err
	}
	file, err := os.Open("/home/obegarde/workspace/github.com/obegarde/gh-readme-downloader/out/linkFile")
	if err != nil {
		return m, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	dec := gob.NewDecoder(reader)
	err = dec.Decode(&m)
	if err != nil {
		return m, err
	}
	return m, nil
}

func (config config) saveLinkFile() error {
	file, err := os.Create("out/linkFile")
	if err != nil {
		return err
	}
	defer file.Close()
	buffer := new(bytes.Buffer)
	encoder := gob.NewEncoder(buffer)
	err = encoder.Encode(config.RepoMap)
	if err != nil {
		return err
	}
	wBytes, err := file.Write(buffer.Bytes())
	if err != nil {
		return err
	}
	fmt.Printf("bytes written for linkFile: %v\n", wBytes)
	return nil
}

func saveReadMe(readme string, owner string, repo string) error {
	filepath := fmt.Sprintf("out/%v%v", owner, repo)
	content := []byte(readme)
	err := os.WriteFile(filepath, content, 0644)
	if err != nil {
		return err
	}
	return nil
}

func (config config) processList(input []userRepoLink, context context.Context, client *github.Client) error {
	for _, val := range input {
		currentContent, err := config.getReadme(context, client, val.User, val.Repo)
		if err != nil {
			if strings.Contains(err.Error(), "403") {
				return err
			}
			fmt.Printf("error when processing and getting readme: %v\n", err)
			continue
		}
		config.saveLinksAndTextFromReadme(currentContent)
	}
	return nil
}

func (cfg config) userRepoList() ([]userRepoLink, error) {
	outList := []userRepoLink{}
	for _, val := range cfg.RepoMap {
		if val.Checked {
			continue
		}
		outList = append(outList, val)
	}
	if len(outList) == 0 {
		return nil, fmt.Errorf("no unchecked links")
	}
	return outList, nil
}

func (cfg config) saveLinksAndTextFromReadme(readme string) error {
	// iterate over the newlines of the readme
	for line := range strings.SplitSeq(readme, "\n") {
		// skips lines without an anchor tag
		if !strings.Contains(line, "[") {
			continue
		}
		// Skip lines that are too short to be links with text and url
		if len(line) < 4 {
			continue
		}
		// skip lines that arent bulletpoints
		if string(line[0]) != "-" {
			continue
		}

		link, err := extractLink(line)
		if link == "" {
			continue
		}
		if err != nil {
			fmt.Printf("Error: %v", err)
			continue
		}
		_, ok := cfg.RepoMap[link]
		if ok {
			continue
		}
		linkInfo, err := getUserAndRepo(link)
		if err != nil {
			fmt.Printf("error getting user and repo: %v\n", err)
			continue
		}
		cfg.RepoMap[link] = linkInfo
	}
	return nil
}

func getUserAndRepo(link string) (userRepoLink, error) {
	splitSlice := strings.Split(link, "/")
	if len(splitSlice) < 5 {
		return userRepoLink{}, fmt.Errorf("too short link")
	}
	user := splitSlice[3]
	repo := splitSlice[4]
	if strings.Contains(repo, "#readme") {
		repo = strings.Split(repo, "#")[0]
	}
	return userRepoLink{
		User:    user,
		Repo:    repo,
		Checked: false,
	}, nil
}

func extractLink(line string) (string, error) {
	// split the line by parenthesis to get the link
	splitFirstParenthesis := strings.Split(line, "(")
	if len(splitFirstParenthesis) < 2 {
		return "", fmt.Errorf("failed to split at first parenthesis: %v", line)
	}
	splitSecondParenthesis := strings.Split(splitFirstParenthesis[1], ")")
	if len(splitSecondParenthesis) < 2 {
		return "", fmt.Errorf("failed to split at second parenthesis %v", line)
	}
	link := splitSecondParenthesis[0]
	if !strings.Contains(link, "github") || strings.Contains(link, "awesome") {
		return "", nil
	}
	return link, nil
}

func (config config) getReadme(ctx context.Context, client *github.Client, owner, repo string) (string, error) {
	// gets the readme and retunts it as a string
	readme, _, err := client.Repositories.GetReadme(ctx, owner, repo, nil)
	if err != nil {
		return "", err
	}
	content, err := readme.GetContent()
	if err != nil {
		return "", err
	}
	err = saveReadMe(content, owner, repo)
	if err != nil {
		return "", err
	}
	NewRepo := userRepoLink{
		User:    owner,
		Repo:    repo,
		Checked: true,
	}
	comboUserRepo := fmt.Sprintf("%v%v", owner, repo)

	config.RepoMap[comboUserRepo] = NewRepo
	return content, nil
}
