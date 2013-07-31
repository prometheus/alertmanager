// Copyright 2013 Prometheus Team
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

package api

import (
	"code.google.com/p/gorest"

	"github.com/prometheus/alert_manager/manager"
)

type AlertManagerService struct {
	gorest.RestService `root:"/api/" consumes:"application/json" produces:"application/json"`

	addEvents      gorest.EndPoint `method:"POST" path:"/events" postdata:"Events"`
	addSilence     gorest.EndPoint `method:"POST" path:"/silences" postdata:"Silence"`
	getSilence     gorest.EndPoint `method:"GET" path:"/silences/{id:int}" output:"string"`
	updateSilence  gorest.EndPoint `method:"POST" path:"/silences/{id:int}" postdata:"Silence"`
	delSilence     gorest.EndPoint `method:"DELETE" path:"/silences/{id:int}"`
	silenceSummary gorest.EndPoint `method:"GET" path:"/silences" output:"string"`

	Aggregator *manager.Aggregator
	Silencer   *manager.Silencer
}
