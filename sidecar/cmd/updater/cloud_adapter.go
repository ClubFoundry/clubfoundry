package main

import (
	"context"

	"github.com/clubfoundry/updater/internal/cloud"
	"github.com/clubfoundry/updater/internal/updater"
)

// cloudVersionAdapter keeps the updater package independent of cloud while
// exposing the release metadata required for same-version reinstallation.
type cloudVersionAdapter struct {
	cli *cloud.Client
}

// FetchVersionMetadata maps cloud release metadata to the updater model.
func (a cloudVersionAdapter) FetchVersionMetadata(ctx context.Context, version string) (*updater.VersionMetadata, error) {
	if a.cli == nil {
		return nil, nil
	}
	meta, err := a.cli.FetchVersionMetadata(ctx, version)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}
	return &updater.VersionMetadata{
		Version:        meta.Version,
		Recalled:       meta.Recalled,
		DownloadUrl:    meta.DownloadUrl,
		DownloadSha256: meta.DownloadSha256,
		DownloadUrls:   append([]string(nil), meta.DownloadUrls...),
	}, nil
}
