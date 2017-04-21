package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"

	"github.com/spf13/cobra"
)

var expireCmd = &cobra.Command{
	Use:   "expire",
	Short: "expire silence",
	Long:  `expire an alertmanager silence`,
	Run:   CommandWrapper(expire),
}

func expire(cmd *cobra.Command, args []string) error {
	u, err := GetAlertmanagerURL()
	if err != nil {
		return err
	}
	basePath := path.Join(u.Path, "/api/v1/silence")

	if len(args) < 1 {
		return errors.New("No silence IDs specified")
	}

	for _, arg := range args {
		u.Path = path.Join(basePath, arg)
		req, err := http.NewRequest("DELETE", u.String(), nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}

		defer res.Body.Close()
		decoder := json.NewDecoder(res.Body)

		response := alertmanagerSilenceResponse{}
		err = decoder.Decode(&response)
		if err != nil {
			return err
		}

		if response.Status == "error" {
			return errors.New(response.Error)
		}
	}
	return nil
}
