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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/go-openapi/analysis"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/prometheus/client_golang/prometheus"
	prometheus_model "github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/rs/cors"

	alertgroupinfolist_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/alertgroupinfolist"

	"github.com/prometheus/alertmanager/api/metrics"
	open_api_models "github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/api/v2/restapi"
	"github.com/prometheus/alertmanager/api/v2/restapi/operations"
	alert_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/alert"
	alertgroup_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/alertgroup"
	alertinfo_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/alertinfo"
	general_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/general"
	receiver_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/receiver"
	silence_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/silence"
	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/matcher/compat"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/alertmanager/util/callback"
)

// API represents an Alertmanager API v2.
type API struct {
	peer            cluster.ClusterPeer
	silences        *silence.Silences
	alerts          provider.Alerts
	alertGroups     groupsFn
	alertGroupInfos groupInfosFn
	getAlertStatus  getAlertStatusFn
	groupMutedFunc  groupMutedFunc
	apiCallback     callback.Callback
	uptime          time.Time

	// mtx protects alertmanagerConfig, setAlertStatus and route.
	mtx sync.RWMutex
	// resolveTimeout represents the default resolve timeout that an alert is
	// assigned if no end time is specified.
	alertmanagerConfig *config.Config
	route              *dispatch.Route
	setAlertStatus     setAlertStatusFn

	logger *slog.Logger
	m      *metrics.Alerts

	Handler http.Handler
}

type (
	groupsFn         func(func(*dispatch.Route) bool, func(*types.Alert, time.Time) bool, func(string) bool) (dispatch.AlertGroups, map[prometheus_model.Fingerprint][]string)
	groupMutedFunc   func(routeID, groupKey string) ([]string, bool)
	groupInfosFn     func(func(*dispatch.Route) bool) dispatch.AlertGroupInfos
	getAlertStatusFn func(prometheus_model.Fingerprint) types.AlertStatus
	setAlertStatusFn func(prometheus_model.LabelSet)
)

// NewAPI returns a new Alertmanager API v2.
func NewAPI(
	alerts provider.Alerts,
	gf groupsFn,
	gif groupInfosFn,
	asf getAlertStatusFn,
	gmf groupMutedFunc,
	silences *silence.Silences,
	apiCallback callback.Callback,
	peer cluster.ClusterPeer,
	l *slog.Logger,
	r prometheus.Registerer,
) (*API, error) {
	if apiCallback == nil {
		apiCallback = callback.NoopAPICallback{}
	}
	api := API{
		alerts:          alerts,
		getAlertStatus:  asf,
		alertGroups:     gf,
		alertGroupInfos: gif,
		groupMutedFunc:  gmf,
		peer:            peer,
		silences:        silences,
		apiCallback:     apiCallback,
		logger:          l,
		m:               metrics.NewAlerts(r),
		uptime:          time.Now(),
	}

	// Load embedded swagger file.
	swaggerSpec, swaggerSpecAnalysis, err := getSwaggerSpec()
	if err != nil {
		return nil, err
	}

	// Create new service API.
	openAPI := operations.NewAlertmanagerAPI(swaggerSpec)

	// Skip the  redoc middleware, only serving the OpenAPI specification and
	// the API itself via RoutesHandler. See:
	// https://github.com/go-swagger/go-swagger/issues/1779
	openAPI.Middleware = func(b middleware.Builder) http.Handler {
		// Manually create the context so that we can use the singleton swaggerSpecAnalysis.
		swaggerContext := middleware.NewRoutableContextWithAnalyzedSpec(swaggerSpec, swaggerSpecAnalysis, openAPI, nil)
		return middleware.Spec("", swaggerSpec.Raw(), swaggerContext.RoutesHandler(b))
	}

	openAPI.AlertGetAlertsHandler = alert_ops.GetAlertsHandlerFunc(api.getAlertsHandler)
	openAPI.AlertinfoGetAlertInfosHandler = alertinfo_ops.GetAlertInfosHandlerFunc(api.getAlertInfosHandler)
	openAPI.AlertPostAlertsHandler = alert_ops.PostAlertsHandlerFunc(api.postAlertsHandler)
	openAPI.AlertgroupGetAlertGroupsHandler = alertgroup_ops.GetAlertGroupsHandlerFunc(api.getAlertGroupsHandler)
	openAPI.AlertgroupinfolistGetAlertGroupInfoListHandler = alertgroupinfolist_ops.GetAlertGroupInfoListHandlerFunc(api.getAlertGroupInfoListHandler)
	openAPI.GeneralGetStatusHandler = general_ops.GetStatusHandlerFunc(api.getStatusHandler)
	openAPI.ReceiverGetReceiversHandler = receiver_ops.GetReceiversHandlerFunc(api.getReceiversHandler)
	openAPI.SilenceDeleteSilenceHandler = silence_ops.DeleteSilenceHandlerFunc(api.deleteSilenceHandler)
	openAPI.SilenceGetSilenceHandler = silence_ops.GetSilenceHandlerFunc(api.getSilenceHandler)
	openAPI.SilenceGetSilencesHandler = silence_ops.GetSilencesHandlerFunc(api.getSilencesHandler)
	openAPI.SilencePostSilencesHandler = silence_ops.PostSilencesHandlerFunc(api.postSilencesHandler)

	handleCORS := cors.Default().Handler
	api.Handler = handleCORS(setResponseHeaders(openAPI.Serve(nil)))

	return &api, nil
}

