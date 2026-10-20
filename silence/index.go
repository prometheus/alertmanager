// Copyright The Prometheus Authors
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

package silence

import (
	"fmt"
	"sort"

	"github.com/prometheus/alertmanager/pkg/labels"
	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

type matcherIndex map[string]labels.MatcherSet

// get retrieves the matcher set for a given silence.
func (c matcherIndex) get(s *pb.Silence) (labels.MatcherSet, error) {
	if m, ok := c[s.Id]; ok {
		return m, nil
	}
	return nil, ErrNotFound
}

// add compiles a silences' matchers and adds them to the cache.
// It returns the compiled matcher set.
func (c matcherIndex) add(s *pb.Silence) (labels.MatcherSet, error) {
	matcherSet := make(labels.MatcherSet, 0, len(s.MatcherSets))

	for _, ms := range s.MatcherSets {
		matchers := make(labels.Matchers, len(ms.Matchers))

		for i, m := range ms.Matchers {
			var mt labels.MatchType
			switch m.Type {
			case pb.Matcher_EQUAL:
				mt = labels.MatchEqual
			case pb.Matcher_NOT_EQUAL:
				mt = labels.MatchNotEqual
			case pb.Matcher_REGEXP:
				mt = labels.MatchRegexp
			case pb.Matcher_NOT_REGEXP:
				mt = labels.MatchNotRegexp
			default:
				return nil, fmt.Errorf("unknown matcher type %q", m.Type)
			}
			matcher, err := labels.NewMatcher(mt, m.Name, m.Pattern)
			if err != nil {
				return nil, err
			}

			matchers[i] = matcher
		}

		matcherSet = append(matcherSet, &matchers)
	}

	c[s.Id] = matcherSet
	return matcherSet, nil
}

// silenceVersion associates a silence with the Silences version when it was created.
type silenceVersion struct {
	id      string
	version int
}

// versionIndex is a index into Silences ordered by the version of Silences when the
// silence was created. The index is always sorted from lowest to highest version.
//
// The versionIndex allows clients of Silences.Query to incrementally update local caches
// of query results. Instead of a new version requiring the client to scan  everything
// again to get an up-to-date picture of Silences, they can use the versionIndex to figure
// out which silences have been added since the last version they saw. This means they can
// just scan the NEW silences, rather than all of them.
type versionIndex []silenceVersion

// add pushes a new silenceVersionMapping to the back of the silenceVersionIndex. It does not
// validate the input.
func (s *versionIndex) add(version int, sil string) {
	*s = append(*s, silenceVersion{version: version, id: sil})
}

// findVersionGreaterThan uses a log(n) search to find the first index of the versionIndex
// which has a version higher than version. If any entries with a higher version exist,
// it returns true and the starting index (which is guaranteed to be a valid index into
// the slice). Otherwise it returns false.
func (s versionIndex) findVersionGreaterThan(version int) (index int, found bool) {
	startIdx := sort.Search(len(s), func(i int) bool {
		return s[i].version > version
	})
	return startIdx, startIdx < len(s)
}

func (s *Silences) indexSilence(sil *pb.Silence) {
	s.version++
	s.vi.add(s.version, sil.Id)
	_, err := s.mi.add(sil)
	if err != nil {
		s.metrics.matcherCompileIndexSilenceErrorsTotal.Inc()
		s.logger.Error("Failed to compile silence matchers", "silence_id", sil.Id, "err", err)
	}
}
