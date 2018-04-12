package fs

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

func Chown(path, username string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return err
	}
	return os.Chown(path, uid, gid)
}

func RecursiveChown(path, username string) error {
	return filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		return Chown(path, username)
	})
}

func CleanDirectory(path string) error {
	err := os.RemoveAll(path)
	if err != nil {
		return err
	}

	return os.Mkdir(path, 0700)
}