var responseHeaders = map[string]string{
	"Cache-Control": "no-store",
}

func setResponseHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for h, v := range responseHeaders {
			w.Header().Set(h, v)
		}
		h.ServeHTTP(w, r)
	})
}

func (api *API) requestLogger(req *http.Request) *slog.Logger {
	return api.logger.With("path", req.URL.Path, "method", req.Method)
}

// Update sets the API struct members that may change between reloads of alertmanager.
func (api *API) Update(cfg *config.Config, setAlertStatus setAlertStatusFn) {
	api.mtx.Lock()
	defer api.mtx.Unlock()

	api.alertmanagerConfig = cfg
	api.route = dispatch.NewRoute(cfg.Route, nil)
	api.setAlertStatus = setAlertStatus
}

func (api *API) getStatusHandler(params general_ops.GetStatusParams) middleware.Responder {
	api.mtx.RLock()
	defer api.mtx.RUnlock()

	original := api.alertmanagerConfig.String()
	uptime := strfmt.DateTime(api.uptime)

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
			Status: &status,
			Peers:  []*open_api_models.PeerStatus{},
		},
	}

	// If alertmanager cluster feature is disabled, then api.peers == nil.
	if api.peer != nil {
		status := api.peer.Status()

		peers := []*open_api_models.PeerStatus{}
		for _, n := range api.peer.Peers() {
			address := n.Address()
			name := n.Name()
			peers = append(peers, &open_api_models.PeerStatus{
				Name:    &name,
				Address: &address,
			})
		}

		sort.Slice(peers, func(i, j int) bool {
			return *peers[i].Name < *peers[j].Name
		})

		resp.Cluster = &open_api_models.ClusterStatus{
			Name:   api.peer.Name(),
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
	for i := range api.alertmanagerConfig.Receivers {
		receivers = append(receivers, &open_api_models.Receiver{Name: &api.alertmanagerConfig.Receivers[i].Name})
	}

	return receiver_ops.NewGetReceiversOK().WithPayload(receivers)
}

func (api *API) getAlertsHandler(params alert_ops.GetAlertsParams) middleware.Responder {
	var (
		receiverFilter *regexp.Regexp
		ctx            = params.HTTPRequest.Context()

		logger = api.requestLogger(params.HTTPRequest)
	)

	matchers, err := parseFilter(params.Filter)
	if err != nil {
		logger.Debug("Failed to parse matchers", "err", err)
		return alertgroup_ops.NewGetAlertGroupsBadRequest().WithPayload(err.Error())
	}

	if params.Receiver != nil {
		receiverFilter, err = regexp.Compile("^(?:" + *params.Receiver + ")$")
		if err != nil {
			logger.Debug("Failed to compile receiver regex", "err", err)
			return alert_ops.
				NewGetAlertsBadRequest().
				WithPayload(
					fmt.Sprintf("failed to parse receiver param: %v", err.Error()),
				)
		}
	}

	alertFilter := api.alertFilter(matchers, *params.Silenced, *params.Inhibited, *params.Active)
	alerts, err := api.getAlerts(ctx, receiverFilter, alertFilter)
	if err != nil {
		logger.Error("Failed to get alerts", "err", err)
		return alert_ops.NewGetAlertsInternalServerError().WithPayload(err.Error())
	}

	callbackRes, err := api.apiCallback.V2GetAlertsCallback(alerts)
	if err != nil {
		logger.Error("Failed to call api callback", "err", err)
		return alert_ops.NewGetAlertsInternalServerError().WithPayload(err.Error())
	}

	return alert_ops.NewGetAlertsOK().WithPayload(callbackRes)
}

func (api *API) getAlertInfosHandler(params alertinfo_ops.GetAlertInfosParams) middleware.Responder {
	var (
		alerts         open_api_models.GettableAlerts
		receiverFilter *regexp.Regexp
		ctx            = params.HTTPRequest.Context()

		logger = api.requestLogger(params.HTTPRequest)
	)

	matchers, err := parseFilter(params.Filter)
	if err != nil {
		logger.Debug("Failed to parse matchers", "err", err)
		return alertinfo_ops.NewGetAlertInfosBadRequest().WithPayload(err.Error())
	}

	if params.Receiver != nil {
		receiverFilter, err = regexp.Compile("^(?:" + *params.Receiver + ")$")
		if err != nil {
			logger.Debug("Failed to compile receiver regex", "err", err)
			return alertinfo_ops.
				NewGetAlertInfosBadRequest().
				WithPayload(
					fmt.Sprintf("failed to parse receiver param: %v", err.Error()),
				)
		}
	}

	if err = validateMaxResult(params.MaxResults); err != nil {
		logger.Error("Failed to parse MaxResults parameter", "err", err)
		return alertinfo_ops.
			NewGetAlertInfosBadRequest().
			WithPayload(
				fmt.Sprintf("failed to parse MaxResults param: %v", *params.MaxResults),
			)
	}

	if err = validateAlertInfoNextToken(params.NextToken); err != nil {
		logger.Error("Failed to parse NextToken parameter", "err", err)
		return alertinfo_ops.
			NewGetAlertInfosBadRequest().
			WithPayload(
				fmt.Sprintf("failed to parse NextToken param: %v", *params.NextToken),
			)
	}

	alertFilter := api.alertFilter(matchers, *params.Silenced, *params.Inhibited, *params.Active)
	groupIdsFilter := api.groupIDFilter(params.GroupID)
	if len(params.GroupID) > 0 {
		alerts, err = api.getAlertsFromAlertGroup(ctx, receiverFilter, alertFilter, groupIdsFilter)
	} else {
		alerts, err = api.getAlerts(ctx, receiverFilter, alertFilter)
	}
	if err != nil {
		logger.Error("Failed to get alerts", "err", err)
		return alertinfo_ops.NewGetAlertInfosInternalServerError().WithPayload(err.Error())
	}

	returnAlertInfos, nextItem := AlertInfosTruncate(alerts, params.MaxResults, params.NextToken)

	response := &open_api_models.GettableAlertInfos{
		Alerts:    returnAlertInfos,
		NextToken: nextItem,
	}

	return alertinfo_ops.NewGetAlertInfosOK().WithPayload(response)
}

func (api *API) postAlertsHandler(params alert_ops.PostAlertsParams) middleware.Responder {
	logger := api.requestLogger(params.HTTPRequest)

	alerts := OpenAPIAlertsToAlerts(params.Alerts)
	now := time.Now()

	api.mtx.RLock()
	resolveTimeout := time.Duration(api.alertmanagerConfig.Global.ResolveTimeout)
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
		if alert.EndsAt.After(time.Now()) {
			api.m.Firing().Inc()
		} else {
			api.m.Resolved().Inc()
		}
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
			api.m.Invalid().Inc()
			continue
		}
		validAlerts = append(validAlerts, a)
	}
	if err := api.alerts.Put(validAlerts...); err != nil {
		logger.Error("Failed to create alerts", "err", err)
		return alert_ops.NewPostAlertsInternalServerError().WithPayload(err.Error())
	}

	if validationErrs.Len() > 0 {
		logger.Error("Failed to validate alerts", "err", validationErrs.Error())
		return alert_ops.NewPostAlertsBadRequest().WithPayload(validationErrs.Error())
	}

	return alert_ops.NewPostAlertsOK()
}

