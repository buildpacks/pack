package acceptance

import (
	"archive/tar"
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver"

	"github.com/google/go-github/v30/github"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
)

const (
	assetCacheDir      = "test-assets-cache"
	assetCacheManifest = "github.json"
)

type GithubAssetFetcher struct {
	ctx          context.Context
	logger       *ilogging.LogWithWriters
	githubClient *github.Client
	cacheDir     string
}

type assetCache map[string]map[string]cachedRepo
type cachedRepo struct {
	Assets   cachedAssets
	Sources  cachedSources
	Versions cachedVersions
}
type cachedAssets map[string][]string
type cachedSources map[string]string
type cachedVersions map[string]string

func NewGithubAssetFetcher() (*GithubAssetFetcher, error) {
	packHome, err := config.PackHome()
	if err != nil {
		return nil, errors.Wrap(err, "getting pack home")
	}
	cacheDir := filepath.Join(packHome, assetCacheDir)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, errors.Wrapf(err, "creating directory %s", cacheDir)
	}

	return &GithubAssetFetcher{
		ctx:          context.TODO(),
		logger:       ilogging.NewLogWithWriters(os.Stdout, os.Stderr), // TODO: not sure if this is what we really want
		githubClient: github.NewClient(nil),
		cacheDir:     cacheDir,
	}, nil
}

func (f *GithubAssetFetcher) FetchReleaseAsset(owner, repo, version string, expr *regexp.Regexp, extract bool) (string, error) {
	if destPath, _ := f.cachedAsset(owner, repo, version, expr); destPath != "" {
		fmt.Printf("found %s in cache for %s/%s %s\n", destPath, owner, repo, version)
		return destPath, nil
	}

	release, _, err := f.githubClient.Repositories.GetReleaseByTag(f.ctx, owner, repo, version)
	if err != nil {
		return "", errors.Wrap(err, "getting release")
	}

	var desiredAsset *github.ReleaseAsset
	for _, asset := range release.Assets {
		if expr.MatchString(*asset.Name) {
			desiredAsset = asset
			break
		}
	}
	if desiredAsset == nil {
		return "", fmt.Errorf("could not find asset matching expression %s", expr.String())
	}

	var returnPath string
	extractType := extractType(extract, *desiredAsset.Name)
	switch extractType {
	case "tgz":
		targetDir := filepath.Join(f.cacheDir, stripExtension(*desiredAsset.Name))
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return "", errors.Wrapf(err, "creating directory %s", targetDir)
		}

		if err := f.downloadAndExtractTgz(*desiredAsset.BrowserDownloadURL, targetDir); err != nil {
			return "", err
		}

		returnPath = targetDir
	case "zip":
		targetPath := filepath.Join(f.cacheDir, *desiredAsset.Name)
		if err := f.downloadAndExtractZip(*desiredAsset.BrowserDownloadURL, targetPath); err != nil {
			return "", err
		}

		returnPath = stripExtension(targetPath)
	default:
		targetPath := filepath.Join(f.cacheDir, *desiredAsset.Name)
		if err := f.downloadAndSave(*desiredAsset.BrowserDownloadURL, targetPath); err != nil {
			return "", err
		}

		returnPath = targetPath
	}

	err = f.writeCacheManifest(owner, repo, func(cache assetCache) {
		existingAssets, found := cache[owner][repo].Assets[version]
		if found {
			cache[owner][repo].Assets[version] = append(existingAssets, returnPath)
		}
		cache[owner][repo].Assets[version] = []string{returnPath}
	})
	if err != nil {
		f.logger.Warn(errors.Wrap(err, "writing cache").Error())
	}
	return returnPath, nil
}

func extractType(extract bool, assetName string) string {
	if extract && strings.Contains(assetName, ".tgz") {
		return "tgz"
	}
	if extract && strings.Contains(assetName, ".zip") {
		return "zip"
	}
	return "none"
}

