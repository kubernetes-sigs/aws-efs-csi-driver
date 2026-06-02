package util

import (
	"fmt"
)

// FileSystemType represents the type of filesystem
type FileSystemType string

const (
	// FileSystemTypeEFS represents Amazon EFS filesystem
	FileSystemTypeEFS FileSystemType = "efs"
	// FileSystemTypeS3Files represents Amazon S3 Files filesystem
	FileSystemTypeS3Files FileSystemType = "s3files"
)

// String returns the string representation of the filesystem type
func (f FileSystemType) String() string {
	return string(f)
}

// ParseFileSystemType converts string to FileSystemType enum.
// Returns an error if the string is not a valid filesystem type
func ParseFileSystemType(s string) (FileSystemType, error) {
	switch s {
	case string(FileSystemTypeEFS):
		return FileSystemTypeEFS, nil
	case string(FileSystemTypeS3Files):
		return FileSystemTypeS3Files, nil
	default:
		return "", fmt.Errorf("invalid filesystem type: %s", s)
	}
}
