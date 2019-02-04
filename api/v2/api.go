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

package v2

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	prometheus_model "github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/rs/cors"

	open_api_models "github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/api/v2/restapi"
	"github.com/prometheus/alertmanager/api/v2/restapi/operations"
	alert_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/alert"
	general_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/general"
	receiver_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/receiver"
	silence_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/silence"
	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/pkg/parse"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
)

// API represents an Alertmanager API v2
type API struct {
	peer           *cluster.Peer
	silences       *silence.Silences
	alerts         provider.Alerts
	getAlertStatus getAlertStatusFn
	uptime         time.Time

	// mtx protects resolveTimeout, alertmanagerConfig and route.
	mtx sync.RWMutex
	// resolveTimeout represents the default resolve timeout that an alert is
	// assigned if no end time is specified.
	resolveTimeout     time.Duration
	alertmanagerConfig *config.Config
	route              *dispatch.Route

	logger log.Logger

	Handler http.Handler
}

type getAlertStatusFn func(prometheus_model.Fingerprint) types.AlertStatus

// NewAPI returns a new Alertmanager API v2
func NewAPI(alerts provider.Alerts, sf getAlertStatusFn, silences *silence.Silences, peer *cluster.Peer, l log.Logger) (*API, error) {
	api := API{
		alerts:         alerts,
		getAlertStatus: sf,
		peer:           peer,
		silences:       silences,
		logger:         l,
		uptime:         time.Now(),
	}

	// load embedded swagger file
	swaggerSpec, err := loads.Analyzed(restapi.SwaggerJSON, "")
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded swagger file: %v", err.Error())
	}

	// create new service API
	openAPI := operations.NewAlertmanagerAPI(swaggerSpec)

	// Skip swagger spec and redoc middleware, only serving the API itself via
	// RoutesHandler. See: https://github.com/go-swagger/go-swagger/issues/1779
	openAPI.Middleware = func(b middleware.Builder) http.Handler {
		return middleware.Spec("", nil, openAPI.Context().RoutesHandler(b))
	}

	openAPI.AlertGetAlertsHandler = alert_ops.GetAlertsHandlerFunc(api.getAlertsHandler)
	openAPI.AlertPostAlertsHandler = alert_ops.PostAlertsHandlerFunc(api.postAlertsHandler)
	openAPI.GeneralGetStatusHandler = general_ops.GetStatusHandlerFunc(api.getStatusHandler)
	openAPI.ReceiverGetReceiversHandler = receiver_ops.GetReceiversHandlerFunc(api.getReceiversHandler)
	openAPI.SilenceDeleteSilenceHandler = silence_ops.DeleteSilenceHandlerFunc(api.deleteSilenceHandler)
	openAPI.SilenceGetSilenceHandler = silence_ops.GetSilenceHandlerFunc(api.getSilenceHandler)
	openAPI.SilenceGetSilencesHandler = silence_ops.GetSilencesHandlerFunc(api.getSilencesHandler)
	openAPI.SilencePostSilencesHandler = silence_ops.PostSilencesHandlerFunc(api.postSilencesHandler)

	openAPI.Logger = func(s string, i ...interface{}) { level.Error(api.logger).Log(i...) }

	handleCORS := cors.Default().Handler
	api.Handler = handleCORS(openAPI.Serve(nil))

	return &api, nil
}

// Update sets the configuration string to a new value.
func (api *API) Update(cfg *config.Config, resolveTimeout time.Duration) error {
	api.mtx.Lock()
	defer api.mtx.Unlock()

	api.resolveTimeout = resolveTimeout
	api.alertmanagerConfig = cfg
	api.route = dispatch.NewRoute(cfg.Route, nil)
	return nil
}

