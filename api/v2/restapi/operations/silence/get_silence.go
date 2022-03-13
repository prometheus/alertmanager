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

package silence

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the generate command

import (
	"net/http"

	"github.com/go-openapi/runtime/middleware"
)

// GetSilenceHandlerFunc turns a function with the right signature into a get silence handler
type GetSilenceHandlerFunc func(GetSilenceParams) middleware.Responder

// Handle executing the request and returning a response
func (fn GetSilenceHandlerFunc) Handle(params GetSilenceParams) middleware.Responder {
	return fn(params)
}

// GetSilenceHandler interface for that can handle valid get silence params
type GetSilenceHandler interface {
	Handle(GetSilenceParams) middleware.Responder
}

// NewGetSilence creates a new http.Handler for the get silence operation
func NewGetSilence(ctx *middleware.Context, handler GetSilenceHandler) *GetSilence {
	return &GetSilence{Context: ctx, Handler: handler}
}

/* GetSilence swagger:route GET /silence/{silenceID} silence getSilence

Get a silence by its ID

*/
type GetSilence struct {
	Context *middleware.Context
	Handler GetSilenceHandler
}

func (o *GetSilence) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	route, rCtx, _ := o.Context.RouteInfo(r)
	if rCtx != nil {
		*r = *rCtx
	}
	var Params = NewGetSilenceParams()
	if err := o.Context.BindValidRequest(r, route, &Params); err != nil { // bind params
		o.Context.Respond(rw, r, route.Produces, route, err)
		return
	}

	res := o.Handler.Handle(Params) // actually handle the request
	o.Context.Respond(rw, r, route.Produces, route, res)

}
