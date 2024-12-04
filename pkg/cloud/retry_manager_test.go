package cloud

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
)

func TestRetryManagerInitialization(t *testing.T) {
	testCases := []struct {
		name              string
		adaptiveRetryMode bool
		expectedRetryer   string
	}{
		{
			name:              "Adaptive mode enabled",
			adaptiveRetryMode: true,
			expectedRetryer:   "adaptive",
		},
		{
			name:              "Standard mode enabled",
			adaptiveRetryMode: false,
			expectedRetryer:   "standard",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rm := newRetryManager(tc.adaptiveRetryMode)

			checkRetryer := func(name string, retryer aws.Retryer) {
				switch tc.expectedRetryer {
				case "adaptive":
					if _, ok := retryer.(*retry.AdaptiveMode); !ok {
						t.Errorf("Expected %s retryer to be adaptive, but got: %T", name, retryer)
					}
				case "standard":
					if _, ok := retryer.(*retry.Standard); !ok {
						t.Errorf("Expected %s retryer to be standard, but got: %T", name, retryer)
					}
				}
			}

			checkRetryer("createAccessPointRetryer", rm.createAccessPointRetryer)
			checkRetryer("deleteAccessPointRetryer", rm.deleteAccessPointRetryer)
			checkRetryer("describeAccessPointsRetryer", rm.describeAccessPointsRetryer)
			checkRetryer("describeMountTargetsRetryer", rm.describeMountTargetsRetryer)
			checkRetryer("describeFileSystemsRetryer", rm.describeFileSystemsRetryer)
		})
	}
}