func (f *GithubAssetFetcher) FetchReleaseSource(owner, repo, version string) (string, error) {
	if destDir, _ := f.cachedSource(owner, repo, version); destDir != "" {
		fmt.Printf("found %s in cache for %s/%s %s\n", destDir, owner, repo, version)
		return destDir, nil
	}

	release, _, err := f.githubClient.Repositories.GetReleaseByTag(f.ctx, owner, repo, version)
	if err != nil {
		return "", errors.Wrap(err, "getting release")
	}

	destDir := filepath.Join(f.cacheDir, strings.ReplaceAll(*release.Name, " ", "-")+"-source")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", errors.Wrapf(err, "creating directory %s", destDir)
	}

	if err := f.downloadAndExtractTgz(*release.TarballURL, destDir); err != nil {
		return "", err
	}

	err = f.writeCacheManifest(owner, repo, func(cache assetCache) {
		cache[owner][repo].Sources[version] = destDir
	})
	if err != nil {
		f.logger.Warn(errors.Wrap(err, "writing cache").Error())
	}
	return destDir, nil
}

func (f *GithubAssetFetcher) FetchReleaseVersion(owner, repo string, n int) (string, error) {
	if version, _ := f.cachedVersion(owner, repo, n); version != "" {
		fmt.Printf("found %s in cache for %s/%s %d\n", version, owner, repo, n)
		return version, nil
	}

	// get all release versions
	releases, _, err := f.githubClient.Repositories.ListReleases(f.ctx, owner, repo, nil)
	if err != nil {
		return "", errors.Wrap(err, "listing releases")
	}

	// sort all release versions
	versions := make([]*semver.Version, len(releases))
	for i, release := range releases {
		version, err := semver.NewVersion(*release.TagName)
		if err != nil {
			return "", errors.Wrap(err, "parsing semver")
		}
		versions[i] = version
	}
	sort.Sort(semver.Collection(versions))

	latestVersion := versions[len(versions)-1]

	// get latest patch of previous minor
	constraint, err := semver.NewConstraint(
		fmt.Sprintf("~%d.%d.x", latestVersion.Major(), latestVersion.Minor()+int64(n)),
	)
	if err != nil {
		return "", errors.Wrap(err, "parsing semver constraint")
	}
	var latestPatchOfPreviousMinor *semver.Version
	for i := len(versions) - 1; i >= 0; i-- {
		if constraint.Check(versions[i]) {
			latestPatchOfPreviousMinor = versions[i]
			break
		}
	}
	if latestPatchOfPreviousMinor == nil {
		return "", errors.Wrapf(err, "obtaining latest patch of previous minor")
	}
	formattedVersion := fmt.Sprintf("v%s", latestPatchOfPreviousMinor.String())

	err = f.writeCacheManifest(owner, repo, func(cache assetCache) {
		cache[owner][repo].Versions[strconv.Itoa(n)] = formattedVersion
	})
	if err != nil {
		f.logger.Warn(errors.Wrap(err, "writing cache").Error())
	}
	return formattedVersion, nil
}

func (f *GithubAssetFetcher) cachedAsset(owner, repo, version string, expr *regexp.Regexp) (string, error) {
	cache, err := f.loadCacheManifest()
	if err != nil {
		return "", errors.Wrap(err, "loading cache")
	}

	assets, found := cache[owner][repo].Assets[version]
	if found {
		for _, asset := range assets {
			if expr.MatchString(asset) {
				return asset, nil
			}
		}
	}
	return "", nil
}

func (f *GithubAssetFetcher) cachedSource(owner, repo, version string) (string, error) {
	cache, err := f.loadCacheManifest()
	if err != nil {
		return "", errors.Wrap(err, "loading cache")
	}

	value, found := cache[owner][repo].Sources[version]
	if found {
		return value, nil
	}
	return "", nil
}

func (f *GithubAssetFetcher) cachedVersion(owner, repo string, n int) (string, error) {
	cache, err := f.loadCacheManifest()
	if err != nil {
		return "", errors.Wrap(err, "loading cache")
	}

	value, found := cache[owner][repo].Versions[strconv.Itoa(n)]
	if found {
		return value, nil
	}
	return "", nil
}

