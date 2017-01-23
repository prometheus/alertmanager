package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/user"
	"path"
	"time"

	"github.com/prometheus/alertmanager/types"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type addResponse struct {
	Status string `json:"status"`
	Data   struct {
		SilenceID string `json:"silenceId"`
	} `json:"data,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
	Error     string `json:"error,omitempty"`
}

var addFlags *flag.FlagSet
var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add silence",
	Long:  `Add a new alertmanager silence`,
	RunE:  add,
}

func init() {
	user, _ := user.Current()
	addCmd.Flags().StringP("author", "a", user.Username, "Username for CreatedBy field")
	addCmd.Flags().StringP("expires", "e", "1h", "Duration of silence (100h)")
	addCmd.Flags().StringP("until", "u", "", "Expire at a certain time (Overwrites expires) RFC3339 format 2006-01-02T15:04:05Z07:00")
	addCmd.Flags().StringP("comment", "c", "", "A comment to help describe the silence")
	viper.BindPFlag("author", addCmd.Flags().Lookup("author"))
	viper.BindPFlag("expires", addCmd.Flags().Lookup("expires"))
	viper.BindPFlag("comment", addCmd.Flags().Lookup("comment"))
	viper.SetDefault("comment_required", false)
	addFlags = addCmd.Flags()
}

func add(cmd *cobra.Command, args []string) error {
	var err error
	matchers, err := parseMatchers(args)
	if err != nil {
		return err
	}

	groups := parseMatcherGroups(matchers)

	until, err := addFlags.GetString("until")
	if err != nil {
		return err
	}

	expires := viper.GetString("expires")
	var endsAt time.Time

	if until != "" {
		endsAt, err = time.Parse(time.RFC3339, until)
		if err != nil {
			return err
		}
	} else {
		duration, err := time.ParseDuration(expires)
		if err != nil {
			return err
		}
		endsAt = time.Now().UTC().Add(duration)
	}

	author := viper.GetString("author")
	comment := viper.GetString("comment")
	comment_required := viper.GetBool("comment_required")

	if comment_required && comment == "" {
		return errors.New("Comment required by config")
	}

	for _, matchers := range groups {
		silence := types.Silence{
			Matchers:  matchers,
			StartsAt:  time.Now().UTC(),
			EndsAt:    endsAt,
			CreatedBy: author,
			Comment:   comment,
		}

		u, err := url.ParseRequestURI(viper.GetString("alertmanager.url"))
		if err != nil {
			return err
		}
		u.Path = path.Join(u.Path, "/api/v1/silences")

		buf := bytes.NewBuffer([]byte{})
		enc := json.NewEncoder(buf)
		err = enc.Encode(silence)
		if err != nil {
			return err
		}

		res, err := http.Post(u.String(), "application/json", buf)
		if err != nil {
			return err
		}

		defer res.Body.Close()
		decoder := json.NewDecoder(res.Body)

		response := addResponse{}
		err = decoder.Decode(&response)
		if err != nil {
			return err
		}

		if response.Status == "error" {
			fmt.Printf("[%s] %s\n", response.ErrorType, response.Error)
		} else {
			fmt.Println(response.Data.SilenceID)
		}
	}
	return nil
}