func (api *API) getAlertGroupsHandler(params alertgroup_ops.GetAlertGroupsParams) middleware.Responder {
	logger := api.requestLogger(params.HTTPRequest)

	matchers, err := parseFilter(params.Filter)
	if err != nil {
		logger.Debug("Failed to parse matchers", "err", err)
		return alertgroup_ops.NewGetAlertGroupsBadRequest().WithPayload(err.Error())
	}

	var receiverFilter *regexp.Regexp
	if params.Receiver != nil {
		receiverFilter, err = regexp.Compile("^(?:" + *params.Receiver + ")$")
		if err != nil {
			logger.Error("Failed to compile receiver regex", "err", err)
			return alertgroup_ops.
				NewGetAlertGroupsBadRequest().
				WithPayload(
					fmt.Sprintf("failed to parse receiver param: %v", err.Error()),
				)
		}
	}

	rf := api.routeFilter(receiverFilter)
	af := api.alertFilter(matchers, *params.Silenced, *params.Inhibited, *params.Active)
	gf := api.groupIDFilter([]string{})
	alertGroups, allReceivers := api.alertGroups(rf, af, gf)

	res := make(open_api_models.AlertGroups, 0, len(alertGroups))

	for _, alertGroup := range alertGroups {
		mutedBy, isMuted := api.groupMutedFunc(alertGroup.RouteID, alertGroup.GroupKey)
		if !*params.Muted && isMuted {
			continue
		}

		ag := &open_api_models.AlertGroup{
			Receiver: &open_api_models.Receiver{Name: &alertGroup.Receiver},
			Labels:   ModelLabelSetToAPILabelSet(alertGroup.Labels),
			Alerts:   make([]*open_api_models.GettableAlert, 0, len(alertGroup.Alerts)),
		}

		for _, alert := range alertGroup.Alerts {
			fp := alert.Fingerprint()
			receivers := allReceivers[fp]
			status := api.getAlertStatus(fp)
			apiAlert := AlertToOpenAPIAlert(alert, status, receivers, mutedBy)
			ag.Alerts = append(ag.Alerts, apiAlert)
		}
		res = append(res, ag)
	}

	callbackRes, err := api.apiCallback.V2GetAlertGroupsCallback(res)
	if err != nil {
		logger.Error("Failed to call api callback", "err", err)
		return alertgroup_ops.NewGetAlertGroupsInternalServerError().WithPayload(err.Error())
	}

	return alertgroup_ops.NewGetAlertGroupsOK().WithPayload(callbackRes)
}

