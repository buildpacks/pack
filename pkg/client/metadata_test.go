package client

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	h "github.com/buildpacks/pack/testhelpers"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestMetadata(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Metadata", testMetadata, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testMetadata(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir  string
		repo    *git.Repository
		commits []plumbing.Hash
	)

	it.Before(func() {
		var err error

		tmpDir, err = ioutil.TempDir("", "test-repo")
		h.AssertNil(t, err)

		repo, err = git.PlainInit(tmpDir, false)
		h.AssertNil(t, err)

		commits = createCommits(t, repo, tmpDir, 5)
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(tmpDir))
	})

	when("#generateTagsMap", func() {
		when("repository has no tags", func() {
			it("should return empty map", func() {
				commitTagsMap := generateTagsMap(repo)
				h.AssertEq(t, len(commitTagsMap), 0)
			})
		})

		when("repository has only unannotated tags", func() {
			it("returns correct map if commits only have one tag", func() {
				for i := 0; i < 4; i++ {
					createUnannotatedTag(t, repo, commits[i])
				}

				commitTagsMap := generateTagsMap(repo)
				h.AssertEq(t, len(commitTagsMap), 4)
				for i := 0; i < 4; i++ {
					tagsInfo, shouldExist := commitTagsMap[commits[i].String()]
					h.AssertEq(t, shouldExist, true)
					h.AssertNotEq(t, tagsInfo[0].Name, "")
					h.AssertEq(t, tagsInfo[0].Type, "unannotated")
					h.AssertEq(t, tagsInfo[0].Message, "")
				}
				_, shouldNotExist := commitTagsMap[commits[3].String()]
				h.AssertEq(t, shouldNotExist, true)
			})

			it("returns map sorted by ascending tag name if commits have multiple tags", func() {
				for i := 0; i < 4; i++ {
					for j := 0; j <= rand.Intn(10); j++ {
						createUnannotatedTag(t, repo, commits[i])
					}
				}

				commitTagsMap := generateTagsMap(repo)
				h.AssertEq(t, len(commitTagsMap), 4)
				for i := 0; i < 4; i++ {
					tagsInfo, shouldExist := commitTagsMap[commits[i].String()]
					h.AssertEq(t, shouldExist, true)

					tagsSortedByName := sort.SliceIsSorted(tagsInfo, func(i, j int) bool {
						return tagsInfo[i].Name < tagsInfo[j].Name
					})
					h.AssertEq(t, tagsSortedByName, true)
				}
			})
		})

		when("repository has only annotated tags", func() {
			it("returns correct map if commits only have one tag", func() {
				for i := 0; i < 4; i++ {
					createAnnotatedTag(t, repo, commits[i])
				}

				commitTagsMap := generateTagsMap(repo)
				h.AssertEq(t, len(commitTagsMap), 4)
				for i := 0; i < 4; i++ {
					tagsInfo, shouldExist := commitTagsMap[commits[i].String()]
					h.AssertEq(t, shouldExist, true)
					h.AssertNotEq(t, tagsInfo[0].Name, "")
					h.AssertEq(t, tagsInfo[0].Type, "annotated")
					h.AssertNotEq(t, tagsInfo[0].Message, "")
				}
				_, shouldNotExist := commitTagsMap[commits[3].String()]
				h.AssertEq(t, shouldNotExist, true)
			})

			it("returns map sorted by descending tag creation time if commits have multiple tags", func() {
				for i := 0; i < 4; i++ {
					for j := 0; j <= rand.Intn(10); j++ {
						createAnnotatedTag(t, repo, commits[i])
					}
				}

				commitTagsMap := generateTagsMap(repo)
				h.AssertEq(t, len(commitTagsMap), 4)
				for i := 0; i < 4; i++ {
					tagsInfo, shouldExist := commitTagsMap[commits[i].String()]
					h.AssertEq(t, shouldExist, true)

					tagsSortedByTime := sort.SliceIsSorted(tagsInfo, func(i, j int) bool {
						return tagsInfo[i].TagTime.After(tagsInfo[j].TagTime)
					})
					h.AssertEq(t, tagsSortedByTime, true)
				}
				_, shouldNotExist := commitTagsMap[commits[3].String()]
				h.AssertEq(t, shouldNotExist, true)
			})
		})

		when("repository has both annotated and unannotated tags", func() {
			it("returns map with annotated tags prior to unnanotated if commits have multiple tags", func() {
				for i := 0; i < 4; i++ {
					for j := 0; j <= rand.Intn(10); j++ {
						createAnnotatedTag(t, repo, commits[i])
					}
					for j := 0; j <= rand.Intn(10); j++ {
						createUnannotatedTag(t, repo, commits[i])
					}
				}

				commitTagsMap := generateTagsMap(repo)
				h.AssertEq(t, len(commitTagsMap), 4)
				for i := 0; i < 4; i++ {
					tagsInfo, shouldExist := commitTagsMap[commits[i].String()]
					h.AssertEq(t, shouldExist, true)

					tagsSortedByType := sort.SliceIsSorted(tagsInfo, func(i, j int) bool {
						if tagsInfo[i].Type == "annotated" && tagsInfo[j].Type == "unannotated" {
							return true
						}
						return false
					})
					h.AssertEq(t, tagsSortedByType, true)
				}
			})
		})
	})
}

func createUnannotatedTag(t *testing.T, repo *git.Repository, commitHash plumbing.Hash) {
	version := rand.Float32()*10 + float32(rand.Intn(20))
	tagName := fmt.Sprintf("v%f-lw", version)
	_, err := repo.CreateTag(tagName, commitHash, nil)
	h.AssertNil(t, err)
}

func createAnnotatedTag(t *testing.T, repo *git.Repository, commitHash plumbing.Hash) {
	version := rand.Float32()*10 + float32(rand.Intn(20))
	tagName := fmt.Sprintf("v%f-%s", version, h.RandString(5))
	tagMessage := fmt.Sprintf("This is an annotated tag for version - %s", tagName)
	tagOpts := git.CreateTagOptions{
		Message: tagMessage,
		Tagger: &object.Signature{
			Name:  "Test Tagger",
			Email: "testtagger@test.com",
			When:  time.Now().Add(time.Hour*time.Duration(rand.Intn(100)) + time.Minute*time.Duration(rand.Intn(100))),
		},
	}
	_, err := repo.CreateTag(tagName, commitHash, &tagOpts)
	h.AssertNil(t, err)
}

func createCommits(t *testing.T, repo *git.Repository, repoPath string, numberOfCommits int) []plumbing.Hash {
	worktree, err := repo.Worktree()
	h.AssertNil(t, err)

	var commitHashes []plumbing.Hash
	for i := 0; i < numberOfCommits; i++ {
		file, err := ioutil.TempFile(repoPath, h.RandString(10))
		h.AssertNil(t, err)

		_, err = worktree.Add(filepath.Base(file.Name()))
		h.AssertNil(t, err)

		commitMsg := fmt.Sprintf("%s %d", "test commit number", i)
		commitOpts := git.CommitOptions{}
		commitHash, err := worktree.Commit(commitMsg, &commitOpts)
		h.AssertNil(t, err)
		commitHashes = append(commitHashes, commitHash)
	}
	return commitHashes
}
