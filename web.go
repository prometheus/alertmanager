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
	"html/template"
	"net/http"

	"github.com/prometheus/common/route"
)

func RegisterWeb(r *route.Router) {

	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		t := template.Must(template.ParseFiles("ui/index.html"))
		t.Execute(w, nil)
	})

	r.Get("/app/*filepath", route.FileServe("ui/app"))
	r.Get("/static/*filepath", route.FileServe("ui/static/"))

}
