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

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/version"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
)

var (
	numReceivedAlerts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "alertmanager",
		Name:      "alerts_received_total",
		Help:      "The total number of received alerts.",
	}, []string{"status"})

	numInvalidAlerts = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "alertmanager",
		Name:      "alerts_invalid_total",
		Help:      "The total number of received alerts that were invalid.",
	})
)

func init() {
	prometheus.Register(numReceivedAlerts)
	prometheus.Register(numInvalidAlerts)
}

var corsHeaders = map[string]string{
	"Access-Control-Allow-Headers":  "Accept, Authorization, Content-Type, Origin",
	"Access-Control-Allow-Methods":  "GET, DELETE, OPTIONS",
	"Access-Control-Allow-Origin":   "*",
	"Access-Control-Expose-Headers": "Date",
}

// Enables cross-site script calls.
func setCORS(w http.ResponseWriter) {
	for h, v := range corsHeaders {
		w.Header().Set(h, v)
	}
}

// API provides registration of handlers for API routes.
type API struct {
	alerts         provider.Alerts
	silences       *silence.Silences
	config         string
	configJSON     config.Config
	resolveTimeout time.Duration
	uptime         time.Time

	groups func() dispatch.AlertOverview

	// context is an indirection for testing.
	context func(r *http.Request) context.Context
	mtx     sync.RWMutex
}

// New returns a new API.
func New(alerts provider.Alerts, silences *silence.Silences, gf func() dispatch.AlertOverview) *API {
	return &API{
		context:  route.Context,
		alerts:   alerts,
		silences: silences,
		groups:   gf,
		uptime:   time.Now(),
	}
}

// Register registers the API handlers under their correct routes
// in the given router.
func (api *API) Register(r *route.Router) {
	ihf := func(name string, f http.HandlerFunc) http.HandlerFunc {
		return prometheus.InstrumentHandlerFunc(name, func(w http.ResponseWriter, r *http.Request) {
			setCORS(w)
			f(w, r)
		})
	}

	r.Options("/*path", ihf("options", func(w http.ResponseWriter, r *http.Request) {}))

	// Register legacy forwarder for alert pushing.
	r.Post("/alerts", ihf("legacy_add_alerts", api.legacyAddAlerts))

	// Register actual API.
	r = r.WithPrefix("/v1")

	r.Get("/status", ihf("status", api.status))
	r.Get("/alerts/groups", ihf("alert_groups", api.alertGroups))

	r.Get("/alerts", ihf("list_alerts", api.listAlerts))
	r.Post("/alerts", ihf("add_alerts", api.addAlerts))

	r.Get("/silences", ihf("list_silences", api.listSilences))
	r.Post("/silences", ihf("add_silence", api.addSilence))
	r.Get("/silence/:sid", ihf("get_silence", api.getSilence))
	r.Del("/silence/:sid", ihf("del_silence", api.delSilence))
}

// Update sets the configuration string to a new value.
func (api *API) Update(cfg string, resolveTimeout time.Duration) error {
	api.mtx.Lock()
	defer api.mtx.Unlock()

	api.config = cfg
	api.resolveTimeout = resolveTimeout

	configJSON, err := config.Load(cfg)
	if err != nil {
		log.Errorf("error: %v", err)
		return err
	}

	api.configJSON = *configJSON
	return nil
}

type errorType string

const (
	errorNone     errorType = ""
	errorInternal           = "server_error"
	errorBadData            = "bad_data"
)

type apiError struct {
	typ errorType
	err error
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s: %s", e.typ, e.err)
}

func (api *API) status(w http.ResponseWriter, req *http.Request) {
	api.mtx.RLock()

	var status = struct {
		Config      string            `json:"config"`
		ConfigJSON  config.Config     `json:"configJSON"`
		VersionInfo map[string]string `json:"versionInfo"`
		Uptime      time.Time         `json:"uptime"`
	}{
		Config:     api.config,
		ConfigJSON: api.configJSON,
		VersionInfo: map[string]string{
			"version":   version.Version,
			"revision":  version.Revision,
			"branch":    version.Branch,
			"buildUser": version.BuildUser,
			"buildDate": version.BuildDate,
			"goVersion": version.GoVersion,
		},
		Uptime: api.uptime,
	}

	api.mtx.RUnlock()

	respond(w, status)
}

func (api *API) alertGroups(w http.ResponseWriter, req *http.Request) {
	respond(w, api.groups())
}

