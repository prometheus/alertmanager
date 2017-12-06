package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/prometheus/alertmanager/types"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"sync"
)

var importFlags *flag.FlagSet
var importCmd = &cobra.Command{
	Use:   "import [JSON file]",
	Short: "Import silences",
	Long: `Import alertmanager silences from JSON file or stdin

  This command can be used to bulk import silences from a JSON file
  created by query command. For example:

  amtool silence query -o json foo > foo.json
  amtool silence import foo.json

  JSON data can also come from stdin if no param is specified.
	`,
	Args: cobra.MaximumNArgs(1),
	Run:  CommandWrapper(bulkImport),
}

func init() {
	importCmd.Flags().BoolP("force", "f", false, "Force adding new silences even if it already exists")
	importCmd.Flags().IntP("worker", "w", 8, "Number of concurrent workers to use for import")
	importFlags = importCmd.Flags()
}

func addSilenceWorker(silencec <-chan *types.Silence, errc chan<- error) {
	for s := range silencec {
		silenceId, err := addSilence(s)
		sid := s.ID
		if err != nil && err.Error() == "[bad_data] not found" {
			// silence doesn't exists yet, retry to create as a new one
			s.ID = ""
			silenceId, err = addSilence(s)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error adding silence id='%v': %v", sid, err)
		} else {
			fmt.Println(silenceId)
		}
		errc <- err
	}
}

func bulkImport(cmd *cobra.Command, args []string) error {
	force, err := importFlags.GetBool("force")
	if err != nil {
		return err
	}

	workers, err := importFlags.GetInt("worker")
	if err != nil {
		return err
	}

	input := os.Stdin
	if len(args) == 1 {
		input, err = os.Open(args[0])
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

	silencec := make(chan *types.Silence, 100)
	errc := make(chan error, 100)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		go func() {
			wg.Add(1)
			addSilenceWorker(silencec, errc)
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

		if force {
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
