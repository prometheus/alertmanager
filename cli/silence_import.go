// Copyright 2018 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	kingpin "github.com/alecthomas/kingpin/v2"

	"github.com/prometheus/alertmanager/api/v2/client/silence"
	"github.com/prometheus/alertmanager/api/v2/models"
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
	importCmd.Action(execWithTimeout(c.bulkImport))
}

func addSilenceWorker(ctx context.Context, sclient silence.ClientService, silencec <-chan *models.PostableSilence, errc chan<- error) {
	for s := range silencec {
		sid := s.ID
		params := silence.NewPostSilencesParams().WithContext(ctx).WithSilence(s)
		postOk, err := sclient.PostSilences(params)
		var e *silence.PostSilencesNotFound
		if errors.As(err, &e) {
			// silence doesn't exists yet, retry to create as a new one
			params.Silence.ID = ""
			postOk, err = sclient.PostSilences(params)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error adding silence id='%v': %v\n", sid, err)
		} else {
			fmt.Println(postOk.Payload.SilenceID)
		}
		errc <- err
	}
}

func (c *silenceImportCmd) bulkImport(ctx context.Context, _ *kingpin.ParseContext) error {
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
		return fmt.Errorf("couldn't unmarshal input data, is it JSON?: %w", err)
	}

	amclient := NewAlertmanagerClient(alertmanagerURL)
	silencec := make(chan *models.PostableSilence, 100)
	errc := make(chan error, 100)
	errDone := make(chan struct{})

	var wg sync.WaitGroup
	var once sync.Once

	closeChannels := func() {
		once.Do(func() {
			close(silencec)
			wg.Wait()
			close(errc)
			<-errDone
			close(errDone)
		})
	}
	defer closeChannels()
	for w := 0; w < c.workers; w++ {
		wg.Go(func() {
			addSilenceWorker(ctx, amclient.Silence, silencec, errc)
		})
	}

	errCount := 0
	go func() {
		for err := range errc {
			if err != nil {
				errCount++
			}
		}
		errDone <- struct{}{}
	}()

	count := 0
	for dec.More() {
		var s models.PostableSilence
		err := dec.Decode(&s)
		if err != nil {
			return fmt.Errorf("couldn't unmarshal input data, is it JSON?: %w", err)
		}

		if c.force {
			// reset the silence ID so Alertmanager will always create new silence
			s.ID = ""
		}

		silencec <- &s
		count++
	}

	closeChannels()

	if errCount > 0 {
		return fmt.Errorf("couldn't import %v out of %v silences", errCount, count)
	}
	return nil
}