func (api *API) getAlertGroupInfoListHandler(params alertgroupinfolist_ops.GetAlertGroupInfoListParams) middleware.Responder {
	logger := api.requestLogger(params.HTTPRequest)

	var receiverFilter *regexp.Regexp
	var err error
	if params.Receiver != nil {
		receiverFilter, err = regexp.Compile("^(?:" + *params.Receiver + ")$")
		if err != nil {
			logger.Error("Failed to compile receiver regex", "err", err)
			return alertgroupinfolist_ops.
				NewGetAlertGroupInfoListBadRequest().
				WithPayload(
					fmt.Sprintf("failed to parse receiver param: %v", err.Error()),
				)
		}
	}

	rf := func(receiverFilter *regexp.Regexp) func(r *dispatch.Route) bool {
		return func(r *dispatch.Route) bool {
			receiver := r.RouteOpts.Receiver
			if receiverFilter != nil && !receiverFilter.MatchString(receiver) {
				return false
			}
			return true
		}
	}(receiverFilter)

	if err = validateNextToken(params.NextToken); err != nil {
		logger.Error("Failed to parse NextToken parameter", "err", err)
		return alertgroupinfolist_ops.
			NewGetAlertGroupInfoListBadRequest().
			WithPayload(
				fmt.Sprintf("failed to parse NextToken param: %v", *params.NextToken),
			)
	}

	if err = validateMaxResult(params.MaxResults); err != nil {
		logger.Error("Failed to parse MaxResults parameter", "err", err)
		return alertgroupinfolist_ops.
			NewGetAlertGroupInfoListBadRequest().
			WithPayload(
				fmt.Sprintf("failed to parse MaxResults param: %v", *params.MaxResults),
			)
	}

	ags := api.alertGroupInfos(rf)
	alertGroupInfos := make([]*open_api_models.AlertGroupInfo, 0, len(ags))
	for _, alertGroup := range ags {

		// Skip the aggregation group if the next token is set and hasn't arrived the nextToken item yet.
		if params.NextToken != nil && *params.NextToken >= alertGroup.ID {
			continue
		}

		ag := &open_api_models.AlertGroupInfo{
			Receiver: &open_api_models.Receiver{Name: &alertGroup.Receiver},
			Labels:   ModelLabelSetToAPILabelSet(alertGroup.Labels),
			ID:       &alertGroup.ID,
		}
		alertGroupInfos = append(alertGroupInfos, ag)
	}

	returnAlertGroupInfos, nextItem := AlertGroupInfoListTruncate(alertGroupInfos, params.MaxResults)

	response := &open_api_models.AlertGroupInfoList{
		AlertGroupInfoList: returnAlertGroupInfos,
		NextToken:          nextItem,
	}

	return alertgroupinfolist_ops.NewGetAlertGroupInfoListOK().WithPayload(response)
}

