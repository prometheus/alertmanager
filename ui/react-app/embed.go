// Copyright 2023 The Prometheus Authors
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

package reactApp

import "embed"

//go:embed dist/26.js.gz dist/26.js.map.gz dist/732.js.gz dist/732.js.map.gz dist/index.html.gz dist/main.js.gz dist/main.js.map.gz
var embedFS embed.FS