func (f *GithubAssetFetcher) loadCacheManifest() (assetCache, error) {
	cacheManifest, err := os.Stat(filepath.Join(f.cacheDir, assetCacheManifest))
	if os.IsNotExist(err) {
		return assetCache{}, nil
	}

	// invalidate cache manifest that is too old
	if time.Since(cacheManifest.ModTime()) > 1*time.Hour {
		return assetCache{}, nil
	}

	content, err := ioutil.ReadFile(filepath.Join(f.cacheDir, assetCacheManifest))
	if err != nil {
		return nil, errors.Wrap(err, "reading cache manifest")
	}

	var cache assetCache
	err = json.Unmarshal(content, &cache)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshaling cache manifest content")
	}

	return cache, nil
}

func (f *GithubAssetFetcher) writeCacheManifest(owner, repo string, op func(cache assetCache)) error {
	cache, err := f.loadCacheManifest()
	if err != nil {
		return errors.Wrap(err, "loading cache")
	}

	// init keys for owner and repo
	if _, found := cache[owner]; !found {
		cache[owner] = map[string]cachedRepo{}
	}
	if _, found := cache[owner][repo]; !found {
		cache[owner][repo] = cachedRepo{
			Assets:   cachedAssets{},
			Sources:  cachedSources{},
			Versions: cachedVersions{},
		}
	}

	op(cache)

	content, err := json.Marshal(cache)
	if err != nil {
		return errors.Wrap(err, "marshaling cache manifest content")
	}
	content = append(content, "\n"...)

	return ioutil.WriteFile(filepath.Join(f.cacheDir, assetCacheManifest), content, 0644)
}

func (f *GithubAssetFetcher) downloadAndSave(assetURI, destPath string) error {
	downloader := blob.NewDownloader(f.logger, f.cacheDir)

	assetBlob, err := downloader.Download(f.ctx, assetURI)
	if err != nil {
		return errors.Wrapf(err, "downloading blob %s", assetURI)
	}

	assetReader, err := assetBlob.Open()
	if err != nil {
		return errors.Wrap(err, "opening blob")
	}
	defer assetReader.Close()

	destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return errors.Wrapf(err, "opening file %s", destPath)
	}
	defer destFile.Close()

	if _, err = io.Copy(destFile, assetReader); err != nil {
		return errors.Wrap(err, "copying data")
	}

	return nil
}

func (f *GithubAssetFetcher) downloadAndExtractTgz(assetURI, destDir string) error {
	downloader := blob.NewDownloader(f.logger, f.cacheDir)

	assetBlob, err := downloader.Download(f.ctx, assetURI)
	if err != nil {
		return errors.Wrapf(err, "downloading blob %s", assetURI)
	}

	assetReader, err := assetBlob.Open()
	if err != nil {
		return errors.Wrapf(err, "opening blob")
	}
	defer assetReader.Close()

	if err := extractTgz(assetReader, destDir); err != nil {
		return errors.Wrap(err, "extracting tgz")
	}

	return nil
}

func (f *GithubAssetFetcher) downloadAndExtractZip(assetURI, destPath string) error {
	if err := f.downloadAndSave(assetURI, destPath); err != nil {
		return err
	}

	if err := extractZip(destPath); err != nil {
		return errors.Wrap(err, "extracting zip")
	}

	return nil
}

func stripExtension(assetFilename string) string {
	return strings.TrimSuffix(assetFilename, path.Ext(assetFilename))
}

func extractTgz(reader io.Reader, destDir string) error {
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()

		switch err {
		case nil:
			// keep going
		case io.EOF:
			return nil
		default:
			return err
		}

		target := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			targetFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			if _, err := io.Copy(targetFile, tarReader); err != nil {
				return err
			}

			targetFile.Close()
		}
	}
}

func extractZip(zipPath string) error {
	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zipReader.Close()

	parentDir := filepath.Dir(zipPath)

	for _, f := range zipReader.File {
		target := filepath.Join(parentDir, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, f.Mode())
			continue
		}

		targetFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, f.Mode())
		if err != nil {
			return err
		}

		sourceFile, err := f.Open()
		if err != nil {
			return err
		}

		_, err = io.Copy(targetFile, sourceFile)
		if err != nil {
			return err
		}

		sourceFile.Close()
		targetFile.Close()
	}

	return nil
}
