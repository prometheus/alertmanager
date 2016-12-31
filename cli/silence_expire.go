package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var expireCmd = &cobra.Command{
	Use:   "expire",
	Short: "expire silence",
	Long:  `expire an alertmanager silence`,
	RunE:  expire,
}

func expire(cmd *cobra.Command, args []string) error {
	u, err := url.Parse(viper.GetString("alertmanager"))
	if err != nil {
		return err
	}
	basePath := path.Join(u.Path, "/api/v1/silence")

	for _, arg := range args {
		u.Path = path.Join(basePath, arg)
		req, err := http.NewRequest("DELETE", u.String(), nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}

		defer res.Body.Close()
		decoder := json.NewDecoder(res.Body)

		response := alertmanagerResponse{}
		err = decoder.Decode(&response)
		if err != nil {
			return err
		}
		if response.Status == "error" {
			fmt.Printf("[%s] %s", response.ErrorType, response.Error)
		}

	}
	return nil
}
