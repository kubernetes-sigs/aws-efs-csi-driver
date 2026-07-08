/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloud

import (
	"context"
	"errors"
	"time"

	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	smithy "github.com/aws/smithy-go"
	smithymiddleware "github.com/aws/smithy-go/middleware"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/metrics"
	"k8s.io/klog/v2"
)

// RecordRequestsMiddleware instruments EFS and S3 Files API calls with Prometheus metrics.
// It is a no-op when the metrics recorder is not initialized.
func RecordRequestsMiddleware(service string) func(*smithymiddleware.Stack) error {
	return func(stack *smithymiddleware.Stack) error {
		return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc("RecordRequestsMiddleware",
			func(ctx context.Context, input smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
				start := time.Now()
				output, metadata, err := next.HandleFinalize(ctx, input)
				duration := time.Since(start).Seconds()
				labels := createLabels(ctx, service)
				metrics.Recorder().ObserveHistogram(metrics.APIRequestDuration, metrics.APIRequestDurationHelpText, duration, labels, nil)
				if err != nil {
					var apiErr smithy.APIError
					if errors.As(err, &apiErr) {
						if _, isThrottle := retry.DefaultThrottleErrorCodes[apiErr.ErrorCode()]; isThrottle {
							metrics.Recorder().IncreaseCount(metrics.APIRequestThrottles, metrics.APIRequestThrottlesHelpText, labels)
						} else {
							labels["code"] = apiErr.ErrorCode()
							metrics.Recorder().IncreaseCount(metrics.APIRequestErrors, metrics.APIRequestErrorsHelpText, labels)
						}
					}
				}
				return output, metadata, err
			}), smithymiddleware.After)
	}
}

// LogServerErrorsMiddleware logs errors from the EFS and S3 Files APIs.
// Throttle errors are logged at high verbosity since they are expected under load.
// This middleware should always be added last so it sees the unmodified error.
func LogServerErrorsMiddleware() func(*smithymiddleware.Stack) error {
	return func(stack *smithymiddleware.Stack) error {
		return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc("LogServerErrorsMiddleware",
			func(ctx context.Context, input smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
				output, metadata, err := next.HandleFinalize(ctx, input)
				if err != nil {
					var apiErr smithy.APIError
					if errors.As(err, &apiErr) {
						if _, isThrottle := retry.DefaultThrottleErrorCodes[apiErr.ErrorCode()]; isThrottle {
							klog.V(4).ErrorS(apiErr, "Throttle error from AWS EFS API")
						} else {
							klog.ErrorS(apiErr, "Error from AWS EFS API")
						}
					} else {
						klog.ErrorS(err, "Unknown error attempting to contact AWS EFS API")
					}
				}
				return output, metadata, err
			}), smithymiddleware.After)
	}
}

func createLabels(ctx context.Context, service string) map[string]string {
	op := awsmiddleware.GetOperationName(ctx)
	if op == "" {
		op = "Unknown"
	}
	return map[string]string{"request": op, "service": service}
}
