// Copyright 2025 Prometheus Team
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

package htmx

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/version"

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
)

//go:embed all:templates all:static
var embedFS embed.FS

// Handlers provides HTTP handlers for HTMX UI
type Handlers struct {
	alerts          provider.Alerts
	silences        *silence.Silences
	alertStatusFunc func(model.Fingerprint) types.AlertStatus
	alertGroupsFunc func(context.Context, func(*dispatch.Route) bool, func(*types.Alert, time.Time) bool) (dispatch.AlertGroups, map[model.Fingerprint][]string, error)
	peer            cluster.ClusterPeer
	config          string
	logger          *slog.Logger
	templates       *template.Template
	version         string
}

// NewHandlers creates HTMX UI handlers
func NewHandlers(
	alerts provider.Alerts,
	silences *silence.Silences,
	alertStatusFunc func(model.Fingerprint) types.AlertStatus,
	alertGroupsFunc func(context.Context, func(*dispatch.Route) bool, func(*types.Alert, time.Time) bool) (dispatch.AlertGroups, map[model.Fingerprint][]string, error),
	peer cluster.ClusterPeer,
	configString string,
	logger *slog.Logger,
) (*Handlers, error) {
	// Verify embedded static files exist
	logger.Info("verifying embedded static files")
	staticFiles, err := embedFS.ReadDir("static")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded static directory: %w", err)
	}
	logger.Info("embedded static files", "count", len(staticFiles))
	for _, file := range staticFiles {
		logger.Info("embedded static file", "name", file.Name(), "is_dir", file.IsDir())
	}

	tmpl, err := template.ParseFS(embedFS, "templates/**/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	logger.Info("HTMX handlers initialized successfully")

	return &Handlers{
		alerts:          alerts,
		silences:        silences,
		alertStatusFunc: alertStatusFunc,
		alertGroupsFunc: alertGroupsFunc,
		peer:            peer,
		config:          configString,
		logger:          logger,
		templates:       tmpl,
		version:         version.Version,
	}, nil
}

// Register registers all HTMX UI routes
func (h *Handlers) Register(r *route.Router) {
	// Serve static assets
	r.Get("/static/*filepath", h.serveStatic)

	// Full page handlers
	r.Get("/", h.alertsPage)
	r.Get("/silences", h.silencesPage)
	r.Get("/status", h.statusPage)

	// HTMX partial handlers
	r.Get("/htmx/alerts", h.alertsPartial)
	r.Get("/htmx/silences", h.silencesPartial)
	r.Get("/htmx/silences/form", h.silenceFormPartial)
	r.Post("/htmx/silences", h.createSilence)
	r.Post("/htmx/silences/:id/expire", h.deleteSilence)  // Use POST instead of DELETE
}

