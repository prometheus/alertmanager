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
	"io"
	"net/http"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/validate"

	"github.com/coatico/alertmanager/api/v2/models"
)

// NewPostAlertsParams creates a new PostAlertsParams object
//
// There are no default values defined in the spec.
func NewPostAlertsParams() PostAlertsParams {

	return PostAlertsParams{}
}

// PostAlertsParams contains all the bound params for the post alerts operation
// typically these are obtained from a http.Request
//
// swagger:parameters postAlerts
type PostAlertsParams struct {

	// HTTP Request Object
	HTTPRequest *http.Request `json:"-"`

	/*The alerts to create
	  Required: true
	  In: body
	*/
	Alerts models.PostableAlerts
}

// BindRequest both binds and validates a request, it assumes that complex things implement a Validatable(strfmt.Registry) error interface
// for simple values it will use straight method calls.
//
// To ensure default values, the struct must have been initialized with NewPostAlertsParams() beforehand.
func (o *PostAlertsParams) BindRequest(r *http.Request, route *middleware.MatchedRoute) error {
	var res []error

	o.HTTPRequest = r

	if runtime.HasBody(r) {
		defer r.Body.Close()
		var body models.PostableAlerts
		if err := route.Consumer.Consume(r.Body, &body); err != nil {
			if err == io.EOF {
				res = append(res, errors.Required("alerts", "body", ""))
			} else {
				res = append(res, errors.NewParseError("alerts", "body", "", err))
			}
		} else {
			// validate body object
			if err := body.Validate(route.Formats); err != nil {
				res = append(res, err)
			}

			ctx := validate.WithOperationRequest(r.Context())
			if err := body.ContextValidate(ctx, route.Formats); err != nil {
				res = append(res, err)
			}

			if len(res) == 0 {
				o.Alerts = body
			}
		}
	} else {
		res = append(res, errors.Required("alerts", "body", ""))
	}
	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
