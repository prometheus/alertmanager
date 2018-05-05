package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/api"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/alertmanager/client"
	"github.com/prometheus/alertmanager/types"
)

type silenceImportCmd struct {
	force   bool
	workers int
	file    string
}

const silenceImportHelp = `Import alertmanager silences from JSON file or stdin

This command can be used to bulk import silences from a JSON file
created by query command. For example:

amtool silence query -o json foo > foo.json

amtool silence import foo.json

JSON data can also come from stdin if no param is specified.
`

func configureSilenceImportCmd(cc *kingpin.CmdClause) {
	var (
		c         = &silenceImportCmd{}
		importCmd = cc.Command("import", silenceImportHelp)
	)

	importCmd.Flag("force", "Force adding new silences even if it already exists").Short('f').BoolVar(&c.force)
	importCmd.Flag("worker", "Number of concurrent workers to use for import").Short('w').Default("8").IntVar(&c.workers)
	importCmd.Arg("input-file", "JSON file with silences").ExistingFileVar(&c.file)
	importCmd.Action(c.bulkImport)
}

func addSilenceWorker(sclient client.SilenceAPI, silencec <-chan *types.Silence, errc chan<- error) {
	for s := range silencec {
		silenceID, err := sclient.Set(context.Background(), *s)
		sid := s.ID
		if err != nil && strings.Contains(err.Error(), "not found") {
			// silence doesn't exists yet, retry to create as a new one
			s.ID = ""
			silenceID, err = sclient.Set(context.Background(), *s)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error adding silence id='%v': %v\n", sid, err)
		} else {
			fmt.Println(silenceID)
		}
		errc <- err
	}
}

func (c *silenceImportCmd) bulkImport(ctx *kingpin.ParseContext) error {
	input := os.Stdin
	var err error
	if c.file != "" {
		input, err = os.Open(c.file)
		if err != nil {
			return err
		}
		defer input.Close()
	}

	dec := json.NewDecoder(input)
	// read open square bracket
	_, err = dec.Token()
	if err != nil {
		return errors.Wrap(err, "couldn't unmarshal input data, is it JSON?")
	}

	apiClient, err := api.NewClient(api.Config{Address: alertmanagerURL.String()})
	if err != nil {
		return err
	}
	silenceAPI := client.NewSilenceAPI(apiClient)
	silencec := make(chan *types.Silence, 100)
	errc := make(chan error, 100)
	var wg sync.WaitGroup
	for w := 0; w < c.workers; w++ {
		wg.Add(1)
		go func() {
			addSilenceWorker(silenceAPI, silencec, errc)
			wg.Done()
		}()
	}

	errCount := 0
	go func() {
		for err := range errc {
			if err != nil {
				errCount++
			}
		}
	}()

	count := 0
	for dec.More() {
		var s types.Silence
		err := dec.Decode(&s)
		if err != nil {
			return errors.Wrap(err, "couldn't unmarshal input data, is it JSON?")
		}

		if c.force {
			// reset the silence ID so Alertmanager will always create new silence
			s.ID = ""
		}

		silencec <- &s
		count++
	}

	close(silencec)
	wg.Wait()
	close(errc)

	if errCount > 0 {
		return fmt.Errorf("couldn't import %v out of %v silences", errCount, count)
	}
	return nil
}
