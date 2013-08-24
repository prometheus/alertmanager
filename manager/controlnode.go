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

package manager

import (
	"sync"
)

type AlertGeneratorNode interface {
	SetOutputNode(AlertReceiverNode)
}

type AlertReceiverNode interface {
	SetInput(Alerts)
}

// Inhibition control node. Calculates inhibition rules between its event
// inputs and only emits uninhibited events.
type InhibitFilter struct {
  mu sync.Mutex

  input Alerts
  output AlertReceiverNode

  inhibitRules InhibitRules
}

func (i *InhibitFilter) SetInhibitRules(r InhibitRules) {
  i.mu.Lock()
  defer i.mu.Unlock()

  i.inhibitRules = r
  i.refreshOutput()
}

func (i *InhibitFilter) SetInput(a Alerts) {
  i.mu.Lock()
  defer i.mu.Unlock()

  i.input = a
  i.refreshOutput()
}

func (i *InhibitFilter) SetOutputNode(n AlertReceiverNode) {
  i.mu.Lock()
  defer i.mu.Unlock()

  i.output = n
  i.refreshOutput()
}

func (i *InhibitFilter) refreshOutput() {
  a := i.inhibitRules.Filter(i.input)
  i.output.SetInput(a)
}

// Silencing control node.
type SilenceFilter struct {
  mu sync.Mutex

  input Alerts
  output AlertReceiverNode

  silencer *Silencer
}

func NewSilenceFilter(s *Silencer) *SilenceFilter {
  return &SilenceFilter{
    silencer: s,
  }
}

func (s *SilenceFilter) SetInput(a Alerts) {
  s.mu.Lock()
  defer s.mu.Unlock()

  s.input = a
  s.refreshOutput()
}

func (s *SilenceFilter) SetOutputNode(n AlertReceiverNode) {
  s.mu.Lock()
  defer s.mu.Unlock()

  s.output = n
  s.refreshOutput()
}


func (s *SilenceFilter) refreshOutput() {
	out := Alerts{}
	for _, a := range s.input {
		if silenced, _ := s.silencer.IsSilenced(a); !silenced {
			out = append(out, a)
		}
	}
  s.output.SetInput(out)
}

// Repeat-interval control node.
type RepeatFilter struct {
  mu sync.Mutex

  input Alerts
  output AlertReceiverNode
}

func NewRepeatFilter() *RepeatFilter {
  return &RepeatFilter{}
}

func (r *RepeatFilter) SetInput(a Alerts) {
  r.mu.Lock()
  defer r.mu.Unlock()

  r.input = a
  r.refreshOutput()
}

func (r *RepeatFilter) SetOutputNode(n AlertReceiverNode) {
  r.mu.Lock()
  defer r.mu.Unlock()

  r.output = n
  r.refreshOutput()
}

func (r *RepeatFilter) refreshOutput() {
	// TODO: only output alerts which haven't notified recently.
  r.output.SetInput(r.input)
}
