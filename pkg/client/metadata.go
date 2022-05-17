package client

import (
	"sort"
	"strings"
	"time"

	"github.com/buildpacks/lifecycle/platform"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

type TagInfo struct {
	Name    string
	Message string
	Type    string
	TagHash string
	TagTime time.Time
}

func GitMetadata(appPath string) *platform.ProjectSource {
	// appPath2 := "/Users/nitish/OSS/pack"
	repo, err := git.PlainOpen(appPath)
	if err != nil {
		print("unable to open git repo")
	}
	headRef, err := repo.Head()
	if err != nil {
		print("unable to parse head")
	}
	commitTagMap := generateTagsMap(repo)

	describe := parseGitDescribe(repo, headRef, commitTagMap)
	refs := parseGitRefs(repo, headRef, commitTagMap)

	projectSource := &platform.ProjectSource{
		Type: "git",
		Version: map[string]interface{}{
			"commit":   headRef.Hash().String(),
			"describe": describe,
		},
		Metadata: map[string]interface{}{
			"refs": refs,
			"url":  "nitis",
		},
	}
	return projectSource
}

func generateTagsMap(repo *git.Repository) map[string][]TagInfo {
	commitTagMap := make(map[string][]TagInfo)
	tags, err := repo.Tags()
	if err != nil {
		return commitTagMap
	}

	tags.ForEach(func(ref *plumbing.Reference) error {
		tagObj, err := repo.TagObject(ref.Hash())
		switch err {
		case nil:
			commitTagMap[tagObj.Target.String()] = append(
				commitTagMap[tagObj.Target.String()],
				TagInfo{Name: tagObj.Name, Message: tagObj.Message, Type: "annotated", TagHash: ref.Hash().String(), TagTime: tagObj.Tagger.When},
			)
		case plumbing.ErrObjectNotFound:
			commitTagMap[ref.Hash().String()] = append(
				commitTagMap[ref.Hash().String()],
				TagInfo{Name: getRefName(ref.Name().String()), Message: "", Type: "unannotated", TagHash: ref.Hash().String(), TagTime: time.Now()},
			)
		default:
			return err
		}
		return nil
	})

	for _, tagRefs := range commitTagMap {
		sort.Slice(tagRefs, func(i, j int) bool {
			if tagRefs[i].Type == "annotated" && tagRefs[j].Type == "annotated" {
				return tagRefs[i].TagTime.After(tagRefs[j].TagTime)
			}
			if tagRefs[i].Type == "unannotated" && tagRefs[j].Type == "unannotated" {
				return tagRefs[i].Name < tagRefs[j].Name
			}
			if tagRefs[i].Type == "annotated" && tagRefs[j].Type == "unannotated" {
				return true
			}
			return false
		})
	}
	return commitTagMap
}

func parseGitDescribe(repo *git.Repository, headRef *plumbing.Reference, commitTagMap map[string][]TagInfo) string {
	logOpts := &git.LogOptions{
		From:  headRef.Hash(),
		Order: git.LogOrderCommitterTime,
	}
	commits, err := repo.Log(logOpts)
	if err != nil {
		print("no commits found")
	}

	latestTag := headRef.Hash().String()
	for {
		commitInfo, err := commits.Next()
		if err == nil {
			if refs, exists := commitTagMap[commitInfo.Hash.String()]; exists {
				latestTag = refs[0].Name
			}
		} else {
			break
		}
	}
	return latestTag
}

func parseGitRefs(repo *git.Repository, headRef *plumbing.Reference, commitTagMap map[string][]TagInfo) []string {
	var parsedRefs []string
	parsedRefs = append(parsedRefs, getRefName(headRef.Name().String()))
	if refs, exists := commitTagMap[headRef.Hash().String()]; exists {
		for _, ref := range refs {
			parsedRefs = append(parsedRefs, ref.Name)
		}
	}
	return parsedRefs
}

func getRefName(ref string) string {
	if refSplit := strings.Split(ref, "/"); len(refSplit) == 3 {
		return refSplit[2]
	}
	return ""
}
