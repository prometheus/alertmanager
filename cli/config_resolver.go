package cli

import (
	"io/ioutil"
	"os"

	"github.com/alecthomas/kingpin"
	"gopkg.in/yaml.v2"
)

type configResolver []map[string]string

func newConfigResolver() (configResolver, error) {
	files := []string{
		os.ExpandEnv("$HOME/.config/amtool/config.yml"),
		"/etc/amtool/config.yml",
	}

	resolver := configResolver{}
	for _, f := range files {
		b, err := ioutil.ReadFile(f)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		var m map[string]string
		err = yaml.Unmarshal(b, &m)
		if err != nil {
			return nil, err
		}
		resolver = append(resolver, m)
	}

	return resolver, nil
}

func (r configResolver) Resolve(key string, context *kingpin.ParseContext) ([]string, error) {
	for _, c := range r {
		if v, ok := c[key]; ok {
			return []string{v}, nil
		}
	}
	return nil, nil
}

// This function maps things which have previously had different names in the
// config file to their new names, so old configurations keep working
func backwardsCompatibilityResolver(key string) string {
	switch key {
	case "require-comment":
		return "comment_required"
	}
	return key
}
