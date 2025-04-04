package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/v70/github"
)

type userRepoLink struct {
	User    string
	Repo    string
	Checked bool
}

type config struct {
	AnchorTextsMap map[string]bool
	LinksMap       map[string]userRepoLink
}

func main() {
	// Create the config, maybe later load from file
	anchorTextsMap := make(map[string]bool)
	linksMap := make(map[string]userRepoLink)
	config := config{
		AnchorTextsMap: anchorTextsMap,
		LinksMap:       linksMap,
	}
	// Create client a github api client
	context := context.Background()
	client := github.NewClient(nil)
	// InitiateRepo
	intialRepo := "https://github.com/sindresorhus/awesome"

	initialUserRepo, err := getUserAndRepo(intialRepo)
	if err != nil {
		fmt.Printf("Error initializaing program: %v\n", err)
	}
	initialUserRepo.Checked = true
	config.LinksMap[intialRepo] = initialUserRepo
	initialContent, err := getReadme(context, client, initialUserRepo.User, initialUserRepo.Repo)
	if err != nil {
		fmt.Printf("failed to get readme: %v", err)
		return
	}
	config.saveLinksAndTextFromReadme(initialContent)

	// work through links until it throws an error this will happen when no new links are to be found
	noLinks := false
	for !noLinks {
		workingList, err := config.userRepoList()
		if err != nil {
			fmt.Printf("error creating workingList: %v\n", err)
			noLinks = true
			continue
		}
		config.processList(workingList, context, client)
	}
	err = config.saveLinkFile()
	if err != nil {
		fmt.Printf("failed to save linkfile:%v\n", err)
	}
	err = config.saveAnchorMap()
	if err != nil {
		fmt.Printf("failed to save anchorFile: %v\n", err)
	}
}

func (config config) saveLinkFile() error {
	err := os.Mkdir("out", 0750)
	if err != nil {
		fmt.Printf("Error: creating directory: %v\n", err)
	}
	file, err := os.Create("out/linkFile")
	if err != nil {
		return err
	}
	defer file.Close()
	buffer := new(bytes.Buffer)
	encoder := gob.NewEncoder(buffer)
	err = encoder.Encode(config.LinksMap)
	if err != nil {
		return err
	}
	wBytes, err := file.Write(buffer.Bytes())
	if err != nil {
		return err
	}
	fmt.Printf("bytes written for anchorfile: %v\n", wBytes)
	return nil
}

func (config config) saveAnchorMap() error {
	err := os.Mkdir("out", 0750)
	if err != nil {
		return err
	}
	anchorFile, err := os.Create("out/wordsFile")
	if err != nil {
		return err
	}
	defer anchorFile.Close()
	buffer := new(bytes.Buffer)
	encoder := gob.NewEncoder(buffer)
	err = encoder.Encode(config.AnchorTextsMap)
	if err != nil {
		return err
	}
	wBytes, err := anchorFile.Write(buffer.Bytes())
	if err != nil {
		return err
	}
	fmt.Printf("bytes written for anchorfile: %v\n", wBytes)
	return nil
}

func (cfg config) processList(input []userRepoLink, context context.Context, client *github.Client) {
	for _, val := range input {
		currentContent, err := getReadme(context, client, val.User, val.Repo)
		if err != nil {
			fmt.Printf("error when processing and getting readme: %v\n", err)
			continue
		}
		cfg.saveLinksAndTextFromReadme(currentContent)
	}
}

func (cfg config) userRepoList() ([]userRepoLink, error) {
	outList := []userRepoLink{}
	for key, val := range cfg.LinksMap {
		if val.Checked {
			continue
		}
		outList = append(outList, val)
		currentUserRepo := val
		currentUserRepo.Checked = true
		cfg.LinksMap[key] = currentUserRepo
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
		anchorText, err := extractAnchorText(line)
		if err != nil {
			fmt.Printf("Error: %v", err)
			continue
		}
		cfg.AnchorTextsMap[anchorText] = true
		link, err := extractLink(line)
		if link == "" {
			continue
		}
		if err != nil {
			fmt.Printf("Error: %v", err)
			continue
		}
		_, ok := cfg.LinksMap[link]
		if ok {
			continue
		}
		linkInfo, err := getUserAndRepo(link)
		if err != nil {
			fmt.Printf("error getting user and repo: %v\n", err)
			continue
		}
		cfg.LinksMap[link] = linkInfo
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

func getReadme(ctx context.Context, client *github.Client, owner, repo string) (string, error) {
	// gets the readme and retunts it as a string
	readme, _, err := client.Repositories.GetReadme(ctx, owner, repo, nil)
	if err != nil {
		return "", err
	}
	content, err := readme.GetContent()
	if err != nil {
		return "", err
	}
	return content, nil
}

func extractAnchorText(line string) (string, error) {
	// Extracts the text inbetween square braces
	splitFirstBrace := strings.Split(line, "[")
	if len(splitFirstBrace) < 2 {
		return "", fmt.Errorf("failed to split at first brace input: %v", line)
	}
	splitSecondBrace := strings.Split(splitFirstBrace[1], "]")
	if len(splitSecondBrace) < 2 {
		return "", fmt.Errorf("failed to split at second brace input: %v", line)
	}
	return splitSecondBrace[0], nil
}
