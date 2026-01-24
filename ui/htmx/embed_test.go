package htmx

import (
	"fmt"
	"io/fs"
	"testing"
)

func TestEmbedFS(t *testing.T) {
	fmt.Println("Checking embedded files:")
	fs.WalkDir(embedFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			fmt.Printf("  - %s\n", path)
		}
		return nil
	})
}
