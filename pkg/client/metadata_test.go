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
	"github.com/go-git/go-git/v5/config"
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
			it("returns empty map", func() {
				commitTagsMap := generateTagsMap(repo)
				h.AssertEq(t, len(commitTagsMap), 0)
			})
		})

		when("repository has only unannotated tags", func() {
			it("returns correct map if commits only have one tag", func() {
				for i := 0; i < 4; i++ {
					createUnannotatedTag(t, repo, commits[i], "")
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
						createUnannotatedTag(t, repo, commits[i], "")
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
					createAnnotatedTag(t, repo, commits[i], "")
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
						createAnnotatedTag(t, repo, commits[i], "")
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
			it("returns map where annotated tags exist prior to unnanotated if commits have multiple tags", func() {
				for i := 0; i < 4; i++ {
					for j := 0; j <= rand.Intn(10); j++ {
						createAnnotatedTag(t, repo, commits[i], "")
					}
					for j := 0; j <= rand.Intn(10); j++ {
						createUnannotatedTag(t, repo, commits[i], "")
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

	when("#parseGitDescribe", func() {
		when("tags are defined only in single branch", func() {
			when("repository has no tags", func() {
				it("returns latest commit hash", func() {
					commitTagsMap := generateTagsMap(repo)
					headRef, err := repo.Head()
					h.AssertNil(t, err)

					output := parseGitDescribe(repo, headRef, commitTagsMap)
					h.AssertEq(t, output, commits[len(commits)-1].String())
				})
			})

			when("repository has only unannotated tags", func() {
				it("returns first tag encountered from HEAD", func() {
					for i := 0; i < 3; i++ {
						tagName := fmt.Sprintf("v0.%d-lw", i+1)
						createUnannotatedTag(t, repo, commits[i], tagName)
					}

					commitTagsMap := generateTagsMap(repo)
					headRef, err := repo.Head()
					h.AssertNil(t, err)
					output := parseGitDescribe(repo, headRef, commitTagsMap)
					h.AssertEq(t, output, "v0.3-lw")
				})

				it("returns proper tag name for tags containing `/`", func() {
					tagName := "v0.1/testing"
					t.Logf("Checking output for tag name: %s", tagName)
					createUnannotatedTag(t, repo, commits[0], tagName)

					commitTagsMap := generateTagsMap(repo)
					headRef, err := repo.Head()
					h.AssertNil(t, err)
					output := parseGitDescribe(repo, headRef, commitTagsMap)
					h.AssertContains(t, output, "v0.1/testing")
				})
			})

			when("repository has only annotated tags", func() {
				it("returns first tag encountered from HEAD", func() {
					for i := 0; i < 3; i++ {
						tagName := fmt.Sprintf("v0.%d", i+1)
						createAnnotatedTag(t, repo, commits[i], tagName)
					}

					commitTagsMap := generateTagsMap(repo)
					headRef, err := repo.Head()
					h.AssertNil(t, err)
					output := parseGitDescribe(repo, headRef, commitTagsMap)
					h.AssertEq(t, output, "v0.3")
				})
			})

			when("repository has both annotated and unannotated tags", func() {
				when("each commit has only one tag", func() {
					it("returns the first tag encountered from HEAD if unannotated tag comes first", func() {
						createAnnotatedTag(t, repo, commits[0], "ann-tag-at-commit-0")
						createUnannotatedTag(t, repo, commits[1], "unann-tag-at-commit-1")
						createAnnotatedTag(t, repo, commits[2], "ann-tag-at-commit-2")
						createUnannotatedTag(t, repo, commits[3], "unann-tag-at-commit-3")
						createUnannotatedTag(t, repo, commits[4], "unann-tag-at-commit-4")

						commitTagsMap := generateTagsMap(repo)
						headRef, err := repo.Head()
						h.AssertNil(t, err)
						output := parseGitDescribe(repo, headRef, commitTagsMap)
						h.AssertEq(t, output, "unann-tag-at-commit-4")
					})

					it("returns the first tag encountered from HEAD if annotated tag comes first", func() {
						createAnnotatedTag(t, repo, commits[0], "ann-tag-at-commit-0")
						createUnannotatedTag(t, repo, commits[1], "unann-tag-at-commit-1")
						createAnnotatedTag(t, repo, commits[2], "ann-tag-at-commit-2")
						createAnnotatedTag(t, repo, commits[3], "ann-tag-at-commit-3")

						commitTagsMap := generateTagsMap(repo)
						headRef, err := repo.Head()
						h.AssertNil(t, err)
						output := parseGitDescribe(repo, headRef, commitTagsMap)
						h.AssertEq(t, output, "ann-tag-at-commit-3")
					})

					it("returns the tag at HEAD if annotated tag exists at HEAD", func() {
						createAnnotatedTag(t, repo, commits[4], "ann-tag-at-HEAD")

						commitTagsMap := generateTagsMap(repo)
						headRef, err := repo.Head()
						h.AssertNil(t, err)
						output := parseGitDescribe(repo, headRef, commitTagsMap)
						h.AssertEq(t, output, "ann-tag-at-HEAD")
					})

					it("returns the tag at HEAD if unannotated tag exists at HEAD", func() {
						createUnannotatedTag(t, repo, commits[4], "unann-tag-at-HEAD")

						commitTagsMap := generateTagsMap(repo)
						headRef, err := repo.Head()
						h.AssertNil(t, err)
						output := parseGitDescribe(repo, headRef, commitTagsMap)
						h.AssertEq(t, output, "unann-tag-at-HEAD")
					})
				})

				when("commits have multiple tags", func() {
					it("returns most recently created tag if a commit has multiple annotated tags", func() {
						createAnnotatedTag(t, repo, commits[1], "ann-tag-1-at-commit-1")
						createAnnotatedTag(t, repo, commits[2], "ann-tag-1-at-commit-2")
						createAnnotatedTag(t, repo, commits[2], "ann-tag-2-at-commit-2")
						createAnnotatedTag(t, repo, commits[2], "ann-tag-3-at-commit-2")

						commitTagsMap := generateTagsMap(repo)
						headRef, err := repo.Head()
						h.AssertNil(t, err)

						output := parseGitDescribe(repo, headRef, commitTagsMap)
						tagsAtCommit := commitTagsMap[commits[2].String()]
						h.AssertEq(t, output, tagsAtCommit[0].Name)
						for i := 1; i < len(tagsAtCommit); i++ {
							h.AssertEq(t, tagsAtCommit[i].TagTime.Before(tagsAtCommit[0].TagTime), true)
						}
					})

					it("returns the tag name that comes first when sorted alphabetically if a commit has multiple unannotated tags", func() {
						createUnannotatedTag(t, repo, commits[1], "ann-tag-1-at-commit-1")
						createUnannotatedTag(t, repo, commits[2], "v0.000002-lw")
						createUnannotatedTag(t, repo, commits[2], "v0.0002-lw")
						createUnannotatedTag(t, repo, commits[2], "v1.0002-lw")

						commitTagsMap := generateTagsMap(repo)
						headRef, err := repo.Head()
						h.AssertNil(t, err)

						output := parseGitDescribe(repo, headRef, commitTagsMap)
						h.AssertEq(t, output, "v0.000002-lw")
					})

					it("returns annotated tag is a commit has both annotated and unannotated tags", func() {
						createAnnotatedTag(t, repo, commits[1], "ann-tag-1-at-commit-1")
						createAnnotatedTag(t, repo, commits[2], "ann-tag-1-at-commit-2")
						createUnannotatedTag(t, repo, commits[2], "unann-tag-1-at-commit-2")

						commitTagsMap := generateTagsMap(repo)
						headRef, err := repo.Head()
						h.AssertNil(t, err)

						output := parseGitDescribe(repo, headRef, commitTagsMap)
						h.AssertEq(t, output, "ann-tag-1-at-commit-2")
					})
				})
			})
		})

		// TODO: tests for tags in different branches
	})

	when("#parseGitRefs", func() {
		when("HEAD is not at a tag", func() {
			it("returns branch name if checked out branch is `master`", func() {
				commitTagsMap := generateTagsMap(repo)
				headRef, err := repo.Head()
				h.AssertNil(t, err)
				output := parseGitRefs(repo, headRef, commitTagsMap)
				expectedOutput := []string{"master"}
				h.AssertEq(t, output, expectedOutput)
			})

			it("returns branch name if checked out branch is not `master`", func() {
				worktree, err := repo.Worktree()
				h.AssertNil(t, err)
				checkoutOpts := &git.CheckoutOptions{
					Branch: plumbing.ReferenceName("refs/heads/tests/05-05/test-branch"),
					Create: true,
				}
				err = worktree.Checkout(checkoutOpts)
				h.AssertNil(t, err)
				createCommits(t, repo, tmpDir, 1)

				commitTagsMap := generateTagsMap(repo)
				headRef, err := repo.Head()
				h.AssertNil(t, err)
				output := parseGitRefs(repo, headRef, commitTagsMap)
				expectedOutput := []string{"tests/05-05/test-branch"}
				h.AssertEq(t, output, expectedOutput)
			})
		})

		when("HEAD is at a commit with single tag", func() {
			it("returns annotated tag and branch name", func() {
				createAnnotatedTag(t, repo, commits[len(commits)-1], "test-tag")
				commitTagsMap := generateTagsMap(repo)
				headRef, err := repo.Head()
				h.AssertNil(t, err)
				output := parseGitRefs(repo, headRef, commitTagsMap)
				expectedOutput := []string{"master", "test-tag"}
				h.AssertEq(t, output, expectedOutput)
			})

			it("returns unannotated tag and branch name", func() {
				createUnannotatedTag(t, repo, commits[len(commits)-1], "test-tag")
				commitTagsMap := generateTagsMap(repo)
				headRef, err := repo.Head()
				h.AssertNil(t, err)
				output := parseGitRefs(repo, headRef, commitTagsMap)
				expectedOutput := []string{"master", "test-tag"}
				h.AssertEq(t, output, expectedOutput)
			})
		})

		when("HEAD is at a commit with multiple tags", func() {
			it("returns correct tag names if all tags are unannotated", func() {
				createUnannotatedTag(t, repo, commits[len(commits)-2], "v0.01-testtag-lw")
				createUnannotatedTag(t, repo, commits[len(commits)-1], "v0.02-testtag-lw-1")
				createUnannotatedTag(t, repo, commits[len(commits)-1], "v0.02-testtag-lw-2")
				commitTagsMap := generateTagsMap(repo)
				headRef, err := repo.Head()
				h.AssertNil(t, err)
				output := parseGitRefs(repo, headRef, commitTagsMap)
				expectedOutput := []string{"master", "v0.02-testtag-lw-1", "v0.02-testtag-lw-2"}
				h.AssertEq(t, output, expectedOutput)
			})

			it("returns correct tag names if all tags are annotated", func() {
				createAnnotatedTag(t, repo, commits[len(commits)-2], "v0.01-testtag")
				createAnnotatedTag(t, repo, commits[len(commits)-1], "v0.02-testtag")
				createAnnotatedTag(t, repo, commits[len(commits)-1], "v0.03-testtag")
				commitTagsMap := generateTagsMap(repo)
				headRef, err := repo.Head()
				h.AssertNil(t, err)
				output := parseGitRefs(repo, headRef, commitTagsMap)
				expectedOutput := []string{"master", "v0.02-testtag", "v0.03-testtag"}
				sort.Strings(output)
				sort.Strings(expectedOutput)
				h.AssertEq(t, output, expectedOutput)
			})

			it("returns correct tag names for both tag types", func() {
				createUnannotatedTag(t, repo, commits[len(commits)-3], "v0.001-testtag-lw")
				createAnnotatedTag(t, repo, commits[len(commits)-2], "v0.01-testtag")
				createUnannotatedTag(t, repo, commits[len(commits)-1], "v0.02-testtag-lw-1")
				createUnannotatedTag(t, repo, commits[len(commits)-1], "v0.02-testtag-lw-2")
				createAnnotatedTag(t, repo, commits[len(commits)-1], "v0.02-testtag-1")

				commitTagsMap := generateTagsMap(repo)
				headRef, err := repo.Head()
				h.AssertNil(t, err)
				output := parseGitRefs(repo, headRef, commitTagsMap)
				expectedOutput := []string{"master", "v0.02-testtag-1", "v0.02-testtag-lw-1", "v0.02-testtag-lw-2"}
				h.AssertEq(t, output, expectedOutput)
			})

			it("returns correct tag names for both tag types when branch is not `master`", func() {
				worktree, err := repo.Worktree()
				h.AssertNil(t, err)
				checkoutOpts := &git.CheckoutOptions{
					Branch: plumbing.ReferenceName("refs/heads/test-branch"),
					Create: true,
				}
				err = worktree.Checkout(checkoutOpts)
				h.AssertNil(t, err)

				createUnannotatedTag(t, repo, commits[len(commits)-3], "v0.001-testtag-lw")
				createAnnotatedTag(t, repo, commits[len(commits)-2], "v0.01-testtag")
				createUnannotatedTag(t, repo, commits[len(commits)-1], "v0.02-testtag-lw-1")
				createUnannotatedTag(t, repo, commits[len(commits)-1], "v0.02-testtag-lw-2")
				createAnnotatedTag(t, repo, commits[len(commits)-1], "v0.02-testtag-1")
				createAnnotatedTag(t, repo, commits[len(commits)-1], "v0.02-testtag-2")

				commitTagsMap := generateTagsMap(repo)
				headRef, err := repo.Head()
				h.AssertNil(t, err)
				output := parseGitRefs(repo, headRef, commitTagsMap)
				expectedOutput := []string{"test-branch", "v0.02-testtag-1", "v0.02-testtag-2", "v0.02-testtag-lw-1", "v0.02-testtag-lw-2"}
				sort.Strings(output)
				sort.Strings(expectedOutput)
				h.AssertEq(t, output, expectedOutput)
			})
		})
	})

	when("#parseGitRemote", func() {
		it("returns fetch url if remote `origin` exists", func() {
			remoteOpts := &config.RemoteConfig{
				Name: "origin",
				URLs: []string{"git@github.com:testorg/testproj.git", "git@github.com:testorg/testproj.git"},
			}
			repo.CreateRemote(remoteOpts)

			output := parseGitRemote(repo)
			h.AssertEq(t, output, "git@github.com:testorg/testproj.git")
		})

		it("returns first remote's fetch url if remote `origin` does not exists", func() {
			remoteOpts1 := &config.RemoteConfig{
				Name: "not-origin",
				URLs: []string{"git@gitlab.com:testorg/testproj.git", "git@gitlab.com:testorg/testproj.git"},
			}
			repo.CreateRemote(remoteOpts1)
			remoteOpts2 := &config.RemoteConfig{
				Name: "not-at-all-origin",
				URLs: []string{"git@github.com:testorg/testproj.git", "git@github.com:testorg/testproj.git"},
			}
			repo.CreateRemote(remoteOpts2)

			output := parseGitRemote(repo)
			h.AssertEq(t, output, "git@gitlab.com:testorg/testproj.git")
		})

		it("returns empty string if no remote exists", func() {
			output := parseGitRemote(repo)
			h.AssertEq(t, output, "")
		})

		it("returns fetch url if fetch and push URLs are different", func() {
			remoteOpts := &config.RemoteConfig{
				Name: "origin",
				URLs: []string{"git@fetch.com:testorg/testproj.git", "git@pushing-p-github.com:testorg/testproj.git"},
			}
			repo.CreateRemote(remoteOpts)

			output := parseGitRemote(repo)
			h.AssertEq(t, output, "git@fetch.com:testorg/testproj.git")
		})
	})
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

func createUnannotatedTag(t *testing.T, repo *git.Repository, commitHash plumbing.Hash, tagName string) {
	if tagName == "" {
		version := rand.Float32()*10 + float32(rand.Intn(20))
		tagName = fmt.Sprintf("v%f-lw", version)
	}
	_, err := repo.CreateTag(tagName, commitHash, nil)
	h.AssertNil(t, err)
}

func createAnnotatedTag(t *testing.T, repo *git.Repository, commitHash plumbing.Hash, tagName string) {
	if tagName == "" {
		version := rand.Float32()*10 + float32(rand.Intn(20))
		tagName = fmt.Sprintf("v%f-%s", version, h.RandString(5))
	}
	tagMessage := fmt.Sprintf("This is an annotated tag for version - %s", tagName)
	tagOpts := &git.CreateTagOptions{
		Message: tagMessage,
		Tagger: &object.Signature{
			Name:  "Test Tagger",
			Email: "testtagger@test.com",
			When:  time.Now().Add(time.Hour*time.Duration(rand.Intn(100)) + time.Minute*time.Duration(rand.Intn(100))),
		},
	}
	_, err := repo.CreateTag(tagName, commitHash, tagOpts)
	h.AssertNil(t, err)
}
