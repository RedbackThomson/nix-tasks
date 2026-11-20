//go:build integration

package integration_test

import (
	"path/filepath"
	"testing"
)

func namesAndExitCodesFlake(t *testing.T) string {
	t.Helper()
	return filepath.Join(testdataDir(t), "names-and-exit-codes")
}
