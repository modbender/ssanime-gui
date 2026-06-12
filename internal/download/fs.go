package download

import "os"

// removeAll deletes a path and everything under it. A thin wrapper so the
// delete-data call site reads intentionally and is easy to stub in tests.
func removeAll(path string) error {
	return os.RemoveAll(path)
}
