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

package alertgroup

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"net/http"

	"github.com/go-openapi/runtime"

	"github.com/coatico/alertmanager/api/v2/models"
)

// GetAlertGroupsOKCode is the HTTP code returned for type GetAlertGroupsOK
const GetAlertGroupsOKCode int = 200

/*
GetAlertGroupsOK Get alert groups response

swagger:response getAlertGroupsOK
*/
type GetAlertGroupsOK struct {

	/*
	  In: Body
	*/
	Payload models.AlertGroups `json:"body,omitempty"`
}

// NewGetAlertGroupsOK creates GetAlertGroupsOK with default headers values
func NewGetAlertGroupsOK() *GetAlertGroupsOK {

	return &GetAlertGroupsOK{}
}

// WithPayload adds the payload to the get alert groups o k response
func (o *GetAlertGroupsOK) WithPayload(payload models.AlertGroups) *GetAlertGroupsOK {
	o.Payload = payload
	return o
}

// SetPayload sets the payload to the get alert groups o k response
func (o *GetAlertGroupsOK) SetPayload(payload models.AlertGroups) {
	o.Payload = payload
}

// WriteResponse to the client
func (o *GetAlertGroupsOK) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {

	rw.WriteHeader(200)
	payload := o.Payload
	if payload == nil {
		// return empty array
		payload = models.AlertGroups{}
	}

	if err := producer.Produce(rw, payload); err != nil {
		panic(err) // let the recovery middleware deal with this
	}
}

// GetAlertGroupsBadRequestCode is the HTTP code returned for type GetAlertGroupsBadRequest
const GetAlertGroupsBadRequestCode int = 400

/*
GetAlertGroupsBadRequest Bad request

swagger:response getAlertGroupsBadRequest
*/
type GetAlertGroupsBadRequest struct {

	/*
	  In: Body
	*/
	Payload string `json:"body,omitempty"`
}

// NewGetAlertGroupsBadRequest creates GetAlertGroupsBadRequest with default headers values
func NewGetAlertGroupsBadRequest() *GetAlertGroupsBadRequest {

	return &GetAlertGroupsBadRequest{}
}

// WithPayload adds the payload to the get alert groups bad request response
func (o *GetAlertGroupsBadRequest) WithPayload(payload string) *GetAlertGroupsBadRequest {
	o.Payload = payload
	return o
}

// SetPayload sets the payload to the get alert groups bad request response
func (o *GetAlertGroupsBadRequest) SetPayload(payload string) {
	o.Payload = payload
}

// WriteResponse to the client
func (o *GetAlertGroupsBadRequest) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {

	rw.WriteHeader(400)
	payload := o.Payload
	if err := producer.Produce(rw, payload); err != nil {
		panic(err) // let the recovery middleware deal with this
	}
}

// GetAlertGroupsInternalServerErrorCode is the HTTP code returned for type GetAlertGroupsInternalServerError
const GetAlertGroupsInternalServerErrorCode int = 500

/*
GetAlertGroupsInternalServerError Internal server error

swagger:response getAlertGroupsInternalServerError
*/
type GetAlertGroupsInternalServerError struct {

	/*
	  In: Body
	*/
	Payload string `json:"body,omitempty"`
}

// NewGetAlertGroupsInternalServerError creates GetAlertGroupsInternalServerError with default headers values
func NewGetAlertGroupsInternalServerError() *GetAlertGroupsInternalServerError {

	return &GetAlertGroupsInternalServerError{}
}

// WithPayload adds the payload to the get alert groups internal server error response
func (o *GetAlertGroupsInternalServerError) WithPayload(payload string) *GetAlertGroupsInternalServerError {
	o.Payload = payload
	return o
}

// SetPayload sets the payload to the get alert groups internal server error response
func (o *GetAlertGroupsInternalServerError) SetPayload(payload string) {
	o.Payload = payload
}

// WriteResponse to the client
func (o *GetAlertGroupsInternalServerError) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {

	rw.WriteHeader(500)
	payload := o.Payload
	if err := producer.Produce(rw, payload); err != nil {
		panic(err) // let the recovery middleware deal with this
	}
}