// serveStatic serves embedded static files
func (h *Handlers) serveStatic(w http.ResponseWriter, req *http.Request) {
	filepath := route.Param(req.Context(), "filepath")

	// Clean the filepath - remove leading slash if present
	filepath = strings.TrimPrefix(filepath, "/")

	fullPath := "static/" + filepath
	h.logger.Info("serving static file", "requested_path", req.URL.Path, "filepath_param", filepath, "embed_path", fullPath)

	// Security: prevent directory traversal
	if strings.Contains(filepath, "..") {
		h.logger.Warn("blocked directory traversal attempt", "filepath", filepath)
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	content, err := embedFS.ReadFile(fullPath)
	if err != nil {
		h.logger.Error("failed to read static file", "embed_path", fullPath, "error", err)

		// List available files for debugging
		h.logger.Error("embedded filesystem contents:")
		embedFS.ReadDir("static")

		http.NotFound(w, req)
		return
	}

	// Set content type based on extension
	if strings.HasSuffix(filepath, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	} else if strings.HasSuffix(filepath, ".css") {
		w.Header().Set("Content-Type", "text/css")
	} else if strings.HasSuffix(filepath, ".ico") {
		w.Header().Set("Content-Type", "image/x-icon")
	}

	// Set caching headers for static assets
	w.Header().Set("Cache-Control", "public, max-age=3600")

	h.logger.Debug("successfully served static file", "filepath", filepath, "size", len(content))
	w.Write(content)
}

// Template data structures
type PageData struct {
	Title      string
	ActivePage string
	Version    string
	Error      string
}

type AlertsPageData struct {
	PageData
	AlertGroups []*models.AlertGroup
	Filters     AlertFilters
	ExpandAll   bool
	Receivers   []string
	Receiver    string
	Filter      string
}

type AlertFilters struct {
	Active    bool
	Silenced  bool
	Inhibited bool
	Muted     bool
	Receiver  string
	Filter    string
}

type SilencesPageData struct {
	PageData
	Silences []*models.GettableSilence
	State    string
	Filter   string
}

type StatusPageData struct {
	PageData
	Status *models.AlertmanagerStatus
}

// alertsPage renders the full alerts page
func (h *Handlers) alertsPage(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// Default to showing only active alerts
	alertGroups, err := h.fetchAlertGroups(ctx, AlertFilters{Active: true})
	if err != nil {
		h.logger.Error("failed to fetch alert groups", "error", err)
		h.renderError(w, "Failed to fetch alert groups", http.StatusInternalServerError)
		return
	}

	// Get available receivers
	receivers := h.getReceivers()

	data := AlertsPageData{
		PageData: PageData{
			Title:      "Alerts",
			ActivePage: "alerts",
			Version:    h.version,
		},
		AlertGroups: alertGroups,
		Filters:     AlertFilters{Active: true},
		ExpandAll:   false,
		Receivers:   receivers,
		Receiver:    "",
		Filter:      "",
	}

	if err := h.templates.ExecuteTemplate(w, "base.tmpl", data); err != nil {
		h.logger.Error("failed to render template", "error", err)
	}
}

// getReceivers returns a list of available receivers
func (h *Handlers) getReceivers() []string {
	// This is a simplified implementation
	// In a real implementation, you'd get this from the config or API
	return []string{}
}

// alertsPartial renders just the alert list fragment
func (h *Handlers) alertsPartial(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// Parse filters from query params
	filters := AlertFilters{
		Active:    req.URL.Query().Get("active") == "true",
		Silenced:  req.URL.Query().Get("silenced") == "true",
		Inhibited: req.URL.Query().Get("inhibited") == "true",
		Muted:     req.URL.Query().Get("muted") == "true",
		Receiver:  req.URL.Query().Get("receiver"),
		Filter:    req.URL.Query().Get("filter"),
	}

	// If no filters selected, default to active
	if !filters.Active && !filters.Silenced && !filters.Inhibited && !filters.Muted {
		filters.Active = true
	}

	alertGroups, err := h.fetchAlertGroups(ctx, filters)
	if err != nil {
		h.logger.Error("failed to fetch alert groups", "error", err)
		http.Error(w, "Failed to fetch alert groups", http.StatusInternalServerError)
		return
	}

	// Get available receivers
	receivers := h.getReceivers()

	data := AlertsPageData{
		AlertGroups: alertGroups,
		Filters:     filters,
		ExpandAll:   req.URL.Query().Get("expandAll") == "true",
		Receivers:   receivers,
		Receiver:    filters.Receiver,
		Filter:      filters.Filter,
	}

	if err := h.templates.ExecuteTemplate(w, "alert_list", data); err != nil {
		h.logger.Error("failed to render template", "error", err)
	}
}

// fetchAlertGroups gets alert groups from the API
func (h *Handlers) fetchAlertGroups(ctx context.Context, filters AlertFilters) ([]*models.AlertGroup, error) {
	// Convert string filters to label matchers
	var matchers []*labels.Matcher
	if filters.Filter != "" {
		var err error
		matchers, err = parseMatchers(filters.Filter)
		if err != nil {
			h.logger.Warn("failed to parse matchers", "error", err, "filter", filters.Filter)
			// Continue with empty matchers if parsing fails
		}
	}

	// Create route filter function
	routeFilter := func(r *dispatch.Route) bool {
		if filters.Receiver == "" {
			return true
		}
		return r.RouteOpts.Receiver == filters.Receiver
	}

	// Create alert filter function
	alertFilter := func(a *types.Alert, now time.Time) bool {
		// Set alert's current status based on its label set
		h.alertStatusFunc(a.Fingerprint())

		// Get alert's current status after seeing if it is suppressed
		status := h.alertStatusFunc(a.Fingerprint())

		// Apply state filters
		if !filters.Active && status.State == types.AlertStateActive {
			return false
		}

		if !filters.Silenced && len(status.SilencedBy) != 0 {
			return false
		}

		if !filters.Inhibited && len(status.InhibitedBy) != 0 {
			return false
		}

		// Apply matcher filters
		for _, matcher := range matchers {
			value, ok := a.Labels[model.LabelName(matcher.Name)]
			if !ok {
				if matcher.Type == labels.MatchNotEqual || matcher.Type == labels.MatchNotRegexp {
					continue
				}
				return false
			}
			if !matcher.Matches(string(value)) {
				return false
			}
		}

		return true
	}

	// Call the alert groups function
	alertGroups, _, err := h.alertGroupsFunc(ctx, routeFilter, alertFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to get alert groups: %w", err)
	}

	// Convert to API models
	result := make([]*models.AlertGroup, 0, len(alertGroups))
	
	for _, group := range alertGroups {
		// Apply mute filter
		_, isMuted := h.getMuteStatus(group.RouteID, group.GroupKey)
		if !filters.Muted && isMuted {
			continue
		}

		// Convert alerts in the group
		apiAlerts := make([]*models.GettableAlert, 0, len(group.Alerts))
		for _, alert := range group.Alerts {
			fp := alert.Fingerprint()
			status := h.alertStatusFunc(fp)
			apiAlert := &models.GettableAlert{
				Annotations: convertLabelsToLabelSet(alert.Annotations),
				StartsAt:    (*strfmt.DateTime)(&alert.StartsAt),
				EndsAt:      (*strfmt.DateTime)(&alert.EndsAt),
				UpdatedAt:   (*strfmt.DateTime)(&alert.UpdatedAt),
				Fingerprint: swag.String(fp.String()),
				Status: &models.AlertStatus{
					State:       swag.String(string(status.State)),
					InhibitedBy: status.InhibitedBy,
					SilencedBy:  status.SilencedBy,
				},
				Receivers:   make([]*models.Receiver, 0),
			}
			// Set embedded Alert fields
			apiAlert.Labels = convertLabelsToLabelSet(alert.Labels)
			apiAlert.GeneratorURL = strfmt.URI(alert.GeneratorURL)
			apiAlerts = append(apiAlerts, apiAlert)
		}

		// Create the alert group model
		apiGroup := &models.AlertGroup{
			Labels:   convertLabelsToLabelSet(group.Labels),
			Receiver: &models.Receiver{Name: &group.Receiver},
			Alerts:   apiAlerts,
		}
		
		result = append(result, apiGroup)
	}

	return result, nil
}

// getMuteStatus determines if a group is muted
func (h *Handlers) getMuteStatus(routeID, groupKey string) ([]string, bool) {
	// This is a simplified implementation
	// In a real implementation, you'd check the group mute status
	return []string{}, false
}

// silencesPage renders the full silences page
func (h *Handlers) silencesPage(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	silences, err := h.fetchSilences(ctx)
	if err != nil {
		h.logger.Error("failed to fetch silences", "error", err)
		h.renderError(w, "Failed to fetch silences", http.StatusInternalServerError)
		return
	}

	data := SilencesPageData{
		PageData: PageData{
			Title:      "Silences",
			ActivePage: "silences",
			Version:    h.version,
		},
		Silences: silences,
		State:    "active",
		Filter:   "",
	}

	if err := h.templates.ExecuteTemplate(w, "base.tmpl", data); err != nil {
		h.logger.Error("failed to render template", "error", err)
	}
}

// silencesPartial renders just the silence list fragment
func (h *Handlers) silencesPartial(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// Get filter parameters
	state := req.URL.Query().Get("state")
	if state == "" {
		state = "active"
	}
	
	filter := req.URL.Query().Get("filter")

	silences, err := h.fetchSilences(ctx)
	if err != nil {
		h.logger.Error("failed to fetch silences", "error", err)
		http.Error(w, "Failed to fetch silences", http.StatusInternalServerError)
		return
	}

	// Filter silences by state
	filteredSilences := make([]*models.GettableSilence, 0)
	for _, silence := range silences {
		if silence.Status == nil || silence.Status.State == nil {
			continue
		}
		
		// Apply state filter
		if state != "" && *silence.Status.State != state {
			continue
		}
		
		// Apply label filter if provided
		if filter != "" {
			matchers, err := parseMatchers(filter)
			if err != nil {
				h.logger.Warn("failed to parse silence filter", "error", err, "filter", filter)
				// Continue with unfiltered results if parsing fails
			} else {
				// Check if silence matches the filter
				matches := true
				for _, matcher := range matchers {
					// For simplicity, we'll match against the silence comment and creator
					// A more sophisticated implementation would match against silence matchers
					if silence.Silence.Comment != nil && !matcher.Matches(*silence.Silence.Comment) {
						if matcher.Type == labels.MatchEqual || matcher.Type == labels.MatchRegexp {
							matches = false
							break
						}
					}
				}
				if !matches {
					continue
				}
			}
		}
		
		filteredSilences = append(filteredSilences, silence)
	}

	data := SilencesPageData{
		Silences: filteredSilences,
		State:    state,
		Filter:   filter,
	}

	if err := h.templates.ExecuteTemplate(w, "silence_list", data); err != nil {
		h.logger.Error("failed to render template", "error", err)
	}
}

// silenceFormPartial renders the silence creation form
func (h *Handlers) silenceFormPartial(w http.ResponseWriter, req *http.Request) {
	if err := h.templates.ExecuteTemplate(w, "silence_form", nil); err != nil {
		h.logger.Error("failed to render template", "error", err)
	}
}

// createSilence handles silence creation
func (h *Handlers) createSilence(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	if err := req.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Parse matchers from textarea (one per line)
	matcherLines := strings.Split(req.FormValue("matchers"), "\n")
	matchers := make([]*silencepb.Matcher, 0)

	for _, line := range matcherLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse matcher: label=value or label=~regex or label!=value or label!~regex
		matcher, err := parseMatcher(line)
		if err != nil {
			http.Error(w, "Invalid matcher: "+line+" ("+err.Error()+")", http.StatusBadRequest)
			return
		}
		matchers = append(matchers, matcher)
	}

	if len(matchers) == 0 {
		http.Error(w, "At least one matcher is required", http.StatusBadRequest)
		return
	}

	// Parse timestamps
	startsAt, err := time.Parse("2006-01-02T15:04", req.FormValue("startsAt"))
	if err != nil {
		http.Error(w, "Invalid start time: "+err.Error(), http.StatusBadRequest)
		return
	}

	endsAt, err := time.Parse("2006-01-02T15:04", req.FormValue("endsAt"))
	if err != nil {
		http.Error(w, "Invalid end time: "+err.Error(), http.StatusBadRequest)
		return
	}

	if endsAt.Before(startsAt) {
		http.Error(w, "End time must be after start time", http.StatusBadRequest)
		return
	}

	createdBy := strings.TrimSpace(req.FormValue("createdBy"))
	comment := strings.TrimSpace(req.FormValue("comment"))

	if createdBy == "" {
		http.Error(w, "Created by is required", http.StatusBadRequest)
		return
	}

	if comment == "" {
		http.Error(w, "Comment is required", http.StatusBadRequest)
		return
	}

	// Create silence using protobuf format
	silence := &silencepb.Silence{
		Matchers:  matchers,
		StartsAt:  startsAt,
		EndsAt:    endsAt,
		CreatedBy: createdBy,
		Comment:   comment,
	}

	if err := h.silences.Set(ctx, silence); err != nil {
		h.logger.Error("failed to create silence", "error", err)
		http.Error(w, "Failed to create silence: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logger.Info("created silence", "id", silence.Id, "created_by", createdBy)

	// Return updated silence list
	h.silencesPartial(w, req)
}

// deleteSilence handles silence deletion
func (h *Handlers) deleteSilence(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	silenceID := route.Param(ctx, "id")

	if err := h.silences.Expire(ctx, silenceID); err != nil {
		h.logger.Error("failed to expire silence", "error", err)
		http.Error(w, "Failed to expire silence: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logger.Info("expired silence", "id", silenceID)

	// Return updated silence list
	h.silencesPartial(w, req)
}

// statusPage renders the status page
func (h *Handlers) statusPage(w http.ResponseWriter, req *http.Request) {
	status := h.fetchStatus()

	data := StatusPageData{
		PageData: PageData{
			Title:      "Status",
			ActivePage: "status",
			Version:    h.version,
		},
		Status: status,
	}

	if err := h.templates.ExecuteTemplate(w, "base.tmpl", data); err != nil {
		h.logger.Error("failed to render template", "error", err)
	}
}

// Helper functions

func (h *Handlers) renderError(w http.ResponseWriter, message string, code int) {
	w.WriteHeader(code)
	data := PageData{
		Title:   "Error",
		Version: h.version,
		Error:   message,
	}
	if err := h.templates.ExecuteTemplate(w, "base.tmpl", data); err != nil {
		h.logger.Error("failed to render error template", "error", err)
		// Fallback to simple error response
		http.Error(w, message, code)
	}
}

func (h *Handlers) fetchSilences(ctx context.Context) ([]*models.GettableSilence, error) {
	// Get all silences using Query method
	pbSilences, _, err := h.silences.Query(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query silences: %w", err)
	}

	result := make([]*models.GettableSilence, 0, len(pbSilences))

	for _, pbSil := range pbSilences {
		startsAt := strfmt.DateTime(pbSil.StartsAt)
		endsAt := strfmt.DateTime(pbSil.EndsAt)
		updatedAt := strfmt.DateTime(pbSil.UpdatedAt)

		gettableSilence := &models.GettableSilence{
			ID:        swag.String(pbSil.Id),
			UpdatedAt: &updatedAt,
			Status: &models.SilenceStatus{
				State: swag.String(string(types.CalcSilenceState(pbSil.StartsAt, pbSil.EndsAt))),
			},
			Silence: models.Silence{
				Matchers:  convertPBMatchersToModels(pbSil.Matchers),
				StartsAt:  &startsAt,
				EndsAt:    &endsAt,
				CreatedBy: swag.String(pbSil.CreatedBy),
				Comment:   swag.String(pbSil.Comment),
			},
		}

		result = append(result, gettableSilence)
	}

	return result, nil
}

func (h *Handlers) fetchStatus() *models.AlertmanagerStatus {
	status := &models.AlertmanagerStatus{
		VersionInfo: &models.VersionInfo{
			Version:   swag.String(version.Version),
			Revision:  swag.String(version.Revision),
			Branch:    swag.String(version.Branch),
			BuildDate: swag.String(version.BuildDate),
			GoVersion: swag.String(version.GoVersion),
			BuildUser: swag.String(version.BuildUser),
		},
		Config: &models.AlertmanagerConfig{
			Original: swag.String(h.config),
		},
	}

	if h.peer != nil {
		peerStatus := h.peer.Status()
		peers := h.peer.Peers()
		peerModels := make([]*models.PeerStatus, 0, len(peers))

		for _, p := range peers {
			peerModels = append(peerModels, &models.PeerStatus{
				Name:    swag.String(p.Name()),
				Address: swag.String(p.Address()),
			})
		}

		status.Cluster = &models.ClusterStatus{
			Status: swag.String(peerStatus),
			Name:   h.peer.Name(),
			Peers:  peerModels,
		}
	}

	return status
}

// Conversion helpers

func convertLabelsToLabelSet(labels model.LabelSet) models.LabelSet {
	result := make(models.LabelSet, len(labels))
	for k, v := range labels {
		result[string(k)] = string(v)
	}
	return result
}

func convertPBMatchersToModels(matchers []*silencepb.Matcher) models.Matchers {
	result := make(models.Matchers, len(matchers))
	for i, m := range matchers {
		result[i] = &models.Matcher{
			Name:    swag.String(m.Name),
			Value:   swag.String(m.Pattern),
			IsRegex: swag.Bool(m.Type == silencepb.Matcher_REGEXP || m.Type == silencepb.Matcher_NOT_REGEXP),
			IsEqual: swag.Bool(m.Type == silencepb.Matcher_EQUAL || m.Type == silencepb.Matcher_REGEXP),
		}
	}
	return result
}

// parseMatchers parses a string of matchers into a slice of label matchers
func parseMatchers(filter string) ([]*labels.Matcher, error) {
	if filter == "" {
		return nil, nil
	}

	filter = strings.TrimSpace(filter)
	if strings.HasPrefix(filter, "{") && strings.HasSuffix(filter, "}") {
		filter = filter[1 : len(filter)-1]
	}

	matcherStrings := strings.Split(filter, ",")
	matchers := make([]*labels.Matcher, 0, len(matcherStrings))

	for _, matcherStr := range matcherStrings {
		matcherStr = strings.TrimSpace(matcherStr)
		if matcherStr == "" {
			continue
		}

		matcher, err := labels.ParseMatcher(matcherStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse matcher %q: %w", matcherStr, err)
		}
		matchers = append(matchers, matcher)
	}

	return matchers, nil
}

func parseMatcher(line string) (*silencepb.Matcher, error) {
	// Parse matcher format: label=value or label=~regex or label!=value or label!~regex
	var name, value string
	var matcherType silencepb.Matcher_Type

	// Try different matcher formats
	if strings.Contains(line, "=~") {
		parts := strings.SplitN(line, "=~", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid regex matcher format")
		}
		name = strings.TrimSpace(parts[0])
		value = strings.TrimSpace(parts[1])
		matcherType = silencepb.Matcher_REGEXP
	} else if strings.Contains(line, "!~") {
		parts := strings.SplitN(line, "!~", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid negative regex matcher format")
		}
		name = strings.TrimSpace(parts[0])
		value = strings.TrimSpace(parts[1])
		matcherType = silencepb.Matcher_NOT_REGEXP
	} else if strings.Contains(line, "!=") {
		parts := strings.SplitN(line, "!=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid negative matcher format")
		}
		name = strings.TrimSpace(parts[0])
		value = strings.TrimSpace(parts[1])
		matcherType = silencepb.Matcher_NOT_EQUAL
	} else if strings.Contains(line, "=") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid matcher format")
		}
		name = strings.TrimSpace(parts[0])
		value = strings.TrimSpace(parts[1])
		matcherType = silencepb.Matcher_EQUAL
	} else {
		return nil, fmt.Errorf("matcher must contain =, !=, =~, or !~")
	}

	if name == "" || value == "" {
		return nil, fmt.Errorf("label name and value cannot be empty")
	}

	// Remove quotes if present
	value = strings.Trim(value, `"'`)

	return &silencepb.Matcher{
		Name:    name,
		Pattern: value,
		Type:    matcherType,
	}, nil
}
