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

package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/prometheus/alertmanager/notify/exec"
)

func main() {
	var msg exec.Message
	err := json.NewDecoder(os.Stdin).Decode(&msg)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("version=%q, group_key=%s\n", msg.Version, msg.GroupKey)

	os.Exit(exec.ExitSuccess)
}
