package guac

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/howeyc/fsnotify"
)

// Watch blocks until the the Watcher's context is canceled or its Done channel
// closed.  It executes function fn when changes in srcDir are detected.
func (w *Watcher) run() {
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
			w.debounce.Stop()
			w.debounce = time.AfterFunc(w.debounceTime, func() { w.fn() })
		case err := <-w.Error:
			log.Println("error:", err)
		}
	}
}

// Watcher watches.
type Watcher struct {
	ctx          context.Context
	srcDir       string
	fn           func() error
	debounceTime time.Duration
	debounce     *time.Timer

	*fsnotify.Watcher
}

// NewWatcher creates a new watcher.
func NewWatcher(ctx context.Context, srcDir string, debounceTime time.Duration, fn func() error) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		ctx:          ctx,
		srcDir:       srcDir,
		fn:           fn,
		debounceTime: debounceTime,
		Watcher:      watcher,
	}

	w.debounce = time.AfterFunc(w.debounceTime, func() { w.fn() })

	go w.run()

	return w, nil
}
