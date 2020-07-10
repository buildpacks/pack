package fakes

import (
	pubbldpkg "github.com/buildpacks/pack/buildpackage"
)

type FakePackageConfigReader struct {
	ReadCalledWithArg string
	ReadReturnConfig  pubbldpkg.Config
	ReadReturnError   error
}

func (r *FakePackageConfigReader) Read(path string) (pubbldpkg.Config, error) {
	r.ReadCalledWithArg = path

	return r.ReadReturnConfig, r.ReadReturnError
}

func NewFakePackageConfigReader(ops ...func(*FakePackageConfigReader)) *FakePackageConfigReader {
	fakePackageConfigReader := &FakePackageConfigReader{
		ReadReturnConfig: pubbldpkg.Config{},
		ReadReturnError:  nil,
	}

	for _, op := range ops {
		op(fakePackageConfigReader)
	}

	return fakePackageConfigReader
}