func (api *API) groupIDFilter(groupIDsFilter []string) func(groupId string) bool {
	return func(groupId string) bool {
		if len(groupIDsFilter) <= 0 {
			return true
		}
		for _, groupIDFilter := range groupIDsFilter {
			if groupIDFilter == groupId {
				return true
			}
		}
		return false
	}
}

func (api *API) routeFilter(receiverFilter *regexp.Regexp) func(r *dispatch.Route) bool {
	return func(receiverFilter *regexp.Regexp) func(r *dispatch.Route) bool {
		return func(r *dispatch.Route) bool {
			receiver := r.RouteOpts.Receiver
			if receiverFilter != nil && !receiverFilter.MatchString(receiver) {
				return false
			}
			return true
		}
	}(receiverFilter)
}

func (api *API) alertFilter(matchers []*labels.Matcher, silenced, inhibited, active bool) func(a *types.Alert, now time.Time) bool {
	return func(a *types.Alert, now time.Time) bool {
		if !a.EndsAt.IsZero() && a.EndsAt.Before(now) {
			return false
		}

		// Set alert's current status based on its label set.
		api.setAlertStatus(a.Labels)

		// Get alert's current status after seeing if it is suppressed.
		status := api.getAlertStatus(a.Fingerprint())

		if !active && status.State == types.AlertStateActive {
			return false
		}

		if !silenced && len(status.SilencedBy) != 0 {
			return false
		}

		if !inhibited && len(status.InhibitedBy) != 0 {
			return false
		}

		return alertMatchesFilterLabels(&a.Alert, matchers)
	}
}

