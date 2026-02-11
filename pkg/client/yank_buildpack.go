package client

import (
	"net/url"
	"runtime"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/registry"
)

// YankBuildpackOptions is a configuration struct that controls the Yanking a buildpack
// from the Buildpack Registry.
type YankBuildpackOptions struct {
	ID      string
	Version string
	Type    string
	URL     string
	Yank    bool
}

// YankBuildpack marks a buildpack on the Buildpack Registry as 'yanked'. This forbids future
// builds from using it.
func (c *Client) YankBuildpack(opts YankBuildpackOptions) error {
	namespace, name, err := registry.ParseNamespaceName(opts.ID)
	if err != nil {
		return err
	}
	issueURL, err := registry.GetIssueURL(opts.URL)
	if err != nil {
		return err
	}

	buildpack := registry.Buildpack{
		Namespace: namespace,
		Name:      name,
		Version:   opts.Version,
		Yanked:    opts.Yank,
	}

	// Try to get registry cache root to load template from registry-index
	var registryRoot string
	home, err := config.PackHome()
	if err == nil {
		registryCache, err := registry.NewRegistryCache(c.logger, home, opts.URL)
		if err == nil {
			// Initialize the cache if needed (this will clone/refresh the registry)
			if err := registryCache.Initialize(); err == nil {
				// Refresh to get latest template
				_ = registryCache.Refresh()
				registryRoot = registryCache.Root
			}
		} else {
			c.logger.Warnf("Error initializing registry cache: %s", err)
		}
	}

	issue, err := registry.CreateGithubIssue(buildpack, registryRoot)
	if err != nil {
		return err
	}

	params := url.Values{}
	params.Add("title", issue.Title)
	params.Add("body", issue.Body)
	// Add template parameter when we have a registry root (GitHub will use the template if it exists)
	if registryRoot != "" {
		if opts.Yank {
			params.Add("template", "yank-buildpack.md")
		} else {
			params.Add("template", "add-buildpack.md")
		}
	}
	issueURL.RawQuery = params.Encode()

	c.logger.Debugf("Open URL in browser: %s", issueURL)
	cmd, err := registry.CreateBrowserCmd(issueURL.String(), runtime.GOOS)
	if err != nil {
		return err
	}

	return cmd.Start()
}
