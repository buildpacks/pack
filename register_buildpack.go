package pack

import (
	"context"
	"errors"
	"net/url"
	"runtime"
	"strings"

	"github.com/buildpacks/pack/internal/buildpackage"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/registry"
)

type RegisterBuildpackOptions struct {
	ImageName string
	Type      string
	URL       string
}

func (c *Client) RegisterBuildpack(ctx context.Context, opts RegisterBuildpackOptions) error {
	appImage, err := c.imageFetcher.Fetch(ctx, opts.ImageName, false, true)
	if err != nil {
		return err
	}

	var buildpackInfo dist.BuildpackInfo
	if _, err := dist.GetLabel(appImage, buildpackage.MetadataLabel, &buildpackInfo); err != nil {
		return err
	}

	namespace, name, err := parseID(buildpackInfo.ID)
	if err != nil {
		return err
	}

	id, err := appImage.Identifier()
	if err != nil {
		return err
	}

	buildpack := registry.Buildpack{
		Namespace: namespace,
		Name:      name,
		Version:   buildpackInfo.Version,
		Address:   id.String(),
		Yanked:    false,
	}

	if opts.Type == "github" {
		issueURL, err := parseURL(opts.URL)
		if err != nil {
			return err
		}

		issue, err := registry.CreateGithubIssue(buildpack)
		if err != nil {
			return err
		}

		params := url.Values{}
		params.Add("title", issue.Title)
		params.Add("body", issue.Body)
		issueURL.RawQuery = params.Encode()

		c.logger.Debugf("Open URL in browser: %s", issueURL)
		cmd, err := registry.CreateBrowserCmd(issueURL.String(), runtime.GOOS)
		if err != nil {
			return err
		}

		return cmd.Start()
	} else if opts.Type == "git" {
		registryCache, err := c.getRegistry(c.logger, opts.URL)
		if err != nil {
			return err
		}

		if err := registry.CreateGitCommit(buildpack, registryCache); err != nil {
			return err
		}
	}

	return nil
}

func parseID(id string) (string, string, error) {
	parts := strings.Split(id, "/")
	if len(parts) < 2 {
		return "", "", errors.New("invalid id: does not contain a namespace")
	} else if len(parts) > 2 {
		return "", "", errors.New("invalid id: contains unexpected characters")
	}

	return parts[0], parts[1], nil
}
