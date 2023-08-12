package driver

import "testing"

const fileSystemId = "fs-123456789"

func TestGetUidAndGid(t *testing.T) {
	tests := []struct {
		name        string
		rawUid      string
		rawGid      string
		rawGidMin   string
		rawGidMax   string
		resultUid   int
		resultGid   int
		expectError bool
	}{
		{
			name:        "Fixed UID and GID",
			rawUid:      "1000",
			rawGid:      "1001",
			rawGidMin:   "",
			rawGidMax:   "",
			resultUid:   1000,
			resultGid:   1001,
			expectError: false,
		},
		{
			name:        "Ranges are ignored if fixed UID and GID are specified",
			rawUid:      "1000",
			rawGid:      "1001",
			rawGidMin:   "5000",
			rawGidMax:   "70000",
			resultUid:   1000,
			resultGid:   1001,
			expectError: false,
		},
		{
			name:        "Invalid UID throws error",
			rawUid:      "invalid",
			rawGid:      "",
			rawGidMin:   "",
			rawGidMax:   "",
			resultUid:   -1,
			resultGid:   -1,
			expectError: true,
		},
		{
			name:        "Negative UID throws error",
			rawUid:      "-200",
			rawGid:      "",
			rawGidMin:   "",
			rawGidMax:   "",
			resultUid:   -1,
			resultGid:   -1,
			expectError: true,
		},
		{
			name:        "Invalid GID throws error",
			rawUid:      "",
			rawGid:      "invalid",
			rawGidMin:   "",
			rawGidMax:   "",
			resultUid:   -1,
			resultGid:   -1,
			expectError: true,
		},
		{
			name:        "Invalid GID throws error even if range is set",
			rawUid:      "",
			rawGid:      "invalid",
			rawGidMin:   "5000",
			rawGidMax:   "70000",
			resultUid:   -1,
			resultGid:   -1,
			expectError: true,
		},
		{
			name:        "Negative GID throws error",
			rawUid:      "",
			rawGid:      "-200",
			rawGidMin:   "",
			rawGidMax:   "",
			resultUid:   -1,
			resultGid:   -1,
			expectError: true,
		},
		{
			name:        "GID Range Used when Fixed GID not provided",
			rawUid:      "2001",
			rawGid:      "",
			rawGidMin:   "5000",
			rawGidMax:   "50000",
			resultUid:   2001,
			resultGid:   5000,
			expectError: false,
		},
		{
			name:        "GID Min cannot be 0",
			rawUid:      "2001",
			rawGid:      "",
			rawGidMin:   "0",
			rawGidMax:   "50000",
			resultUid:   -1,
			resultGid:   -1,
			expectError: true,
		},
		{
			name:        "GID Min must be numeric",
			rawUid:      "2001",
			rawGid:      "",
			rawGidMin:   "foo",
			rawGidMax:   "50000",
			resultUid:   -1,
			resultGid:   -1,
			expectError: true,
		},
		{
			name:        "GID Max must be numeric",
			rawUid:      "2001",
			rawGid:      "",
			rawGidMin:   "1000",
			rawGidMax:   "foo",
			resultUid:   -1,
			resultGid:   -1,
			expectError: true,
		},
		{
			name:        "GID Min must be less than GID Max",
			rawUid:      "2001",
			rawGid:      "",
			rawGidMin:   "500",
			rawGidMax:   "100",
			resultUid:   -1,
			resultGid:   -1,
			expectError: true,
		},
		{
			name:        "Both GID Min and GID Max must be provided",
			rawUid:      "2001",
			rawGid:      "",
			rawGidMin:   "500",
			rawGidMax:   "",
			resultUid:   -1,
			resultGid:   -1,
			expectError: true,
		},
		{
			name:        "If no GID parameters are provided fallback to the defaults",
			rawUid:      "2001",
			rawGid:      "",
			rawGidMin:   "",
			rawGidMax:   "",
			resultUid:   2001,
			resultGid:   DefaultGidMin,
			expectError: false,
		},
		{
			name:        "If no UID/GID parameters are provided fallback to the defaults in both cases",
			rawUid:      "",
			rawGid:      "",
			rawGidMin:   "",
			rawGidMax:   "",
			resultUid:   DefaultGidMin,
			resultGid:   DefaultGidMin,
			expectError: false,
		},
	}
	for _, test := range tests {
		fsIdManager := NewFileSystemIdentityManager()
		t.Run(test.name, func(t *testing.T) {
			uid, gid, err := fsIdManager.GetUidAndGid(test.rawUid, test.rawGid, test.rawGidMin, test.rawGidMax, fileSystemId)
			if test.expectError {
				if err == nil {
					t.Fatalf("Expected error but completed successfully")
				}
			} else {
				if err != nil {
					t.Fatalf("Didn't expect error but found %v", err)
				}
			}
			if uid != test.resultUid {
				t.Fatalf("Expected UID to be %d, but was %d", test.resultUid, uid)
			}
			if gid != test.resultGid {
				t.Fatalf("Expected GID to be %d, but was %d", test.resultGid, gid)
			}
		})
	}
}
