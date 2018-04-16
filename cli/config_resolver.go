package cli

import (
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
)

var (
	// kingpin transforms flags to <APPNAME>_<FLAG>.
	envarTransformRegexp = regexp.MustCompile(`[^a-zA-Z0-9_]+`)
	oldFlags             = map[string]string{
		"comment_required": "require-comment",
	}
)

// loadConfig loads flags from YAML files and injects them as environment variables.
func loadConfig(app *kingpin.Application, files ...string) error {
	flags := map[string]string{}
	for _, f := range files {
		b, err := ioutil.ReadFile(f)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}

		var m map[string]string
		err = yaml.Unmarshal(b, &m)
		if err != nil {
			return err
		}
		for k, v := range m {
			if new, ok := oldFlags[k]; ok {
				if _, ok := m[new]; ok {
					continue
				}
				k = new
			}
			if _, ok := flags[k]; !ok {
				flags[k] = v
			}
		}
	}

	// kingpin maps flags to <APPNAME>_<FLAG> variables.
	for k, v := range flags {
		k := strings.ToUpper(envarTransformRegexp.ReplaceAllString(app.Name+"_"+k, "_"))
		os.Setenv(k, v)
	}

	return nil
}
