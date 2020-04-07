package dist_test

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestBuildpack(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "buildpack", testBuildpack, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuildpack(t *testing.T, when spec.G, it spec.S) {
	var writeBlobToFile = func(bp dist.Buildpack) string {
		t.Helper()

		bpReader, err := bp.Open()
		h.AssertNil(t, err)

		tmpDir, err := ioutil.TempDir("", "")
		h.AssertNil(t, err)

		p := filepath.Join(tmpDir, "bp.tar")
		bpWriter, err := os.Create(p)
		h.AssertNil(t, err)

		_, err = io.Copy(bpWriter, bpReader)
		h.AssertNil(t, err)

		err = bpReader.Close()
		h.AssertNil(t, err)

		return p
	}

	when("#BuildpackFromRootBlob", func() {
		it("parses the descriptor file", func() {
			bp, err := dist.BuildpackFromRootBlob(&readerBlob{
				openFn: func() io.ReadCloser {
					tarBuilder := archive.TarBuilder{}
					tarBuilder.AddFile("buildpack.toml", 0700, time.Now(), []byte(`
api = "0.3"

[buildpack]
id = "bp.one"
version = "1.2.3"
homepage = "http://geocities.com/cool-bp"

[[stacks]]
id = "some.stack.id"
`))
					return tarBuilder.Reader()
				},
			})
			h.AssertNil(t, err)

			h.AssertEq(t, bp.Descriptor().API.String(), "0.3")
			h.AssertEq(t, bp.Descriptor().Info.ID, "bp.one")
			h.AssertEq(t, bp.Descriptor().Info.Version, "1.2.3")
			h.AssertEq(t, bp.Descriptor().Info.Homepage, "http://geocities.com/cool-bp")
			h.AssertEq(t, bp.Descriptor().Stacks[0].ID, "some.stack.id")
		})

		it("translates blob to distribution format", func() {
			bp, err := dist.BuildpackFromRootBlob(&readerBlob{
				openFn: func() io.ReadCloser {
					tarBuilder := archive.TarBuilder{}
					tarBuilder.AddFile("buildpack.toml", 0700, time.Now(), []byte(`
api = "0.3"

[buildpack]
id = "bp.one"
version = "1.2.3"

[[stacks]]
id = "some.stack.id"
`))

					tarBuilder.AddDir("bin", 0700, time.Now())
					tarBuilder.AddFile("bin/detect", 0700, time.Now(), []byte("detect-contents"))
					tarBuilder.AddFile("bin/build", 0700, time.Now(), []byte("build-contents"))
					return tarBuilder.Reader()
				},
			})
			h.AssertNil(t, err)

			tarPath := writeBlobToFile(bp)
			defer os.Remove(tarPath)

			h.AssertOnTarEntry(t, tarPath,
				"/cnb/buildpacks/bp.one",
				h.IsDirectory(),
				h.HasFileMode(0755),
				h.HasModTime(archive.NormalizedDateTime),
			)

			h.AssertOnTarEntry(t, tarPath,
				"/cnb/buildpacks/bp.one/1.2.3",
				h.IsDirectory(),
				h.HasFileMode(0755),
				h.HasModTime(archive.NormalizedDateTime),
			)

			h.AssertOnTarEntry(t, tarPath,
				"/cnb/buildpacks/bp.one/1.2.3/bin",
				h.IsDirectory(),
				h.HasFileMode(0755),
				h.HasModTime(archive.NormalizedDateTime),
			)

			h.AssertOnTarEntry(t, tarPath,
				"/cnb/buildpacks/bp.one/1.2.3/bin/detect",
				h.HasFileMode(0755),
				h.HasModTime(archive.NormalizedDateTime),
				h.ContentEquals("detect-contents"),
			)

			h.AssertOnTarEntry(t, tarPath,
				"/cnb/buildpacks/bp.one/1.2.3/bin/build",
				h.HasFileMode(0755),
				h.HasModTime(archive.NormalizedDateTime),
				h.ContentEquals("build-contents"),
			)
		})

		it("surfaces errors encountered while reading blob", func() {
			realBlob := &readerBlob{
				openFn: func() io.ReadCloser {
					tarBuilder := archive.TarBuilder{}
					tarBuilder.AddFile("buildpack.toml", 0700, time.Now(), []byte(`
api = "0.3"

[buildpack]
id = "bp.one"
version = "1.2.3"

[[stacks]]
id = "some.stack.id"
`))
					return tarBuilder.Reader()
				},
			}

			bp, err := dist.BuildpackFromRootBlob(&errorBlob{
				realBlob: realBlob,
			})
			h.AssertNil(t, err)

			bpReader, err := bp.Open()
			h.AssertNil(t, err)

			_, err = io.Copy(ioutil.Discard, bpReader)
			h.AssertError(t, err, "error from errBlob")
		})

		when("calculating permissions", func() {
			bpTOMLData := `
api = "0.3"

[buildpack]
id = "bp.one"
version = "1.2.3"

[[stacks]]
id = "some.stack.id"
`

			when("no exec bits set", func() {
				it("sets to 0755 if directory", func() {
					bp, err := dist.BuildpackFromRootBlob(&readerBlob{
						openFn: func() io.ReadCloser {
							tarBuilder := archive.TarBuilder{}
							tarBuilder.AddFile("buildpack.toml", 0700, time.Now(), []byte(bpTOMLData))
							tarBuilder.AddDir("some-dir", 0600, time.Now())
							return tarBuilder.Reader()
						},
					})
					h.AssertNil(t, err)

					tarPath := writeBlobToFile(bp)
					defer os.Remove(tarPath)

					h.AssertOnTarEntry(t, tarPath,
						"/cnb/buildpacks/bp.one/1.2.3/some-dir",
						h.HasFileMode(0755),
					)
				})
			})

			when("no exec bits set", func() {
				it("sets to 0755 if 'bin/detect' or 'bin/build'", func() {
					bp, err := dist.BuildpackFromRootBlob(&readerBlob{
						openFn: func() io.ReadCloser {
							tarBuilder := archive.TarBuilder{}
							tarBuilder.AddFile("buildpack.toml", 0700, time.Now(), []byte(bpTOMLData))
							tarBuilder.AddFile("bin/detect", 0600, time.Now(), []byte("detect-contents"))
							tarBuilder.AddFile("bin/build", 0600, time.Now(), []byte("build-contents"))
							return tarBuilder.Reader()
						},
					})
					h.AssertNil(t, err)

					tarPath := writeBlobToFile(bp)
					defer os.Remove(tarPath)

					h.AssertOnTarEntry(t, tarPath,
						"/cnb/buildpacks/bp.one/1.2.3/bin/detect",
						h.HasFileMode(0755),
					)

					h.AssertOnTarEntry(t, tarPath,
						"/cnb/buildpacks/bp.one/1.2.3/bin/build",
						h.HasFileMode(0755),
					)
				})
			})

			when("not directory, 'bin/detect', or 'bin/build'", func() {
				it("sets to 0755 if ANY exec bit is set", func() {
					bp, err := dist.BuildpackFromRootBlob(&readerBlob{
						openFn: func() io.ReadCloser {
							tarBuilder := archive.TarBuilder{}
							tarBuilder.AddFile("buildpack.toml", 0700, time.Now(), []byte(bpTOMLData))
							tarBuilder.AddFile("some-file", 0700, time.Now(), []byte("some-data"))
							return tarBuilder.Reader()
						},
					})
					h.AssertNil(t, err)

					tarPath := writeBlobToFile(bp)
					defer os.Remove(tarPath)

					h.AssertOnTarEntry(t, tarPath,
						"/cnb/buildpacks/bp.one/1.2.3/some-file",
						h.HasFileMode(0755),
					)
				})
			})

			when("not directory, 'bin/detect', or 'bin/build'", func() {
				it("sets to 0644 if NO exec bits set", func() {
					bp, err := dist.BuildpackFromRootBlob(&readerBlob{
						openFn: func() io.ReadCloser {
							tarBuilder := archive.TarBuilder{}
							tarBuilder.AddFile("buildpack.toml", 0700, time.Now(), []byte(bpTOMLData))
							tarBuilder.AddFile("some-file", 0600, time.Now(), []byte("some-data"))
							return tarBuilder.Reader()
						},
					})
					h.AssertNil(t, err)

					tarPath := writeBlobToFile(bp)
					defer os.Remove(tarPath)

					h.AssertOnTarEntry(t, tarPath,
						"/cnb/buildpacks/bp.one/1.2.3/some-file",
						h.HasFileMode(0644),
					)
				})
			})
		})

		when("there is no descriptor file", func() {
			it("returns error", func() {
				_, err := dist.BuildpackFromRootBlob(&readerBlob{
					openFn: func() io.ReadCloser {
						tarBuilder := archive.TarBuilder{}
						return tarBuilder.Reader()
					},
				})
				h.AssertError(t, err, "could not find entry path 'buildpack.toml'")
			})
		})

		when("there is no api field", func() {
			it("assumes an api version", func() {
				bp, err := dist.BuildpackFromRootBlob(&readerBlob{
					openFn: func() io.ReadCloser {
						tarBuilder := archive.TarBuilder{}
						tarBuilder.AddFile("buildpack.toml", 0700, time.Now(), []byte(`
[buildpack]
id = "bp.one"
version = "1.2.3"

[[stacks]]
id = "some.stack.id"`))
						return tarBuilder.Reader()
					},
				})
				h.AssertNil(t, err)
				h.AssertEq(t, bp.Descriptor().API.String(), "0.1")
			})
		})

		when("there is no id", func() {
			it("returns error", func() {
				_, err := dist.BuildpackFromRootBlob(&readerBlob{
					openFn: func() io.ReadCloser {
						tarBuilder := archive.TarBuilder{}
						tarBuilder.AddFile("buildpack.toml", 0700, time.Now(), []byte(`
[buildpack]
id = ""
version = "1.2.3"

[[stacks]]
id = "some.stack.id"`))
						return tarBuilder.Reader()
					},
				})
				h.AssertError(t, err, "'buildpack.id' is required")
			})
		})

		when("there is no version", func() {
			it("returns error", func() {
				_, err := dist.BuildpackFromRootBlob(&readerBlob{
					openFn: func() io.ReadCloser {
						tarBuilder := archive.TarBuilder{}
						tarBuilder.AddFile("buildpack.toml", 0700, time.Now(), []byte(`
[buildpack]
id = "bp.one"
version = ""

[[stacks]]
id = "some.stack.id"`))
						return tarBuilder.Reader()
					},
				})
				h.AssertError(t, err, "'buildpack.version' is required")
			})
		})

		when("both stacks and order are present", func() {
			it("returns error", func() {
				_, err := dist.BuildpackFromRootBlob(&readerBlob{
					openFn: func() io.ReadCloser {
						tarBuilder := archive.TarBuilder{}
						tarBuilder.AddFile("buildpack.toml", 0700, time.Now(), []byte(`
[buildpack]
id = "bp.one"
version = "1.2.3"

[[stacks]]
id = "some.stack.id"

[[order]]
[[order.group]]
  id = "bp.nested"
  version = "bp.nested.version"
`))
						return tarBuilder.Reader()
					},
				})
				h.AssertError(t, err, "cannot have both 'stacks' and an 'order' defined")
			})
		})

		when("missing stacks and order", func() {
			it("returns error", func() {
				_, err := dist.BuildpackFromRootBlob(&readerBlob{
					openFn: func() io.ReadCloser {
						tarBuilder := archive.TarBuilder{}
						tarBuilder.AddFile("buildpack.toml", 0700, time.Now(), []byte(`
[buildpack]
id = "bp.one"
version = "1.2.3"
`))
						return tarBuilder.Reader()
					},
				})
				h.AssertError(t, err, "must have either 'stacks' or an 'order' defined")
			})
		})
	})

	when("#Match", func() {
		it("compares, using only the id and version", func() {
			other := dist.BuildpackInfo{
				ID:       "same",
				Version:  "1.2.3",
				Homepage: "something else",
			}

			self := dist.BuildpackInfo{
				ID:      "same",
				Version: "1.2.3",
			}

			match := self.Match(other)

			h.AssertEq(t, match, true)

			self.ID = "different"
			match = self.Match(other)

			h.AssertEq(t, match, false)
		})
	})
}

type errorBlob struct {
	notFirst bool
	realBlob dist.Blob
}

func (e *errorBlob) Open() (io.ReadCloser, error) {
	if !e.notFirst {
		e.notFirst = true
		return e.realBlob.Open()
	}
	return nil, errors.New("error from errBlob")
}

type readerBlob struct {
	openFn func() io.ReadCloser
}

func (r *readerBlob) Open() (io.ReadCloser, error) {
	return r.openFn(), nil
}