func removeEmptyLabels(ls prometheus_model.LabelSet) {
	for k, v := range ls {
		if string(v) == "" {
			delete(ls, k)
		}
	}
}

func receiversMatchFilter(receivers []string, filter *regexp.Regexp) bool {
	for _, r := range receivers {
		if filter.MatchString(r) {
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
			if !m.Matches(v) {
				return false
			}
		}
	}

	return true
}

func (api *API) getSilencesHandler(params silence_ops.GetSilencesParams) middleware.Responder {
	logger := api.requestLogger(params.HTTPRequest)

	matchers, err := parseFilter(params.Filter)
	if err != nil {
		logger.Debug("Failed to parse matchers", "err", err)
		return silence_ops.NewGetSilencesBadRequest().WithPayload(err.Error())
	}

	psils, _, err := api.silences.Query()
	if err != nil {
		logger.Error("Failed to get silences", "err", err)
		return silence_ops.NewGetSilencesInternalServerError().WithPayload(err.Error())
	}

	sils := open_api_models.GettableSilences{}
	for _, ps := range psils {
		if !CheckSilenceMatchesFilterLabels(ps, matchers) {
			continue
		}
		silence, err := GettableSilenceFromProto(ps)
		if err != nil {
			logger.Error("Failed to unmarshal silence from proto", "err", err)
			return silence_ops.NewGetSilencesInternalServerError().WithPayload(err.Error())
		}
		sils = append(sils, &silence)
	}

	SortSilences(sils)

	return silence_ops.NewGetSilencesOK().WithPayload(sils)
}

var silenceStateOrder = map[types.SilenceState]int{
	types.SilenceStateActive:  1,
	types.SilenceStatePending: 2,
	types.SilenceStateExpired: 3,
}

// SortSilences sorts first according to the state "active, pending, expired"
// then by end time or start time depending on the state.
// Active silences should show the next to expire first
// pending silences are ordered based on which one starts next
// expired are ordered based on which one expired most recently.
func SortSilences(sils open_api_models.GettableSilences) {
	sort.Slice(sils, func(i, j int) bool {
		state1 := types.SilenceState(*sils[i].Status.State)
		state2 := types.SilenceState(*sils[j].Status.State)
		if state1 != state2 {
			return silenceStateOrder[state1] < silenceStateOrder[state2]
		}
		switch state1 {
		case types.SilenceStateActive:
			endsAt1 := time.Time(*sils[i].Silence.EndsAt)
			endsAt2 := time.Time(*sils[j].Silence.EndsAt)
			return endsAt1.Before(endsAt2)
		case types.SilenceStatePending:
			startsAt1 := time.Time(*sils[i].Silence.StartsAt)
			startsAt2 := time.Time(*sils[j].Silence.StartsAt)
			return startsAt1.Before(startsAt2)
		case types.SilenceStateExpired:
			endsAt1 := time.Time(*sils[i].Silence.EndsAt)
			endsAt2 := time.Time(*sils[j].Silence.EndsAt)
			return endsAt1.After(endsAt2)
		}
		return false
	})
}

