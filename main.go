package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/stuartnelson3/guac"
)

func main() {
	var (
		port = flag.String("port", "8080", "port to listen on")
	)
	flag.Parse()

	http.HandleFunc("/script.js", func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open("dist/script.js")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()

		fs, err := f.Stat()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		http.ServeContent(w, r, "script.js", fs.ModTime(), f)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open("index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()

		fs, err := f.Stat()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		http.ServeContent(w, r, "index", fs.ModTime(), f)
	})

	// Recompile the elm code whenever a change is detected.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recompileFn := func() error {
		cmd := exec.Command("elm-make", "src/Main.elm", "--output", "dist/script.js")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	watcher, err := guac.NewWatcher(ctx, "./src", recompileFn)
	if err != nil {
		log.Fatalf("error watching: %v", err)
	}
	go watcher.Run()

	log.Printf("starting listener on port %s", *port)
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		log.Fatal(err)
	}
}