func (api *API) getStatusHandler(params general_ops.GetStatusParams) middleware.Responder {
	api.mtx.RLock()
	defer api.mtx.RUnlock()

	original := api.alertmanagerConfig.String()
	uptime := strfmt.DateTime(api.uptime)

	name := ""
	status := open_api_models.ClusterStatusStatusDisabled

	resp := open_api_models.AlertmanagerStatus{
		Uptime: &uptime,
		VersionInfo: &open_api_models.VersionInfo{
			Version:   &version.Version,
			Revision:  &version.Revision,
			Branch:    &version.Branch,
			BuildUser: &version.BuildUser,
			BuildDate: &version.BuildDate,
			GoVersion: &version.GoVersion,
		},
		Config: &open_api_models.AlertmanagerConfig{
			Original: &original,
		},
		Cluster: &open_api_models.ClusterStatus{
			Name:   &name,
			Status: &status,
			Peers:  []*open_api_models.PeerStatus{},
		},
	}

	// If alertmanager cluster feature is disabled, then api.peers == nil.
	if api.peer != nil {
		name := api.peer.Name()
		status := api.peer.Status()

		peers := []*open_api_models.PeerStatus{}
		for _, n := range api.peer.Peers() {
			address := n.Address()
			peers = append(peers, &open_api_models.PeerStatus{
				Name:    &n.Name,
				Address: &address,
			})
		}

		resp.Cluster = &open_api_models.ClusterStatus{
			Name:   &name,
			Status: &status,
			Peers:  peers,
		}
	}

	return general_ops.NewGetStatusOK().WithPayload(&resp)
}

func (api *API) getReceiversHandler(params receiver_ops.GetReceiversParams) middleware.Responder {
	api.mtx.RLock()
	defer api.mtx.RUnlock()

	receivers := make([]*open_api_models.Receiver, 0, len(api.alertmanagerConfig.Receivers))
	for _, r := range api.alertmanagerConfig.Receivers {
		receivers = append(receivers, &open_api_models.Receiver{Name: &r.Name})
	}

	return receiver_ops.NewGetReceiversOK().WithPayload(receivers)
}

func (api *API) getAlertsHandler(params alert_ops.GetAlertsParams) middleware.Responder {
	var (
		err            error
		receiverFilter *regexp.Regexp
		// Initialize result slice to prevent api returning `null` when there
		// are no alerts present
		res      = open_api_models.GettableAlerts{}
		matchers = []*labels.Matcher{}
	)

	if params.Filter != nil {
		for _, matcherString := range params.Filter {
			matcher, err := parse.Matcher(matcherString)
			if err != nil {
				level.Error(api.logger).Log("msg", "failed to parse matchers", "err", err)
				return alert_ops.NewGetAlertsBadRequest().WithPayload(err.Error())
			}

			matchers = append(matchers, matcher)
		}
	}

	if params.Receiver != nil {
		receiverFilter, err = regexp.Compile("^(?:" + *params.Receiver + ")$")
		if err != nil {
			return alert_ops.
				NewGetAlertsBadRequest().
				WithPayload(
					fmt.Sprintf("failed to parse receiver param: %v", err.Error()),
				)
		}
	}

	alerts := api.alerts.GetPending()
	defer alerts.Close()

	api.mtx.RLock()
	// TODO(fabxc): enforce a sensible timeout.
	for a := range alerts.Next() {
		if err = alerts.Err(); err != nil {
			break
		}

		routes := api.route.Match(a.Labels)
		receivers := make([]*open_api_models.Receiver, 0, len(routes))
		for _, r := range routes {
			receivers = append(receivers, &open_api_models.Receiver{Name: &r.RouteOpts.Receiver})
		}

		if receiverFilter != nil && !receiversMatchFilter(receivers, receiverFilter) {
			continue
		}

		if !alertMatchesFilterLabels(&a.Alert, matchers) {
			continue
		}

		// Continue if the alert is resolved.
		if !a.Alert.EndsAt.IsZero() && a.Alert.EndsAt.Before(time.Now()) {
			continue
		}

		status := api.getAlertStatus(a.Fingerprint())

		if !*params.Active && status.State == types.AlertStateActive {
			continue
		}

		if !*params.Unprocessed && status.State == types.AlertStateUnprocessed {
			continue
		}

		if !*params.Silenced && len(status.SilencedBy) != 0 {
			continue
		}

		if !*params.Inhibited && len(status.InhibitedBy) != 0 {
			continue
		}

		state := string(status.State)
		startsAt := strfmt.DateTime(a.StartsAt)
		updatedAt := strfmt.DateTime(a.UpdatedAt)
		endsAt := strfmt.DateTime(a.EndsAt)
		fingerprint := a.Fingerprint().String()

		alert := open_api_models.GettableAlert{
			Alert: open_api_models.Alert{
				GeneratorURL: strfmt.URI(a.GeneratorURL),
				Labels:       modelLabelSetToAPILabelSet(a.Labels),
			},
			Annotations: modelLabelSetToAPILabelSet(a.Annotations),
			StartsAt:    &startsAt,
			UpdatedAt:   &updatedAt,
			EndsAt:      &endsAt,
			Fingerprint: &fingerprint,
			Receivers:   receivers,
			Status: &open_api_models.AlertStatus{
				State:       &state,
				SilencedBy:  status.SilencedBy,
				InhibitedBy: status.InhibitedBy,
			},
		}

		if alert.Status.SilencedBy == nil {
			alert.Status.SilencedBy = []string{}
		}

		if alert.Status.InhibitedBy == nil {
			alert.Status.InhibitedBy = []string{}
		}

		res = append(res, &alert)
	}
	api.mtx.RUnlock()

	if err != nil {
		level.Error(api.logger).Log("msg", "failed to get alerts", "err", err)
		return alert_ops.NewGetAlertsInternalServerError().WithPayload(err.Error())
	}
	sort.Slice(res, func(i, j int) bool {
		return *res[i].Fingerprint < *res[j].Fingerprint
	})

	return alert_ops.NewGetAlertsOK().WithPayload(res)
}

