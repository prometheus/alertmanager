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
	"fmt"
	"log/slog"
	"strings"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/proto"
)

func newValidationInterceptor(logger *slog.Logger, requests ...proto.Message) connect.UnaryInterceptorFunc {
	validator, err := protovalidate.New(protovalidate.WithMessages(requests...))
	if err != nil {
		panic(fmt.Errorf("create Connect request validator: %w", err))
	}
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
			if err := validator.Validate(request.Any().(proto.Message)); err != nil {
				return nil, validationError(ctx, logger, err)
			}
			return next(ctx, request)
		}
	}
}

func validationError(ctx context.Context, logger *slog.Logger, err error) error {
	var validationErr *protovalidate.ValidationError
	if !errors.As(err, &validationErr) {
		return internalError(ctx, logger, "failed to validate request", err)
	}
	violations := make([]*errdetails.BadRequest_FieldViolation, 0, len(validationErr.Violations))
	for _, violation := range validationErr.Violations {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field:       protovalidate.FieldPathString(violation.Proto.GetField()),
			Description: violation.Proto.GetMessage(),
			Reason:      strings.ToUpper(strings.ReplaceAll(violation.Proto.GetRuleId(), ".", "_")),
		})
	}
	detail, err := connect.NewErrorDetail(&errdetails.BadRequest{FieldViolations: violations})
	if err != nil {
		return internalError(ctx, logger, "failed to encode validation details", err)
	}
	result := connect.NewError(connect.CodeInvalidArgument, errors.New("invalid request"))
	result.AddDetail(detail)
	return result
}
