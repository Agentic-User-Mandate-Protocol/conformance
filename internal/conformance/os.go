package conformance

import "os"

func statPathIsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