func (api *API) listAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := api.alerts.GetPending()
	defer alerts.Close()

	var (
		err error
		res []*types.Alert
	)
	// TODO(fabxc): enforce a sensible timeout.
	for a := range alerts.Next() {
		if err = alerts.Err(); err != nil {
			break
		}
		res = append(res, a)
	}

	if err != nil {
		respondError(w, apiError{
			typ: errorInternal,
			err: err,
		}, nil)
		return
	}
	respond(w, types.Alerts(res...))
}

func (api *API) legacyAddAlerts(w http.ResponseWriter, r *http.Request) {
	var legacyAlerts = []struct {
		Summary     model.LabelValue `json:"summary"`
		Description model.LabelValue `json:"description"`
		Runbook     model.LabelValue `json:"runbook"`
		Labels      model.LabelSet   `json:"labels"`
		Payload     model.LabelSet   `json:"payload"`
	}{}
	if err := receive(r, &legacyAlerts); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var alerts []*types.Alert
	for _, la := range legacyAlerts {
		a := &types.Alert{
			Alert: model.Alert{
				Labels:      la.Labels,
				Annotations: la.Payload,
			},
		}
		if a.Annotations == nil {
			a.Annotations = model.LabelSet{}
		}
		a.Annotations["summary"] = la.Summary
		a.Annotations["description"] = la.Description
		a.Annotations["runbook"] = la.Runbook

		alerts = append(alerts, a)
	}

	api.insertAlerts(w, r, alerts...)
}

func (api *API) addAlerts(w http.ResponseWriter, r *http.Request) {
	var alerts []*types.Alert
	if err := receive(r, &alerts); err != nil {
		respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
		return
	}

	api.insertAlerts(w, r, alerts...)
}

func (api *API) insertAlerts(w http.ResponseWriter, r *http.Request, alerts ...*types.Alert) {
	now := time.Now()

	for _, alert := range alerts {
		alert.UpdatedAt = now

		// Ensure StartsAt is set.
		if alert.StartsAt.IsZero() {
			alert.StartsAt = now
		}
		// If no end time is defined, set a timeout after which an alert
		// is marked resolved if it is not updated.
		if alert.EndsAt.IsZero() {
			alert.Timeout = true
			alert.EndsAt = now.Add(api.resolveTimeout)

			numReceivedAlerts.WithLabelValues("firing").Inc()
		} else {
			numReceivedAlerts.WithLabelValues("resolved").Inc()
		}
	}

	// Make a best effort to insert all alerts that are valid.
	var (
		validAlerts    = make([]*types.Alert, 0, len(alerts))
		validationErrs = &types.MultiError{}
	)
	for _, a := range alerts {
		if err := a.Validate(); err != nil {
			validationErrs.Add(err)
			numInvalidAlerts.Inc()
			continue
		}
		validAlerts = append(validAlerts, a)
	}
	if err := api.alerts.Put(validAlerts...); err != nil {
		respondError(w, apiError{
			typ: errorInternal,
			err: err,
		}, nil)
		return
	}

	if validationErrs.Len() > 0 {
		respondError(w, apiError{
			typ: errorBadData,
			err: validationErrs,
		}, nil)
		return
	}

	respond(w, nil)
}

func (api *API) addSilence(w http.ResponseWriter, r *http.Request) {
	var sil types.Silence
	if err := receive(r, &sil); err != nil {
		respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
		return
	}
	psil, err := silenceToProto(&sil)
	if err != nil {
		respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
		return
	}
	// Drop start time for new silences so we default to now.
	if sil.ID == "" && sil.StartsAt.Before(time.Now()) {
		psil.StartsAt = nil
	}

	sid, err := api.silences.Create(psil)
	if err != nil {
		respondError(w, apiError{
			typ: errorInternal,
			err: err,
		}, nil)
		return
	}

	respond(w, struct {
		SilenceID string `json:"silenceId"`
	}{
		SilenceID: sid,
	})
}

func (api *API) getSilence(w http.ResponseWriter, r *http.Request) {
	sid := route.Param(api.context(r), "sid")

	sils, err := api.silences.Query(silence.QIDs(sid))
	if err != nil || len(sils) == 0 {
		http.Error(w, fmt.Sprint("Error getting silence: ", err), http.StatusNotFound)
		return
	}
	sil, err := silenceFromProto(sils[0])
	if err != nil {
		respondError(w, apiError{
			typ: errorInternal,
			err: err,
		}, nil)
		return
	}

	respond(w, sil)
}

