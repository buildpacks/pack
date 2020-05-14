package fakes

import (
	"context"
	"github.com/buildpacks/pack/internal/build"
)

type FakeCreator struct {
	CreateCallCount int
	DetectCallCount int
	AnalyzeCallCount int
	RestoreCallCount int
	BuildCallCount int
	ExportCallCount int
}


func (l *FakeCreator) Create(ctx context.Context, publish, clearCache bool, runImage, launchCacheName, cacheName, repoName, networkMode string, phaseFactory build.PhaseFactory) error {
	l.CreateCallCount++
	return nil
}

func (l *FakeCreator) Detect(ctx context.Context, networkMode string, volumes []string, phaseFactory build.PhaseFactory) error {
	l.DetectCallCount++
	return nil
}

func (l *FakeCreator) Analyze(ctx context.Context, repoName, cacheName, networkMode string, publish, clearCache bool, phaseFactory build.PhaseFactory) error {
	l.AnalyzeCallCount++
	return nil
}

func (l *FakeCreator) Restore(ctx context.Context, cacheName, networkMode string, phaseFactory build.PhaseFactory) error {
	l.RestoreCallCount++
	return nil
}

func (l *FakeCreator) Build(ctx context.Context, networkMode string, volumes []string, phaseFactory build.PhaseFactory) error {
	l.BuildCallCount++
	return nil
}

func (l *FakeCreator) Export(ctx context.Context, repoName string, runImage string, publish bool, launchCacheName, cacheName, networkMode string, phaseFactory build.PhaseFactory) error {
	l.ExportCallCount++
	return nil
}