func (api *API) postAlertsHandler(params alert_ops.PostAlertsParams) middleware.Responder {
	alerts := openAPIAlertsToAlerts(params.Alerts)
	now := time.Now()

	api.mtx.RLock()
	resolveTimeout := api.resolveTimeout
	api.mtx.RUnlock()

	for _, alert := range alerts {
		alert.UpdatedAt = now

		// Ensure StartsAt is set.
		if alert.StartsAt.IsZero() {
			if alert.EndsAt.IsZero() {
				alert.StartsAt = now
			} else {
				alert.StartsAt = alert.EndsAt
			}
		}
		// If no end time is defined, set a timeout after which an alert
		// is marked resolved if it is not updated.
		if alert.EndsAt.IsZero() {
			alert.Timeout = true
			alert.EndsAt = now.Add(resolveTimeout)
		}
		// TODO: Take care of the metrics endpoint
		// if alert.EndsAt.After(time.Now()) {
		// 	numReceivedAlerts.WithLabelValues("firing").Inc()
		// } else {
		// 	numReceivedAlerts.WithLabelValues("resolved").Inc()
		// }
	}

	// Make a best effort to insert all alerts that are valid.
	var (
		validAlerts    = make([]*types.Alert, 0, len(alerts))
		validationErrs = &types.MultiError{}
	)
	for _, a := range alerts {
		removeEmptyLabels(a.Labels)

		if err := a.Validate(); err != nil {
			validationErrs.Add(err)
			// numInvalidAlerts.Inc()
			continue
		}
		validAlerts = append(validAlerts, a)
	}
	if err := api.alerts.Put(validAlerts...); err != nil {
		level.Error(api.logger).Log("msg", "failed to create alerts", "err", err)
		return alert_ops.NewPostAlertsInternalServerError().WithPayload(err.Error())
	}

	if validationErrs.Len() > 0 {
		level.Error(api.logger).Log("msg", "failed to validate alerts", "err", validationErrs.Error())
		return alert_ops.NewPostAlertsBadRequest().WithPayload(validationErrs.Error())
	}

	return alert_ops.NewPostAlertsOK()
}

func openAPIAlertsToAlerts(apiAlerts open_api_models.PostableAlerts) []*types.Alert {
	alerts := []*types.Alert{}
	for _, apiAlert := range apiAlerts {
		alert := types.Alert{
			Alert: prometheus_model.Alert{
				Labels:       apiLabelSetToModelLabelSet(apiAlert.Labels),
				Annotations:  apiLabelSetToModelLabelSet(apiAlert.Annotations),
				StartsAt:     time.Time(apiAlert.StartsAt),
				EndsAt:       time.Time(apiAlert.EndsAt),
				GeneratorURL: string(apiAlert.GeneratorURL),
			},
		}
		alerts = append(alerts, &alert)
	}

	return alerts
}

