package guac

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/howeyc/fsnotify"
)

// Watch blocks until the the Watcher's context is canceled or its Done channel
// closed.  It executes function fn when changes in srcDir are detected.
func (w *Watcher) Run() {
	filepath.Walk(w.srcDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			err = w.Watch(path)
			if err != nil {
				return err
			}
			log.Printf("Watching for file changes in %s\n", path)
		}
		return nil
	})

	defer w.Close()
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-w.Event:
			if err := w.fn(); err != nil {
				log.Println("error:", err)
			}
		case err := <-w.Error:
			log.Println("error:", err)
		}
	}
}

// Watcher watches.
type Watcher struct {
	ctx    context.Context
	srcDir string
	fn     func() error

	*fsnotify.Watcher
}

// NewWatcher creates a new watcher.
func NewWatcher(ctx context.Context, srcDir string, fn func() error) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		ctx:     ctx,
		srcDir:  srcDir,
		fn:      fn,
		Watcher: watcher,
	}, nil
}
