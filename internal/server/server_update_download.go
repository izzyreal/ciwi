package server

import (
	"context"
	"os"

	serverupdate "github.com/izzyreal/ciwi/internal/server/update"
)

func looksLikeGoRunBinary(path string) bool {
	return serverupdate.LooksLikeGoRunBinary(path)
}

func downloadUpdateAsset(ctx context.Context, assetURL, assetName string) (string, error) {
	return serverupdate.DownloadAsset(ctx, assetURL, assetName, serverupdate.DownloadOptions{
		AuthToken: envOrDefault("CIWI_GITHUB_TOKEN", ""),
	})
}

func downloadTextAsset(ctx context.Context, assetURL string) (string, error) {
	return serverupdate.DownloadTextAsset(ctx, assetURL, serverupdate.DownloadOptions{
		AuthToken: envOrDefault("CIWI_GITHUB_TOKEN", ""),
	})
}

func verifyFileSHA256(path, assetName, checksumContent string) error {
	return serverupdate.VerifyFileSHA256(path, assetName, checksumContent)
}

func exeExt() string {
	return serverupdate.ExeExt()
}

func copyFile(src, dst string, mode os.FileMode) error {
	return serverupdate.CopyFile(src, dst, mode)
}