// CheckSilenceMatchesFilterLabels returns true if
// a given silence matches a list of matchers.
// A silence matches a filter (list of matchers) if
// for all matchers in the filter, there exists a matcher in the silence
// such that their names, types, and values are equivalent.
func CheckSilenceMatchesFilterLabels(s *silencepb.Silence, matchers []*labels.Matcher) bool {
	for _, matcher := range matchers {
		found := false
		for _, m := range s.Matchers {
			if matcher.Name == m.Name &&
				(matcher.Type == labels.MatchEqual && m.Type == silencepb.Matcher_EQUAL ||
					matcher.Type == labels.MatchRegexp && m.Type == silencepb.Matcher_REGEXP ||
					matcher.Type == labels.MatchNotEqual && m.Type == silencepb.Matcher_NOT_EQUAL ||
					matcher.Type == labels.MatchNotRegexp && m.Type == silencepb.Matcher_NOT_REGEXP) &&
				matcher.Value == m.Pattern {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (api *API) getSilenceHandler(params silence_ops.GetSilenceParams) middleware.Responder {
	logger := api.requestLogger(params.HTTPRequest)

	sils, _, err := api.silences.Query(silence.QIDs(params.SilenceID.String()))
	if err != nil {
		logger.Error("Failed to get silence by id", "err", err, "id", params.SilenceID.String())
		return silence_ops.NewGetSilenceInternalServerError().WithPayload(err.Error())
	}

	if len(sils) == 0 {
		logger.Error("Failed to find silence", "err", err, "id", params.SilenceID.String())
		return silence_ops.NewGetSilenceNotFound()
	}

	sil, err := GettableSilenceFromProto(sils[0])
	if err != nil {
		logger.Error("Failed to convert unmarshal from proto", "err", err)
		return silence_ops.NewGetSilenceInternalServerError().WithPayload(err.Error())
	}

	return silence_ops.NewGetSilenceOK().WithPayload(&sil)
}

func (api *API) deleteSilenceHandler(params silence_ops.DeleteSilenceParams) middleware.Responder {
	logger := api.requestLogger(params.HTTPRequest)

	sid := params.SilenceID.String()
	if err := api.silences.Expire(sid); err != nil {
		logger.Error("Failed to expire silence", "err", err)
		if errors.Is(err, silence.ErrNotFound) {
			return silence_ops.NewDeleteSilenceNotFound()
		}
		return silence_ops.NewDeleteSilenceInternalServerError().WithPayload(err.Error())
	}
	return silence_ops.NewDeleteSilenceOK()
}

func (api *API) postSilencesHandler(params silence_ops.PostSilencesParams) middleware.Responder {
	logger := api.requestLogger(params.HTTPRequest)

	sil, err := PostableSilenceToProto(params.Silence)
	if err != nil {
		logger.Error("Failed to marshal silence to proto", "err", err)
		return silence_ops.NewPostSilencesBadRequest().WithPayload(
			fmt.Sprintf("failed to convert API silence to internal silence: %v", err.Error()),
		)
	}

	if sil.StartsAt.After(sil.EndsAt) || sil.StartsAt.Equal(sil.EndsAt) {
		msg := "Failed to create silence: start time must be before end time"
		logger.Error(msg, "starts_at", sil.StartsAt, "ends_at", sil.EndsAt)
		return silence_ops.NewPostSilencesBadRequest().WithPayload(msg)
	}

	if sil.EndsAt.Before(time.Now()) {
		msg := "Failed to create silence: end time can't be in the past"
		logger.Error(msg, "ends_at", sil.EndsAt)
		return silence_ops.NewPostSilencesBadRequest().WithPayload(msg)
	}

	if err = api.silences.Set(sil); err != nil {
		logger.Error("Failed to create silence", "err", err)
		if errors.Is(err, silence.ErrNotFound) {
			return silence_ops.NewPostSilencesNotFound().WithPayload(err.Error())
		}
		return silence_ops.NewPostSilencesBadRequest().WithPayload(err.Error())
	}

	return silence_ops.NewPostSilencesOK().WithPayload(&silence_ops.PostSilencesOKBody{
		SilenceID: sil.Id,
	})
}

func parseFilter(filter []string) ([]*labels.Matcher, error) {
	matchers := make([]*labels.Matcher, 0, len(filter))
	for _, matcherString := range filter {
		matcher, err := compat.Matcher(matcherString, "api")
		if err != nil {
			return nil, err
		}

		matchers = append(matchers, matcher)
	}
	return matchers, nil
}

var (
	swaggerSpecCacheMx       sync.Mutex
	swaggerSpecCache         *loads.Document
	swaggerSpecAnalysisCache *analysis.Spec
)

// getSwaggerSpec loads and caches the swagger spec. If a cached version already exists,
// it returns the cached one. The reason why we cache it is because some downstream projects
// (e.g. Grafana Mimir) creates many Alertmanager instances in the same process, so they would
// incur in a significant memory penalty if we would reload the swagger spec each time.
func getSwaggerSpec() (*loads.Document, *analysis.Spec, error) {
	swaggerSpecCacheMx.Lock()
	defer swaggerSpecCacheMx.Unlock()

	// Check if a cached version exists.
	if swaggerSpecCache != nil {
		return swaggerSpecCache, swaggerSpecAnalysisCache, nil
	}

	// Load embedded swagger file.
	swaggerSpec, err := loads.Analyzed(restapi.SwaggerJSON, "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load embedded swagger file: %w", err)
	}

	swaggerSpecCache = swaggerSpec
	swaggerSpecAnalysisCache = analysis.New(swaggerSpec.Spec())
	return swaggerSpec, swaggerSpecAnalysisCache, nil
}

func (api *API) getAlertsFromAlertGroup(ctx context.Context, receiverFilter *regexp.Regexp, alertFilter func(a *types.Alert, now time.Time) bool, groupIdsFilter func(groupId string) bool) (open_api_models.GettableAlerts, error) {
	res := open_api_models.GettableAlerts{}
	routeFilter := api.routeFilter(receiverFilter)
	alertGroups, allReceivers := api.alertGroups(routeFilter, alertFilter, groupIdsFilter)
	for _, alertGroup := range alertGroups {
		for _, alert := range alertGroup.Alerts {
			if err := ctx.Err(); err != nil {
				break
			}
			fp := alert.Fingerprint()
			receivers := allReceivers[fp]
			status := api.getAlertStatus(fp)
			apiAlert := AlertToOpenAPIAlert(alert, status, receivers)
			res = append(res, apiAlert)
		}
	}

	sort.Slice(res, func(i, j int) bool {
		return *res[i].Fingerprint < *res[j].Fingerprint
	})

	return res, nil
}

func (api *API) getAlerts(ctx context.Context, receiverFilter *regexp.Regexp, alertFilter func(a *types.Alert, now time.Time) bool) (open_api_models.GettableAlerts, error) {
	var err error
	res := open_api_models.GettableAlerts{}

	alerts := api.alerts.GetPending()
	defer alerts.Close()

	now := time.Now()

	api.mtx.RLock()
	for a := range alerts.Next() {
		if err = alerts.Err(); err != nil {
			break
		}
		if err = ctx.Err(); err != nil {
			break
		}

		routes := api.route.Match(a.Labels)
		receivers := make([]string, 0, len(routes))
		for _, r := range routes {
			receivers = append(receivers, r.RouteOpts.Receiver)
		}

		if receiverFilter != nil && !receiversMatchFilter(receivers, receiverFilter) {
			continue
		}

		if !alertFilter(a, now) {
			continue
		}

		alert := AlertToOpenAPIAlert(a, api.getAlertStatus(a.Fingerprint()), receivers, nil)

		res = append(res, alert)
	}
	api.mtx.RUnlock()

	if err != nil {
		return res, err
	}
	sort.Slice(res, func(i, j int) bool {
		return *res[i].Fingerprint < *res[j].Fingerprint
	})

	return res, nil
}

func validateMaxResult(maxItem *int64) error {
	if maxItem != nil {
		if *maxItem < 0 {
			return errors.New("the maxItem need to be larger than or equal to 0")
		}
	}
	return nil
}

func validateAlertInfoNextToken(nextToken *string) error {
	if nextToken != nil {
		_, err := prometheus_model.ParseFingerprint(*nextToken)
		if err != nil {
			return err
		}
	}
	return nil
}

func validateNextToken(nextToken *string) error {
	if nextToken != nil {
		match, _ := regexp.MatchString("^[a-fA-F0-9]{40}$", *nextToken)
		if !match {
			return fmt.Errorf("invalid nextToken: %s", *nextToken)
		}
	}
	return nil
}
