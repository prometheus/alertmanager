package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
)

type API struct {
	alerts   provider.Alerts
	silences provider.Silences
	// context is an indirection for testing.
	context func(r *http.Request) context.Context
}

func NewAPI(r *route.Router, alerts provider.Alerts, silences provider.Silences) *API {
	api := &API{
		context:  route.Context,
		alerts:   alerts,
		silences: silences,
	}

	r.Get("/alerts", api.listAlerts)
	r.Post("/alerts", api.addAlerts)

	r.Get("/silences", api.listSilences)
	r.Post("/silences", api.addSilence)

	r.Get("/silence/:sid", api.getSilence)
	r.Put("/silence/:sid", api.setSilence)
	r.Del("/silence/:sid", api.delSilence)

	return api
}

type errorType string

const (
	errorNone     errorType = ""
	errorTimeout            = "timeout"
	errorCanceled           = "canceled"
	errorBadData            = "bad_data"
)

type apiError struct {
	typ errorType
	err error
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s: %s", e.typ, e.err)
}

func (api *API) listAlerts(w http.ResponseWriter, r *http.Request) {
	// 	alerts, err := api.alerts.GetAll()
	// 	if err != nil {
	// 		respondError(w, apiError{
	// 			typ: errorBadData,
	// 			err: err,
	// 		}, nil)
	// 		return
	// 	}
	// 	respond(w, alerts)
}

func (api *API) addAlerts(w http.ResponseWriter, r *http.Request) {
	var alerts []*types.Alert
	if err := receive(r, &alerts); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, alert := range alerts {
		now := time.Now()

		alert.UpdatedAt = now

		if alert.StartsAt.IsZero() {
			alert.StartsAt = now
		}
		if alert.EndsAt.IsZero() {
			alert.EndsAt = alert.StartsAt.Add(ResolveTimeout)
		}
	}

	// TODO(fabxc): validate input.
	if err := api.alerts.Put(alerts...); err != nil {
		respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
		return
	}

	respond(w, nil)
}

func (api *API) addSilence(w http.ResponseWriter, r *http.Request) {
	var sil types.Silence
	if err := receive(r, &sil); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// TODO(fabxc): validate input.
	if err := api.silences.Set(&sil); err != nil {
		respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
		return
	}

	respond(w, nil)
}

func (api *API) getSilence(w http.ResponseWriter, r *http.Request) {
	sids := route.Param(api.context(r), "sid")
	sid, err := strconv.ParseUint(sids, 10, 64)
	if err != nil {
		respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
	}

	sil, err := api.silences.Get(model.Fingerprint(sid))
	if err != nil {
		http.Error(w, fmt.Sprint("Error getting silence: ", err), http.StatusNotFound)
		return
	}

	respond(w, &sil)
}

func (api *API) setSilence(w http.ResponseWriter, r *http.Request) {
	var sil types.Silence
	if err := receive(r, &sil); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sids := route.Param(api.context(r), "sid")
	sid, err := strconv.ParseUint(sids, 10, 64)
	if err != nil {
		respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
	}
	sil.ID = model.Fingerprint(sid)

	if err := api.silences.Set(&sil); err != nil {
		respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, &sil)
		return
	}
	respond(w, nil)
}

func (api *API) delSilence(w http.ResponseWriter, r *http.Request) {
	sids := route.Param(api.context(r), "sid")
	sid, err := strconv.ParseUint(sids, 10, 64)
	if err != nil {
		respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
	}

	if err := api.silences.Del(model.Fingerprint(sid)); err != nil {
		respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
		return
	}
	respond(w, nil)
}

func (api *API) listSilences(w http.ResponseWriter, r *http.Request) {
	sils, err := api.silences.All()
	if err != nil {
		respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, nil)
		return
	}
	respond(w, sils)
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
		return
	}
	w.Write(b)
}

func respondError(w http.ResponseWriter, apiErr apiError, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(422)

	b, err := json.Marshal(&response{
		Status:    statusError,
		ErrorType: apiErr.typ,
		Error:     apiErr.err.Error(),
		Data:      data,
	})
	if err != nil {
		return
	}
	w.Write(b)
}

func receive(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	return dec.Decode(v)
}
