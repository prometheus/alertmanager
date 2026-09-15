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

package config

import (
	"os"

	"github.com/alecthomas/kingpin/v2"
	"gopkg.in/yaml.v2"
)

type getFlagger interface {
	GetFlag(name string) *kingpin.FlagClause
}

// Resolver represents a configuration file resolver for kingpin.
type Resolver struct {
	flags map[string]string
}

// NewResolver returns a Resolver structure.
func NewResolver(files []string, legacyFlags map[string]string) (*Resolver, error) {
	flags := map[string]string{}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			continue
		}
		b, err := os.ReadFile(f)
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
		for k, v := range m {
			if flag, ok := legacyFlags[k]; ok {
				if _, ok := m[flag]; ok {
					continue
				}
				k = flag
			}
			if _, ok := flags[k]; !ok {
				flags[k] = v
			}
		}
	}

	return &Resolver{flags: flags}, nil
}

func (c *Resolver) setDefault(v getFlagger) {
	for name, value := range c.flags {
		f := v.GetFlag(name)
		if f != nil {
			f.Default(value)
		}
	}
}

// Bind sets active flags with their default values from the configuration file(s).
func (c *Resolver) Bind(app *kingpin.Application, args []string) error {
	// Parse the command line arguments to get the selected command.
	pc, err := app.ParseContext(args)
	if err != nil {
		return err
	}

	c.setDefault(app)
	if pc.SelectedCommand != nil {
		c.setDefault(pc.SelectedCommand)
	}

	return nil
}
