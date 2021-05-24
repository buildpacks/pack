package image_test

import (
	"context"
	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/image"
	"github.com/buildpacks/pack/testhelpers"
	"github.com/buildpacks/pack/testmocks"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"testing"
)

func TestProxiedFetcher(t *testing.T) {
	spec.Run(t, "Proxied Fetcher", testProxiedFetcher, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testProxiedFetcher(t *testing.T,when spec.G, it spec.S) {
	var (
		mockController *gomock.Controller
		mockFetcher *testmocks.MockImageFetcher
		mockImage *testmocks.MockImage
		assert = testhelpers.NewAssertionManager(t)
	)
	it.Before(func() {
		mockController = gomock.NewController(t)
		mockImage = testmocks.NewImage("test-image", "", nil)
		mockFetcher = testmocks.NewMockImageFetcher(mockController)
	})
	when("fetching an image", func () {
		it.Before(func() {
			mockFetcher.EXPECT().Fetch(gomock.Any(), "host:8080/proxy_path/org/repo:latest", true, pubcfg.PullIfNotPresent).Return(mockImage, nil)
		})
		it("replaces host with proxied hostname", func() {
			subject := image.NewProxiedFetcher("host:8080/proxy_path/", mockFetcher)
			fetchedImage, err := subject.Fetch(context.Background(), "index.docker.io/org/repo", true, pubcfg.PullIfNotPresent)
			assert.Nil(err)

			assert.Equal(fetchedImage.Name(), mockImage.Name())
		})

		it("replaces default registry name", func() {
			subject := image.NewProxiedFetcher("host:8080/proxy_path", mockFetcher)
			fetchedImage, err := subject.Fetch(context.Background(), "org/repo", true, pubcfg.PullIfNotPresent)
			assert.Nil(err)

			assert.Equal(fetchedImage.Name(), mockImage.Name())
		})

		it("is idempotent on replaced hostnames", func() {
			subject := image.NewProxiedFetcher("host:8080/proxy_path", mockFetcher)
			fetchedImage, err := subject.Fetch(context.Background(), "host:8080/proxy_path/org/repo:latest", true, pubcfg.PullIfNotPresent)
			assert.Nil(err)

			assert.Equal(fetchedImage.Name(), mockImage.Name())
		})
	})


	when("error cases", func() {
		when("passed an invalid image name to fetch", func() {
			it("errors with a helpful error message", func() {
				subject := image.NewProxiedFetcher("host:8080/proxy_path", mockFetcher)
				_, err := subject.Fetch(context.Background(), "::::", true, pubcfg.PullIfNotPresent)

				assert.ErrorContains(err, "image name is invalid")
			})
		})
		when("proxy host is semantically invalid", func() {
			it("errors with a helpful error message", func() {
				subject := image.NewProxiedFetcher("%%%", mockFetcher)
				_, err := subject.Fetch(context.Background(), "index.docker.io/org/repo", true, pubcfg.PullIfNotPresent)
				assert.ErrorContains(err, "proxied image name is invalid")
			})
		})
	})
}