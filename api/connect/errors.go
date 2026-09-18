// Copyright The Prometheus Authors
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

package apiconnect

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	commonv3alpha "github.com/prometheus/alertmanager/api/common/v3alpha"
)

type rpcError struct {
	code    connect.Code
	message string
	cause   error
	details []proto.Message
}

func (e *rpcError) Error() string {
	return e.message
}

func (e *rpcError) Unwrap() error {
	return e.cause
}

func newRPCError(code connect.Code, message string, cause error, details ...proto.Message) error {
	return &rpcError{code: code, message: message, cause: cause, details: details}
}

func invalidArgumentError(message string, violations ...*commonv3alpha.FieldViolation) error {
	if len(violations) == 0 {
		return newRPCError(connect.CodeInvalidArgument, message, nil)
	}
	return newRPCError(connect.CodeInvalidArgument, message, nil, &commonv3alpha.ValidationErrorDetail{Violations: violations})
}

func requiredFeatureError(feature, description string) error {
	return newRPCError(
		connect.CodeFailedPrecondition,
		description,
		nil,
		&commonv3alpha.RequiredFeatureDetail{Feature: feature, Description: description},
	)
}

func partialResultError(code connect.Code, message string, detail *commonv3alpha.PartialResultDetail) error {
	if detail == nil {
		return newRPCError(code, message, nil)
	}
	return newRPCError(code, message, nil, detail)
}

func translateRPCError(ctx context.Context, err error) error {
	err = normalizeContextError(ctx, err)
	if err == nil || connect.CodeOf(err) != connect.CodeUnknown {
		return err
	}

	var typed *rpcError
	if !errors.As(err, &typed) {
		return connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	result := connect.NewError(typed.code, errors.New(typed.message))
	for _, message := range typed.details {
		if message == nil {
			continue
		}
		detail, detailErr := connect.NewErrorDetail(message)
		if detailErr != nil {
			return connect.NewError(connect.CodeInternal, errors.New("internal error"))
		}
		result.AddDetail(detail)
	}
	return result
}