func removeEmptyLabels(ls prometheus_model.LabelSet) {
	for k, v := range ls {
		if string(v) == "" {
			delete(ls, k)
		}
	}
}

func modelLabelSetToAPILabelSet(modelLabelSet prometheus_model.LabelSet) open_api_models.LabelSet {
	apiLabelSet := open_api_models.LabelSet{}
	for key, value := range modelLabelSet {
		apiLabelSet[string(key)] = string(value)
	}

	return apiLabelSet
}

func apiLabelSetToModelLabelSet(apiLabelSet open_api_models.LabelSet) prometheus_model.LabelSet {
	modelLabelSet := prometheus_model.LabelSet{}
	for key, value := range apiLabelSet {
		modelLabelSet[prometheus_model.LabelName(key)] = prometheus_model.LabelValue(value)
	}

	return modelLabelSet
}

func receiversMatchFilter(receivers []*open_api_models.Receiver, filter *regexp.Regexp) bool {
	for _, r := range receivers {
		if filter.MatchString(string(*r.Name)) {
			return true
		}
	}

	return false
}

func alertMatchesFilterLabels(a *prometheus_model.Alert, matchers []*labels.Matcher) bool {
	sms := make(map[string]string)
	for name, value := range a.Labels {
		sms[string(name)] = string(value)
	}
	return matchFilterLabels(matchers, sms)
}

func matchFilterLabels(matchers []*labels.Matcher, sms map[string]string) bool {
	for _, m := range matchers {
		v, prs := sms[m.Name]
		switch m.Type {
		case labels.MatchNotRegexp, labels.MatchNotEqual:
			if m.Value == "" && prs {
				continue
			}
			if !m.Matches(v) {
				return false
			}
		default:
			if m.Value == "" && !prs {
				continue
			}
			if !prs || !m.Matches(v) {
				return false
			}
		}
	}

	return true
}

func (api *API) getSilencesHandler(params silence_ops.GetSilencesParams) middleware.Responder {
	matchers := []*labels.Matcher{}
	if params.Filter != nil {
		for _, matcherString := range params.Filter {
			matcher, err := parse.Matcher(matcherString)
			if err != nil {
				level.Error(api.logger).Log("msg", "failed to parse matchers", "err", err)
				return alert_ops.NewGetAlertsBadRequest().WithPayload(err.Error())
			}

			matchers = append(matchers, matcher)
		}
	}

	psils, err := api.silences.Query()
	if err != nil {
		level.Error(api.logger).Log("msg", "failed to get silences", "err", err)
		return silence_ops.NewGetSilencesInternalServerError().WithPayload(err.Error())
	}

	sils := open_api_models.GettableSilences{}
	for _, ps := range psils {
		silence, err := gettableSilenceFromProto(ps)
		if err != nil {
			level.Error(api.logger).Log("msg", "failed to unmarshal silence from proto", "err", err)
			return silence_ops.NewGetSilencesInternalServerError().WithPayload(err.Error())
		}
		if !gettableSilenceMatchesFilterLabels(silence, matchers) {
			continue
		}
		sils = append(sils, &silence)
	}

	return silence_ops.NewGetSilencesOK().WithPayload(sils)
}

func gettableSilenceMatchesFilterLabels(s open_api_models.GettableSilence, matchers []*labels.Matcher) bool {
	sms := make(map[string]string)
	for _, m := range s.Matchers {
		sms[*m.Name] = *m.Value
	}

	return matchFilterLabels(matchers, sms)
}

func (api *API) getSilenceHandler(params silence_ops.GetSilenceParams) middleware.Responder {
	sils, err := api.silences.Query(silence.QIDs(params.SilenceID.String()))
	if err != nil {
		level.Error(api.logger).Log("msg", "failed to get silence by id", "err", err)
		return silence_ops.NewGetSilenceInternalServerError().WithPayload(err.Error())
	}

	if len(sils) == 0 {
		level.Error(api.logger).Log("msg", "failed to find silence", "err", err)
		return silence_ops.NewGetSilenceNotFound()
	}

	sil, err := gettableSilenceFromProto(sils[0])
	if err != nil {
		level.Error(api.logger).Log("msg", "failed to convert unmarshal from proto", "err", err)
		return silence_ops.NewGetSilenceInternalServerError().WithPayload(err.Error())
	}

	return silence_ops.NewGetSilenceOK().WithPayload(&sil)
}

