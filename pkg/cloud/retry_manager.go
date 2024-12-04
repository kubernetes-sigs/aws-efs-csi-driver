package cloud

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
)

const (
	// retryMaxAttempt sets max number of EFS API call attempts.
	// Set high enough to ensure default sidecar timeout will cancel context long before we stop retrying.
	adaptiveRetryMaxAttempt = 5
)

// retryManager dictates the retry strategies of EFS API calls. Can be configured to use standard sdk retryer or more
// client side rate limited by adaptive mode set via adaptive-retry-mode of controller.
// Each mutating EFS API has its own retryer because the AWS SDK v2 rate limits on a retryer object level, not by API name.
// Separate retryers ensures that throttling one API doesn't unintentionally throttle others with separate token buckets.
type retryManager struct {
	createAccessPointRetryer    aws.Retryer
	deleteAccessPointRetryer    aws.Retryer
	describeAccessPointsRetryer aws.Retryer
	describeMountTargetsRetryer aws.Retryer
	describeFileSystemsRetryer  aws.Retryer
}

func newRetryManager(adaptiveRetryMode bool) *retryManager {
	var rm *retryManager
	if adaptiveRetryMode {
		rm = &retryManager{
			createAccessPointRetryer:    newAdaptiveRetryer(),
			deleteAccessPointRetryer:    newAdaptiveRetryer(),
			describeAccessPointsRetryer: newAdaptiveRetryer(),
			describeMountTargetsRetryer: newAdaptiveRetryer(),
			describeFileSystemsRetryer:  newAdaptiveRetryer(),
		}
	} else {
		rm = &retryManager{
			createAccessPointRetryer:    retry.NewStandard(),
			deleteAccessPointRetryer:    retry.NewStandard(),
			describeAccessPointsRetryer: retry.NewStandard(),
			describeMountTargetsRetryer: retry.NewStandard(),
			describeFileSystemsRetryer:  retry.NewStandard(),
		}
	}
	return rm
}

// AdaptiveRetryer restricts attempts of API calls that recently hit throttle errors.
func newAdaptiveRetryer() *retry.AdaptiveMode {
	return retry.NewAdaptiveMode(func(ao *retry.AdaptiveModeOptions) {
		ao.StandardOptions = append(ao.StandardOptions, func(so *retry.StandardOptions) {
			so.MaxAttempts = adaptiveRetryMaxAttempt
		})
	})
}