func (api *API) delSilence(w http.ResponseWriter, r *http.Request) {
	sid := route.Param(api.context(r), "sid")

	if err := api.silences.Expire(sid); err != nil {
		respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
		return
	}
	respond(w, nil)
}

func (api *API) listSilences(w http.ResponseWriter, r *http.Request) {
	psils, err := api.silences.Query()
	if err != nil {
		respondError(w, apiError{
			typ: errorInternal,
			err: err,
		}, nil)
		return
	}

	var sils []*types.Silence
	for _, ps := range psils {
		s, err := silenceFromProto(ps)
		if err != nil {
			respondError(w, apiError{
				typ: errorInternal,
				err: err,
			}, nil)
			return
		}
		sils = append(sils, s)
	}

	respond(w, sils)
}

func silenceToProto(s *types.Silence) (*silencepb.Silence, error) {
	startsAt, err := ptypes.TimestampProto(s.StartsAt)
	if err != nil {
		return nil, err
	}
	endsAt, err := ptypes.TimestampProto(s.EndsAt)
	if err != nil {
		return nil, err
	}
	updatedAt, err := ptypes.TimestampProto(s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	sil := &silencepb.Silence{
		Id:        s.ID,
		StartsAt:  startsAt,
		EndsAt:    endsAt,
		UpdatedAt: updatedAt,
	}
	for _, m := range s.Matchers {
		matcher := &silencepb.Matcher{
			Name:    m.Name,
			Pattern: m.Value,
			Type:    silencepb.Matcher_EQUAL,
		}
		if m.IsRegex {
			matcher.Type = silencepb.Matcher_REGEXP
		}
		sil.Matchers = append(sil.Matchers, matcher)
	}
	sil.Comments = append(sil.Comments, &silencepb.Comment{
		Timestamp: updatedAt,
		Author:    s.CreatedBy,
		Comment:   s.Comment,
	})
	return sil, nil
}

func silenceFromProto(s *silencepb.Silence) (*types.Silence, error) {
	startsAt, err := ptypes.Timestamp(s.StartsAt)
	if err != nil {
		return nil, err
	}
	endsAt, err := ptypes.Timestamp(s.EndsAt)
	if err != nil {
		return nil, err
	}
	updatedAt, err := ptypes.Timestamp(s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	sil := &types.Silence{
		ID:        s.Id,
		StartsAt:  startsAt,
		EndsAt:    endsAt,
		UpdatedAt: updatedAt,
	}
	for _, m := range s.Matchers {
		matcher := &types.Matcher{
			Name:  m.Name,
			Value: m.Pattern,
		}
		switch m.Type {
		case silencepb.Matcher_EQUAL:
		case silencepb.Matcher_REGEXP:
			matcher.IsRegex = true
		default:
			return nil, fmt.Errorf("unknown matcher type")
		}
		sil.Matchers = append(sil.Matchers, matcher)
	}
	if len(s.Comments) > 0 {
		sil.CreatedBy = s.Comments[0].Author
		sil.Comment = s.Comments[0].Comment
	}

	return sil, nil
}

type status string

const (
	statusSuccess status = "success"
	statusError          = "error"
)

type response struct {
	Status    status      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType errorType   `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
}

func respond(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	b, err := json.Marshal(&response{
		Status: statusSuccess,
		Data:   data,
	})
	if err != nil {
		log.Errorf("errorr: %v", err)
		return
	}
	w.Write(b)
}

func respondError(w http.ResponseWriter, apiErr apiError, data interface{}) {
	w.Header().Set("Content-Type", "application/json")

	switch apiErr.typ {
	case errorBadData:
		w.WriteHeader(http.StatusBadRequest)
	case errorInternal:
		w.WriteHeader(http.StatusInternalServerError)
	default:
		panic(fmt.Sprintf("unknown error type %q", apiErr))
	}

	b, err := json.Marshal(&response{
		Status:    statusError,
		ErrorType: apiErr.typ,
		Error:     apiErr.err.Error(),
		Data:      data,
	})
	if err != nil {
		return
	}
	log.Errorf("api error: %v", apiErr.Error())

	w.Write(b)
}

func receive(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	err := dec.Decode(v)
	if err != nil {
		log.Debugf("Decoding request failed: %v", err)
	}
	return err
}
