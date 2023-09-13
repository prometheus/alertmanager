// Code generated by go-swagger; DO NOT EDIT.

// Copyright Prometheus Team
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
//

package alert

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"net/http"
	"time"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	cr "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// NewGetAlertsParams creates a new GetAlertsParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewGetAlertsParams() *GetAlertsParams {
	return &GetAlertsParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewGetAlertsParamsWithTimeout creates a new GetAlertsParams object
// with the ability to set a timeout on a request.
func NewGetAlertsParamsWithTimeout(timeout time.Duration) *GetAlertsParams {
	return &GetAlertsParams{
		timeout: timeout,
	}
}

// NewGetAlertsParamsWithContext creates a new GetAlertsParams object
// with the ability to set a context for a request.
func NewGetAlertsParamsWithContext(ctx context.Context) *GetAlertsParams {
	return &GetAlertsParams{
		Context: ctx,
	}
}

// NewGetAlertsParamsWithHTTPClient creates a new GetAlertsParams object
// with the ability to set a custom HTTPClient for a request.
func NewGetAlertsParamsWithHTTPClient(client *http.Client) *GetAlertsParams {
	return &GetAlertsParams{
		HTTPClient: client,
	}
}

/*
GetAlertsParams contains all the parameters to send to the API endpoint

	for the get alerts operation.

	Typically these are written to a http.Request.
*/
type GetAlertsParams struct {

	/* Active.

	   Show active alerts

	   Default: true
	*/
	Active *bool

	/* Filter.

	   A list of matchers to filter alerts by
	*/
	Filter []string

	/* Inhibited.

	   Show inhibited alerts

	   Default: true
	*/
	Inhibited *bool

	/* Muted.

	   Show muted alerts

	   Default: true
	*/
	Muted *bool

	/* Receiver.

	   A regex matching receivers to filter alerts by
	*/
	Receiver *string

	/* Silenced.

	   Show silenced alerts

	   Default: true
	*/
	Silenced *bool

	/* Unprocessed.

	   Show unprocessed alerts

	   Default: true
	*/
	Unprocessed *bool

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the get alerts params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *GetAlertsParams) WithDefaults() *GetAlertsParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the get alerts params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *GetAlertsParams) SetDefaults() {
	var (
		activeDefault = bool(true)

		inhibitedDefault = bool(true)

		mutedDefault = bool(true)

		silencedDefault = bool(true)

		unprocessedDefault = bool(true)
	)

	val := GetAlertsParams{
		Active:      &activeDefault,
		Inhibited:   &inhibitedDefault,
		Muted:       &mutedDefault,
		Silenced:    &silencedDefault,
		Unprocessed: &unprocessedDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the get alerts params
func (o *GetAlertsParams) WithTimeout(timeout time.Duration) *GetAlertsParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get alerts params
func (o *GetAlertsParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get alerts params
func (o *GetAlertsParams) WithContext(ctx context.Context) *GetAlertsParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get alerts params
func (o *GetAlertsParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get alerts params
func (o *GetAlertsParams) WithHTTPClient(client *http.Client) *GetAlertsParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get alerts params
func (o *GetAlertsParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithActive adds the active to the get alerts params
func (o *GetAlertsParams) WithActive(active *bool) *GetAlertsParams {
	o.SetActive(active)
	return o
}

// SetActive adds the active to the get alerts params
func (o *GetAlertsParams) SetActive(active *bool) {
	o.Active = active
}

// WithFilter adds the filter to the get alerts params
func (o *GetAlertsParams) WithFilter(filter []string) *GetAlertsParams {
	o.SetFilter(filter)
	return o
}

// SetFilter adds the filter to the get alerts params
func (o *GetAlertsParams) SetFilter(filter []string) {
	o.Filter = filter
}

// WithInhibited adds the inhibited to the get alerts params
func (o *GetAlertsParams) WithInhibited(inhibited *bool) *GetAlertsParams {
	o.SetInhibited(inhibited)
	return o
}

// SetInhibited adds the inhibited to the get alerts params
func (o *GetAlertsParams) SetInhibited(inhibited *bool) {
	o.Inhibited = inhibited
}

// WithMuted adds the muted to the get alerts params
func (o *GetAlertsParams) WithMuted(muted *bool) *GetAlertsParams {
	o.SetMuted(muted)
	return o
}

// SetMuted adds the muted to the get alerts params
func (o *GetAlertsParams) SetMuted(muted *bool) {
	o.Muted = muted
}

// WithReceiver adds the receiver to the get alerts params
func (o *GetAlertsParams) WithReceiver(receiver *string) *GetAlertsParams {
	o.SetReceiver(receiver)
	return o
}

// SetReceiver adds the receiver to the get alerts params
func (o *GetAlertsParams) SetReceiver(receiver *string) {
	o.Receiver = receiver
}

// WithSilenced adds the silenced to the get alerts params
func (o *GetAlertsParams) WithSilenced(silenced *bool) *GetAlertsParams {
	o.SetSilenced(silenced)
	return o
}

// SetSilenced adds the silenced to the get alerts params
func (o *GetAlertsParams) SetSilenced(silenced *bool) {
	o.Silenced = silenced
}

// WithUnprocessed adds the unprocessed to the get alerts params
func (o *GetAlertsParams) WithUnprocessed(unprocessed *bool) *GetAlertsParams {
	o.SetUnprocessed(unprocessed)
	return o
}

// SetUnprocessed adds the unprocessed to the get alerts params
func (o *GetAlertsParams) SetUnprocessed(unprocessed *bool) {
	o.Unprocessed = unprocessed
}

// WriteToRequest writes these params to a swagger request
func (o *GetAlertsParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.Active != nil {

		// query param active
		var qrActive bool

		if o.Active != nil {
			qrActive = *o.Active
		}
		qActive := swag.FormatBool(qrActive)
		if qActive != "" {

			if err := r.SetQueryParam("active", qActive); err != nil {
				return err
			}
		}
	}

	if o.Filter != nil {

		// binding items for filter
		joinedFilter := o.bindParamFilter(reg)

		// query array param filter
		if err := r.SetQueryParam("filter", joinedFilter...); err != nil {
			return err
		}
	}

	if o.Inhibited != nil {

		// query param inhibited
		var qrInhibited bool

		if o.Inhibited != nil {
			qrInhibited = *o.Inhibited
		}
		qInhibited := swag.FormatBool(qrInhibited)
		if qInhibited != "" {

			if err := r.SetQueryParam("inhibited", qInhibited); err != nil {
				return err
			}
		}
	}

	if o.Muted != nil {

		// query param muted
		var qrMuted bool

		if o.Muted != nil {
			qrMuted = *o.Muted
		}
		qMuted := swag.FormatBool(qrMuted)
		if qMuted != "" {

			if err := r.SetQueryParam("muted", qMuted); err != nil {
				return err
			}
		}
	}

	if o.Receiver != nil {

		// query param receiver
		var qrReceiver string

		if o.Receiver != nil {
			qrReceiver = *o.Receiver
		}
		qReceiver := qrReceiver
		if qReceiver != "" {

			if err := r.SetQueryParam("receiver", qReceiver); err != nil {
				return err
			}
		}
	}

	if o.Silenced != nil {

		// query param silenced
		var qrSilenced bool

		if o.Silenced != nil {
			qrSilenced = *o.Silenced
		}
		qSilenced := swag.FormatBool(qrSilenced)
		if qSilenced != "" {

			if err := r.SetQueryParam("silenced", qSilenced); err != nil {
				return err
			}
		}
	}

	if o.Unprocessed != nil {

		// query param unprocessed
		var qrUnprocessed bool

		if o.Unprocessed != nil {
			qrUnprocessed = *o.Unprocessed
		}
		qUnprocessed := swag.FormatBool(qrUnprocessed)
		if qUnprocessed != "" {

			if err := r.SetQueryParam("unprocessed", qUnprocessed); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamGetAlerts binds the parameter filter
func (o *GetAlertsParams) bindParamFilter(formats strfmt.Registry) []string {
	filterIR := o.Filter

	var filterIC []string
	for _, filterIIR := range filterIR { // explode []string

		filterIIV := filterIIR // string as string
		filterIC = append(filterIC, filterIIV)
	}

	// items.CollectionFormat: "multi"
	filterIS := swag.JoinByFormat(filterIC, "multi")

	return filterIS
}
