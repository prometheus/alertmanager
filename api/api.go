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
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/pkg/labels"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/pkg/parse"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
	"github.com/weaveworks/mesh"
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
	config         *config.Config
	route          *dispatch.Route
	resolveTimeout time.Duration
	uptime         time.Time
	mrouter        *mesh.Router
	logger         log.Logger

	groups         groupsFn
	getAlertStatus getAlertStatusFn

	mtx sync.RWMutex
}

type groupsFn func([]*labels.Matcher) dispatch.AlertOverview
type getAlertStatusFn func(model.Fingerprint) types.AlertStatus

// New returns a new API.
func New(alerts provider.Alerts, silences *silence.Silences, gf groupsFn, sf getAlertStatusFn, router *mesh.Router, l log.Logger) *API {
	return &API{
		alerts:         alerts,
		silences:       silences,
		groups:         gf,
		getAlertStatus: sf,
		uptime:         time.Now(),
		mrouter:        router,
		logger:         l,
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
	r.Get("/receivers", ihf("receivers", api.receivers))
	r.Get("/alerts/groups", ihf("alert_groups", api.alertGroups))

	r.Get("/alerts", ihf("list_alerts", api.listAlerts))
	r.Post("/alerts", ihf("add_alerts", api.addAlerts))

	r.Get("/silences", ihf("list_silences", api.listSilences))
	r.Post("/silences", ihf("add_silence", api.setSilence))
	r.Get("/silence/:sid", ihf("get_silence", api.getSilence))
	r.Del("/silence/:sid", ihf("del_silence", api.delSilence))
}

// Update sets the configuration string to a new value.
func (api *API) Update(cfg *config.Config, resolveTimeout time.Duration) error {
	api.mtx.Lock()
	defer api.mtx.Unlock()

	api.resolveTimeout = resolveTimeout
	api.config = cfg
	api.route = dispatch.NewRoute(cfg.Route, nil)
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

func (api *API) receivers(w http.ResponseWriter, req *http.Request) {
	api.mtx.RLock()
	defer api.mtx.RUnlock()

	receivers := make([]string, 0, len(api.config.Receivers))
	for _, r := range api.config.Receivers {
		receivers = append(receivers, r.Name)
	}

	api.respond(w, receivers)
}

func (api *API) status(w http.ResponseWriter, req *http.Request) {
	api.mtx.RLock()

	var status = struct {
		ConfigYAML  string            `json:"configYAML"`
		ConfigJSON  *config.Config    `json:"configJSON"`
		VersionInfo map[string]string `json:"versionInfo"`
		Uptime      time.Time         `json:"uptime"`
		MeshStatus  *meshStatus       `json:"meshStatus"`
	}{
		ConfigYAML: api.config.String(),
		ConfigJSON: api.config,
		VersionInfo: map[string]string{
			"version":   version.Version,
			"revision":  version.Revision,
			"branch":    version.Branch,
			"buildUser": version.BuildUser,
			"buildDate": version.BuildDate,
			"goVersion": version.GoVersion,
		},
		Uptime:     api.uptime,
		MeshStatus: getMeshStatus(api),
	}

	api.mtx.RUnlock()

	api.respond(w, status)
}

type meshStatus struct {
	Name     string       `json:"name"`
	NickName string       `json:"nickName"`
	Peers    []peerStatus `json:"peers"`
}

type peerStatus struct {
	Name     string `json:"name"`     // e.g. "00:00:00:00:00:01"
	NickName string `json:"nickName"` // e.g. "a"
	UID      uint64 `json:"uid"`      // e.g. "14015114173033265000"
}

func getMeshStatus(api *API) *meshStatus {
	if api.mrouter == nil {
		return nil
	}

	status := mesh.NewStatus(api.mrouter)
	strippedStatus := &meshStatus{
		Name:     status.Name,
		NickName: status.NickName,
		Peers:    make([]peerStatus, len(status.Peers)),
	}

	for i := 0; i < len(status.Peers); i++ {
		strippedStatus.Peers[i] = peerStatus{
			Name:     status.Peers[i].Name,
			NickName: status.Peers[i].NickName,
			UID:      uint64(status.Peers[i].UID),
		}
	}

	return strippedStatus
}

func (api *API) alertGroups(w http.ResponseWriter, r *http.Request) {
	var err error
	matchers := []*labels.Matcher{}

	if filter := r.FormValue("filter"); filter != "" {
		matchers, err = parse.Matchers(filter)
		if err != nil {
			api.respondError(w, apiError{
				typ: errorBadData,
				err: err,
			}, nil)
			return
		}
	}

	groups := api.groups(matchers)

	api.respond(w, groups)
}

func (api *API) listAlerts(w http.ResponseWriter, r *http.Request) {
	var (
		err error
		re  *regexp.Regexp
		// Initialize result slice to prevent api returning `null` when there
		// are no alerts present
		res           = []*dispatch.APIAlert{}
		matchers      = []*labels.Matcher{}
		showSilenced  = true
		showInhibited = true
	)

	if filter := r.FormValue("filter"); filter != "" {
		matchers, err = parse.Matchers(filter)
		if err != nil {
			api.respondError(w, apiError{
				typ: errorBadData,
				err: err,
			}, nil)
			return
		}
	}

	if silencedParam := r.FormValue("silenced"); silencedParam != "" {
		if silencedParam == "false" {
			showSilenced = false
		} else if silencedParam != "true" {
			api.respondError(w, apiError{
				typ: errorBadData,
				err: fmt.Errorf(
					"parameter 'silenced' can either be 'true' or 'false', not '%v'",
					silencedParam,
				),
			}, nil)
			return
		}
	}

	if inhibitedParam := r.FormValue("inhibited"); inhibitedParam != "" {
		if inhibitedParam == "false" {
			showInhibited = false
		} else if inhibitedParam != "true" {
			api.respondError(w, apiError{
				typ: errorBadData,
				err: fmt.Errorf(
					"parameter 'inhibited' can either be 'true' or 'false', not '%v'",
					inhibitedParam,
				),
			}, nil)
			return
		}
	}

	if receiverParam := r.FormValue("receiver"); receiverParam != "" {
		re, err = regexp.Compile("^(?:" + receiverParam + ")$")
		if err != nil {
			api.respondError(w, apiError{
				typ: errorBadData,
				err: fmt.Errorf(
					"failed to parse receiver param: %s",
					receiverParam,
				),
			}, nil)
			return
		}
	}

	alerts := api.alerts.GetPending()
	defer alerts.Close()

	// TODO(fabxc): enforce a sensible timeout.
	for a := range alerts.Next() {
		if err = alerts.Err(); err != nil {
			break
		}

		routes := api.route.Match(a.Labels)
		receivers := make([]string, 0, len(routes))
		for _, r := range routes {
			receivers = append(receivers, r.RouteOpts.Receiver)
		}

		if re != nil && !regexpAny(re, receivers) {
			continue
		}

		if !alertMatchesFilterLabels(&a.Alert, matchers) {
			continue
		}

		// Continue if alert is resolved
		if !a.Alert.EndsAt.IsZero() && a.Alert.EndsAt.Before(time.Now()) {
			continue
		}

		status := api.getAlertStatus(a.Fingerprint())

		if !showSilenced && len(status.SilencedBy) != 0 {
			continue
		}

		if !showInhibited && len(status.InhibitedBy) != 0 {
			continue
		}

		apiAlert := &dispatch.APIAlert{
			Alert:       &a.Alert,
			Status:      status,
			Receivers:   receivers,
			Fingerprint: a.Fingerprint().String(),
		}

		res = append(res, apiAlert)
	}

	if err != nil {
		api.respondError(w, apiError{
			typ: errorInternal,
			err: err,
		}, nil)
		return
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Fingerprint < res[j].Fingerprint
	})
	api.respond(w, res)
}

func regexpAny(re *regexp.Regexp, ss []string) bool {
	for _, s := range ss {
		if re.MatchString(s) {
			return true
		}
	}

	return false
}

func alertMatchesFilterLabels(a *model.Alert, matchers []*labels.Matcher) bool {
	for _, m := range matchers {
		if v, prs := a.Labels[model.LabelName(m.Name)]; !prs || !m.Matches(string(v)) {
			return false
		}
	}

	return true
}

func (api *API) legacyAddAlerts(w http.ResponseWriter, r *http.Request) {
	var legacyAlerts = []struct {
		Summary     model.LabelValue `json:"summary"`
		Description model.LabelValue `json:"description"`
		Runbook     model.LabelValue `json:"runbook"`
		Labels      model.LabelSet   `json:"labels"`
		Payload     model.LabelSet   `json:"payload"`
	}{}
	if err := api.receive(r, &legacyAlerts); err != nil {
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
	if err := api.receive(r, &alerts); err != nil {
		api.respondError(w, apiError{
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
		api.respondError(w, apiError{
			typ: errorInternal,
			err: err,
		}, nil)
		return
	}

	if validationErrs.Len() > 0 {
		api.respondError(w, apiError{
			typ: errorBadData,
			err: validationErrs,
		}, nil)
		return
	}

	api.respond(w, nil)
}

func (api *API) setSilence(w http.ResponseWriter, r *http.Request) {
	var sil types.Silence
	if err := api.receive(r, &sil); err != nil {
		api.respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
		return
	}
	psil, err := silenceToProto(&sil)
	if err != nil {
		api.respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
		return
	}

	sid, err := api.silences.Set(psil)
	if err != nil {
		api.respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
		return
	}

	api.respond(w, struct {
		SilenceID string `json:"silenceId"`
	}{
		SilenceID: sid,
	})
}

func (api *API) getSilence(w http.ResponseWriter, r *http.Request) {
	sid := route.Param(r.Context(), "sid")

	sils, err := api.silences.Query(silence.QIDs(sid))
	if err != nil || len(sils) == 0 {
		http.Error(w, fmt.Sprint("Error getting silence: ", err), http.StatusNotFound)
		return
	}
	sil, err := silenceFromProto(sils[0])
	if err != nil {
		api.respondError(w, apiError{
			typ: errorInternal,
			err: err,
		}, nil)
		return
	}

	api.respond(w, sil)
}

func (api *API) delSilence(w http.ResponseWriter, r *http.Request) {
	sid := route.Param(r.Context(), "sid")

	if err := api.silences.Expire(sid); err != nil {
		api.respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
		return
	}
	api.respond(w, nil)
}

func (api *API) listSilences(w http.ResponseWriter, r *http.Request) {
	psils, err := api.silences.Query()
	if err != nil {
		api.respondError(w, apiError{
			typ: errorInternal,
			err: err,
		}, nil)
		return
	}

	matchers := []*labels.Matcher{}
	if filter := r.FormValue("filter"); filter != "" {
		matchers, err = parse.Matchers(filter)
		if err != nil {
			api.respondError(w, apiError{
				typ: errorBadData,
				err: err,
			}, nil)
			return
		}
	}

	sils := []*types.Silence{}
	for _, ps := range psils {
		s, err := silenceFromProto(ps)
		if err != nil {
			api.respondError(w, apiError{
				typ: errorInternal,
				err: err,
			}, nil)
			return
		}

		if !matchesFilterLabels(s, matchers) {
			continue
		}
		sils = append(sils, s)
	}

	var active, pending, expired, silences []*types.Silence

	for _, s := range sils {
		switch s.Status.State {
		case "active":
			active = append(active, s)
		case "pending":
			pending = append(pending, s)
		case "expired":
			expired = append(expired, s)
		}
	}

	sort.Slice(active, func(i int, j int) bool {
		return active[i].EndsAt.Before(active[j].EndsAt)
	})
	sort.Slice(pending, func(i int, j int) bool {
		return pending[i].StartsAt.Before(pending[j].EndsAt)
	})
	sort.Slice(expired, func(i int, j int) bool {
		return expired[i].EndsAt.After(expired[j].EndsAt)
	})

	silences = append(silences, active...)
	silences = append(silences, pending...)
	silences = append(silences, expired...)

	api.respond(w, silences)
}

func matchesFilterLabels(s *types.Silence, matchers []*labels.Matcher) bool {
	sms := map[string]string{}
	for _, m := range s.Matchers {
		sms[m.Name] = m.Value
	}
	for _, m := range matchers {
		if v, prs := sms[m.Name]; !prs || !m.Matches(v) {
			return false
		}
	}

	return true
}

func silenceToProto(s *types.Silence) (*silencepb.Silence, error) {
	sil := &silencepb.Silence{
		Id:        s.ID,
		StartsAt:  s.StartsAt,
		EndsAt:    s.EndsAt,
		UpdatedAt: s.UpdatedAt,
		Comment:   s.Comment,
		CreatedBy: s.CreatedBy,
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
	return sil, nil
}

func silenceFromProto(s *silencepb.Silence) (*types.Silence, error) {
	sil := &types.Silence{
		ID:        s.Id,
		StartsAt:  s.StartsAt,
		EndsAt:    s.EndsAt,
		UpdatedAt: s.UpdatedAt,
		Status: types.SilenceStatus{
			State: types.CalcSilenceState(s.StartsAt, s.EndsAt),
		},
		Comment:   s.Comment,
		CreatedBy: s.CreatedBy,
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

func (api *API) respond(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	b, err := json.Marshal(&response{
		Status: statusSuccess,
		Data:   data,
	})
	if err != nil {
		level.Error(api.logger).Log("msg", "Error marshalling JSON", "err", err)
		return
	}
	w.Write(b)
}

func (api *API) respondError(w http.ResponseWriter, apiErr apiError, data interface{}) {
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
	level.Error(api.logger).Log("msg", "API error", "err", apiErr.Error())

	w.Write(b)
}

func (api *API) receive(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	err := dec.Decode(v)
	if err != nil {
		level.Debug(api.logger).Log("msg", "Decoding request failed", "err", err)
	}
	return err
}
