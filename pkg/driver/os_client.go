package driver

import "os"

type OsClient interface {
	MkDirAllWithPerms(path string, perms os.FileMode, uid, gid int) error
	MkDirAllWithPermsNoOwnership(path string, perms os.FileMode) error
	GetPerms(path string) (os.FileMode, error)
	Remove(path string) error
	RemoveAll(path string) error
}

type RealOsClient struct{}

func NewOsClient() *RealOsClient {
	return &RealOsClient{}
}

func (o *RealOsClient) MkDirAllWithPerms(path string, perms os.FileMode, uid, gid int) error {
	err := os.MkdirAll(path, perms)
	if err != nil {
		return err
	}
	// Extra CHMOD guarantees we get the permissions we desire, inspite of the umask
	err = os.Chmod(path, perms)
	if err != nil {
		return err
	}
	err = os.Chown(path, uid, gid)
	if err != nil {
		return err
	}
	return nil
}

func (o *RealOsClient) MkDirAllWithPermsNoOwnership(path string, perms os.FileMode) error {
	err := os.MkdirAll(path, perms)
	if err != nil {
		return err
	}
	// Extra CHMOD guarantees we get the permissions we desire, inspite of the umask
	err = os.Chmod(path, perms)
	if err != nil {
		return err
	}
	return nil
}

func (o *RealOsClient) Remove(path string) error {
	return os.Remove(path)
}

func (o *RealOsClient) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (o *RealOsClient) GetPerms(path string) (os.FileMode, error) {
	fInfo, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fInfo.Mode(), nil
}
