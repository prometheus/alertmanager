package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"

	"github.com/alecthomas/kingpin"
)

var (
	expireCmd = silenceCmd.Command("expire", "expire an alertmanager silence")
	expireIds = expireCmd.Arg("silence-ids", "Ids of silences to expire").Strings()
)

func init() {
	expireCmd.Action(expire)
	longHelpText["silence expire"] = `Expire an alertmanager silence`
}

func expire(element *kingpin.ParseElement, ctx *kingpin.ParseContext) error {
	if len(*expireIds) < 1 {
		return errors.New("no silence IDs specified")
	}

	basePath := "/api/v1/silence"
	for _, id := range *expireIds {
		u := GetAlertmanagerURL(path.Join(basePath, id))
		req, err := http.NewRequest("DELETE", u.String(), nil)
		if err != nil {
			return err
		}
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
