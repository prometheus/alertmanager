// Copyright 2015 Prometheus Team
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
	"io/ioutil"
	"net/http"
	"os"

	"github.com/prometheus/common/route"
)

func RegisterWeb(r *route.Router) {

	r.Get("/app/*filepath", route.FileServe("ui/app/"))
	r.Get("/static/*filepath", route.FileServe("ui/lib/"))

	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		f, err := os.Open("ui/app/index.html")
		if err != nil {
			panic(err)
		}
		defer f.Close()

		b, err := ioutil.ReadAll(f)
		if err != nil {
			panic(err)
		}
		w.Write(b)
	})
}