func (api *API) deleteSilenceHandler(params silence_ops.DeleteSilenceParams) middleware.Responder {
	sid := params.SilenceID.String()

	if err := api.silences.Expire(sid); err != nil {
		level.Error(api.logger).Log("msg", "failed to expire silence", "err", err)
		return silence_ops.NewDeleteSilenceInternalServerError().WithPayload(err.Error())
	}
	return silence_ops.NewDeleteSilenceOK()
}

func gettableSilenceFromProto(s *silencepb.Silence) (open_api_models.GettableSilence, error) {
	start := strfmt.DateTime(s.StartsAt)
	end := strfmt.DateTime(s.EndsAt)
	updated := strfmt.DateTime(s.UpdatedAt)
	state := string(types.CalcSilenceState(s.StartsAt, s.EndsAt))
	sil := open_api_models.GettableSilence{
		Silence: open_api_models.Silence{
			StartsAt:  &start,
			EndsAt:    &end,
			Comment:   &s.Comment,
			CreatedBy: &s.CreatedBy,
		},
		ID:        &s.Id,
		UpdatedAt: &updated,
		Status: &open_api_models.SilenceStatus{
			State: &state,
		},
	}

	for _, m := range s.Matchers {
		matcher := &open_api_models.Matcher{
			Name:  &m.Name,
			Value: &m.Pattern,
		}
		switch m.Type {
		case silencepb.Matcher_EQUAL:
			f := false
			matcher.IsRegex = &f
		case silencepb.Matcher_REGEXP:
			t := true
			matcher.IsRegex = &t
		default:
			return sil, fmt.Errorf(
				"unknown matcher type for matcher '%v' in silence '%v'",
				m.Name,
				s.Id,
			)
		}
		sil.Matchers = append(sil.Matchers, matcher)
	}

	return sil, nil
}

func (api *API) postSilencesHandler(params silence_ops.PostSilencesParams) middleware.Responder {

	sil, err := postableSilenceToProto(params.Silence)
	if err != nil {
		level.Error(api.logger).Log("msg", "failed to marshal silence to proto", "err", err)
		return silence_ops.NewPostSilencesBadRequest().WithPayload(
			fmt.Sprintf("failed to convert API silence to internal silence: %v", err.Error()),
		)
	}

	if sil.StartsAt.After(sil.EndsAt) || sil.StartsAt.Equal(sil.EndsAt) {
		msg := "failed to create silence: start time must be equal or after end time"
		level.Error(api.logger).Log("msg", msg, "err", err)
		return silence_ops.NewPostSilencesBadRequest().WithPayload(msg)
	}

	if sil.EndsAt.Before(time.Now()) {
		msg := "failed to create silence: end time can't be in the past"
		level.Error(api.logger).Log("msg", msg, "err", err)
		return silence_ops.NewPostSilencesBadRequest().WithPayload(msg)
	}

	sid, err := api.silences.Set(sil)
	if err != nil {
		level.Error(api.logger).Log("msg", "failed to create silence", "err", err)
		return silence_ops.NewPostSilencesBadRequest().WithPayload(err.Error())
	}

	return silence_ops.NewPostSilencesOK().WithPayload(&silence_ops.PostSilencesOKBody{
		SilenceID: sid,
	})
}

func postableSilenceToProto(s *open_api_models.PostableSilence) (*silencepb.Silence, error) {
	sil := &silencepb.Silence{
		Id:        s.ID,
		StartsAt:  time.Time(*s.StartsAt),
		EndsAt:    time.Time(*s.EndsAt),
		Comment:   *s.Comment,
		CreatedBy: *s.CreatedBy,
	}
	for _, m := range s.Matchers {
		matcher := &silencepb.Matcher{
			Name:    *m.Name,
			Pattern: *m.Value,
			Type:    silencepb.Matcher_EQUAL,
		}
		if *m.IsRegex {
			matcher.Type = silencepb.Matcher_REGEXP
		}
		sil.Matchers = append(sil.Matchers, matcher)
	}
	return sil, nil
}
