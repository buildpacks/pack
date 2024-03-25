package fakes

import (
	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/pkg/dist"
)

type FakePackageConfigReader struct {
	ReadCalledWithArg string
	ReadReturnConfig  pubbldpkg.Config
	ReadReturnError   error

	ReadBuildpackDescriptorCalledWithArg string
	ReadBuildpackDescriptorReturn        dist.BuildpackDescriptor
	ReadBuildpackDescriptorReturnError   error

	ReadExtensionDescriptorCalledWithArg string
	ReadExtensionDescriptorReturn        dist.ExtensionDescriptor
	ReadExtensionDescriptorReturnError   error
}

// ReadExtensionDescriptor implements commands.PackageConfigReader.
func (r *FakePackageConfigReader) ReadExtensionDescriptor(path string) (dist.ExtensionDescriptor, error) {
	r.ReadExtensionDescriptorCalledWithArg = path

	return r.ReadExtensionDescriptorReturn, r.ReadExtensionDescriptorReturnError
}

func (r *FakePackageConfigReader) Read(path string) (pubbldpkg.Config, error) {
	r.ReadCalledWithArg = path

	return r.ReadReturnConfig, r.ReadReturnError
}

func (r *FakePackageConfigReader) ReadBuildpackDescriptor(path string) (dist.BuildpackDescriptor, error) {
	r.ReadBuildpackDescriptorCalledWithArg = path

	return r.ReadBuildpackDescriptorReturn, r.ReadBuildpackDescriptorReturnError
}

func NewFakePackageConfigReader(ops ...func(*FakePackageConfigReader)) *FakePackageConfigReader {
	fakePackageConfigReader := &FakePackageConfigReader{
		ReadReturnConfig:                   pubbldpkg.Config{},
		ReadBuildpackDescriptorReturn:      dist.BuildpackDescriptor{},
		ReadReturnError:                    nil,
		ReadBuildpackDescriptorReturnError: nil,
		ReadExtensionDescriptorReturn:      dist.ExtensionDescriptor{},
		ReadExtensionDescriptorReturnError: nil,
	}

	for _, op := range ops {
		op(fakePackageConfigReader)
	}

	return fakePackageConfigReader
}
