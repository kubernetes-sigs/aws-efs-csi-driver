package util

import (
	"testing"
)

func TestParseFileSystemType(t *testing.T) {
	testCases := []struct {
		name          string
		input         string
		expected      FileSystemType
		expectedError bool
	}{
		{
			name:          "Valid EFS filesystem type",
			input:         "efs",
			expected:      FileSystemTypeEFS,
			expectedError: false,
		},
		{
			name:          "Valid S3Files filesystem type",
			input:         "s3files",
			expected:      FileSystemTypeS3Files,
			expectedError: false,
		},
		{
			name:          "Invalid filesystem type",
			input:         "invalid",
			expected:      "",
			expectedError: true,
		},
		{
			name:          "Empty string filesystem type",
			input:         "",
			expected:      "",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseFileSystemType(tc.input)

			if tc.expectedError {
				if err == nil {
					t.Errorf("ParseFileSystemType(%v) expected error but got none", tc.input)
				}
				if result != tc.expected {
					t.Errorf("ParseFileSystemType(%v) = %v, expected %v", tc.input, result, tc.expected)
				}
			} else {
				if err != nil {
					t.Errorf("ParseFileSystemType(%v) returned unexpected error: %v", tc.input, err)
				}
				if result != tc.expected {
					t.Errorf("ParseFileSystemType(%v) = %v, expected %v", tc.input, result, tc.expected)
				}
			}
		})
	}
}
