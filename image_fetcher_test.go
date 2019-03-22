package pack_test

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/fatih/color"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestImageFetcher(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "ImageFetcher", testImageFetcher, spec.Report(report.Terminal{}))
}

func testImageFetcher(t *testing.T, when spec.G, it spec.S) {
	var (
		fetcher          pack.ImageFetcher
		mockController   *gomock.Controller
		mockImageFactory *mocks.MockImageFactory
		mockRemoteImage  *mocks.MockImage
		mockLocalImage   *mocks.MockImage
		mockDocker       *mocks.MockDocker
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockImageFactory = mocks.NewMockImageFactory(mockController)
		mockRemoteImage = mocks.NewMockImage(mockController)
		mockLocalImage = mocks.NewMockImage(mockController)
		mockDocker = mocks.NewMockDocker(mockController)
		fetcher = pack.ImageFetcher{
			Docker:  mockDocker,
			Factory: mockImageFactory,
		}
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#FetchUpdatedLocalImage", func() {
		when("remote image exists", func() {
			it.Before(func() {
				mockRemoteImage.EXPECT().Found().Return(true, nil)
				mockImageFactory.EXPECT().NewRemote("some/image").Return(mockRemoteImage, nil)
				mockImageFactory.EXPECT().NewLocal("some/image").Return(mockLocalImage, nil)
			})

			it("pulls remote image", func() {
				mockDocker.EXPECT().PullImage(gomock.Any(), "some/image", gomock.Any())
				img, err := fetcher.FetchUpdatedLocalImage(context.TODO(), "some/image", ioutil.Discard)
				h.AssertNil(t, err)
				h.AssertSameInstance(t, img, mockLocalImage)
			})
		})

		when("remote image does not exist", func() {
			it.Before(func() {
				mockRemoteImage.EXPECT().Found().Return(false, nil)
				mockImageFactory.EXPECT().NewRemote("some/image").Return(mockRemoteImage, nil)
				mockImageFactory.EXPECT().NewLocal("some/image").Return(mockLocalImage, nil)
			})

			it("skips pulling image", func() {
				mockDocker.EXPECT().PullImage(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				img, err := fetcher.FetchUpdatedLocalImage(context.TODO(), "some/image", ioutil.Discard)
				h.AssertNil(t, err)
				h.AssertSameInstance(t, img, mockLocalImage)
			})
		})
	})

	when("#FetchLocalImage", func() {
		it.Before(func() {
			mockImageFactory.EXPECT().NewLocal("some/image").Return(mockLocalImage, nil)
		})

		it("returns local image", func() {
			img, err := fetcher.FetchLocalImage("some/image")
			h.AssertNil(t, err)
			h.AssertSameInstance(t, img, mockLocalImage)
		})
	})

	when("#FetchRemoteImage", func() {
		it.Before(func() {
			mockImageFactory.EXPECT().NewRemote("some/image").Return(mockRemoteImage, nil)
		})

		it("returns remote image", func() {
			img, err := fetcher.FetchRemoteImage("some/image")
			h.AssertNil(t, err)
			h.AssertSameInstance(t, img, mockRemoteImage)
		})
	})
}
