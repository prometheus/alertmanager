package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/stuartnelson3/guac"
)

func main() {
	var (
		port  = flag.String("port", "8080", "port to listen on")
		dev   = flag.Bool("dev", true, "enable code rebuilding")
		debug = flag.Bool("debug", false, "enable elm debugger")
	)
	flag.Parse()

	http.HandleFunc("/script.js", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "script.js")
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	http.HandleFunc("/api/v1/silences", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "silences.json")
	})

	if *dev {
		// Recompile the elm code whenever a change is detected.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		const elmMake = "elm-make"
		elmMakeArgs := []string{"src/Main.elm", "--output", "script.js"}

		if *debug {
			elmMakeArgs = append(elmMakeArgs, "--debug")
		}

		recompileFn := func() error {
			cmd := exec.Command(elmMake, elmMakeArgs...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}

		_, err := guac.NewWatcher(ctx, "./src", 50*time.Millisecond, recompileFn)
		if err != nil {
			log.Fatalf("error watching: %v", err)
		}
	}

	log.Printf("starting listener on port %s", *port)
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		log.Fatal(err)
	}
}
