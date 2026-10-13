package typesafe

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"gopkg.in/yaml.v3"
)

const maxCredentialFileSize = 64 << 10

func readCredentialFile(path string) ([]byte, error) {
	rootPath, leaf, err := credentialLocation(path)
	if err != nil {
		return nil, errors.New("invalid credential location")
	}
	root, err := openCredentialRoot(rootPath, false)
	if err != nil {
		return nil, credentialReadError(err)
	}
	defer root.Close()

	file, err := root.OpenFile(leaf, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, credentialReadError(err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || !privateCredentialInfo(info, false) {
		return nil, errors.New("credential file is unsafe")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxCredentialFileSize+1))
	if err != nil {
		return nil, errors.New("could not read credential file")
	}
	if len(data) > maxCredentialFileSize {
		return nil, errors.New("credential file is too large")
	}
	return data, nil
}

func writeCredentialFile(path string, value credentialFile) error {
	rootPath, leaf, err := credentialLocation(path)
	if err != nil {
		return errors.New("invalid credential location")
	}
	root, err := openCredentialRoot(rootPath, true)
	if err != nil {
		return errors.New("could not securely access credential directory")
	}
	defer root.Close()

	if err := validateExistingCredential(root, leaf); err != nil {
		return err
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return errors.New("could not encode credentials")
	}
	if len(data) > maxCredentialFileSize {
		return errors.New("credential file is too large")
	}
	tempName, file, err := createCredentialTemp(root)
	if err != nil {
		return errors.New("could not create credential file")
	}
	defer root.Remove(tempName)

	if err := writeCredentialTemp(file, data); err != nil {
		return errors.New("could not write credential file")
	}
	if err := root.Rename(tempName, leaf); err != nil {
		return errors.New("could not save credential file")
	}
	return nil
}

func credentialLocation(path string) (string, string, error) {
	if path == "" {
		return "", "", errors.New("empty credential path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", errors.New("invalid credential path")
	}
	clean := filepath.Clean(abs)
	leaf := filepath.Base(clean)
	if leaf == "." || leaf == string(filepath.Separator) || leaf == ".." {
		return "", "", errors.New("invalid credential path")
	}
	return filepath.Dir(clean), leaf, nil
}

func openCredentialRoot(path string, create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(path, 0700); err != nil {
			return nil, err
		}
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.IsDir() || !privateCredentialInfo(before, true) {
		return nil, errors.New("unsafe credential directory")
	}

	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	actual, err := root.Stat(".")
	if err != nil || !actual.IsDir() || !os.SameFile(before, actual) || !privateCredentialInfo(actual, true) {
		root.Close()
		return nil, errors.New("unsafe credential directory")
	}
	return root, nil
}
func privateCredentialInfo(info os.FileInfo, directory bool) bool {
	if info == nil || info.Mode().Perm()&0077 != 0 {
		return false
	}
	if directory {
		if !info.IsDir() {
			return false
		}
	} else if !info.Mode().IsRegular() {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}

func validateExistingCredential(root *os.Root, leaf string) error {
	file, err := root.OpenFile(leaf, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return errors.New("credential file is unsafe")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !privateCredentialInfo(info, false) {
		return errors.New("credential file is unsafe")
	}
	return nil
}

func createCredentialTemp(root *os.Root) (string, *os.File, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", nil, err
	}
	name := ".typesafe-credential-" + hex.EncodeToString(random[:])
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return "", nil, err
	}
	if err := file.Chmod(0600); err != nil {
		file.Close()
		root.Remove(name)
		return "", nil, err
	}
	return name, file, nil
}

func writeCredentialTemp(file *os.File, data []byte) error {
	info, err := file.Stat()
	if err != nil || !privateCredentialInfo(info, false) || info.Mode().Perm() != 0600 {
		file.Close()
		return errors.New("unsafe credential temporary file")
	}
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func credentialReadError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return os.ErrNotExist
	}
	return errors.New("could not securely read credential file")
}
