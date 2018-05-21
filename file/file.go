package file


import (
    "errors"
    "syscall"
    "os"
)


// A FileInfo describes a file and is returned by Stat and Lstat.
type FileInfo interface {
    os.FileInfo
    UID() (int, error) // UID of the file owner. Returns an error on non-POSIX file systems.
    GID() (int, error) // GID of the file owner. Returns an error on non-POSIX file systems.
}

type fileInfo struct {
    os.FileInfo
    uid *int
    gid *int
}

func (f fileInfo) UID() (int, error) {
    if f.uid == nil {
        return -1, errors.New("uid not implemented")
    }

    return *f.uid, nil
}

func (f fileInfo) GID() (int, error) {
    if f.gid == nil {
        return -1, errors.New("gid not implemented")
    }

    return *f.gid, nil
}

func stat(name string, statFunc func(name string) (os.FileInfo, error)) (FileInfo, error) {
    info, err := statFunc(name)
    if err != nil {
        return nil, err
    }

    stat, ok := info.Sys().(*syscall.Stat_t)
    if !ok {
        return nil, errors.New("failed to get uid/gid")
    }

    uid := int(stat.Uid)
    gid := int(stat.Gid)
    return fileInfo{FileInfo: info, uid: &uid, gid: &gid}, nil
}

// Stat returns a FileInfo describing the named file.
// If there is an error, it will be of type *PathError.
func Stat(name string) (FileInfo, error) {
    return stat(name, os.Stat)
}
