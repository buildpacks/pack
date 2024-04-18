package dist

import (
	"context"

	"golang.org/x/sync/errgroup"
)

type Target struct {
	OS            string         `json:"os" toml:"os"`
	Arch          string         `json:"arch" toml:"arch"`
	ArchVariant   string         `json:"variant,omitempty" toml:"variant,omitempty"`
	Distributions []Distribution `json:"distributions,omitempty" toml:"distributions,omitempty"`
}

type Distribution struct {
	Name     string   `json:"name,omitempty" toml:"name,omitempty"`
	Versions []string `json:"versions,omitempty" toml:"versions,omitempty"`
}

func (t Target) Range(ctx context.Context, perform func(target Target) error) error {
	errs, _ := errgroup.WithContext(ctx)
	if len(t.Distributions) == 0 {
		t.Distributions = make([]Distribution, 0)
		errs.Go(func() error {
			return perform(t)
		})
	}

	for _, distro := range t.Distributions {
		target := t
		t.Distributions = []Distribution{distro}
		if len(distro.Versions) == 0 {
			errs.Go(func() error {
				return perform(target)
			})
		}

		for _, version := range distro.Versions {
			t.Distributions[0].Versions = []string{version}
			errs.Go(func() error {
				return perform(target)
			})
		}
	}

	return errs.Wait()
}
