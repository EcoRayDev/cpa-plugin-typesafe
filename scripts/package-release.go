// Package a single shared library at the ZIP root, as required by the plugin store.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	library := flag.String("library", "", "Shared library to package")
	archive := flag.String("archive", "", "Output ZIP")
	checksum := flag.String("checksum", "", "Output sha256sum file")
	flag.Parse()
	if err := packageLibrary(*library, *archive, *checksum); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func packageLibrary(library, archive, checksum string) error {
	if library == "" || archive == "" || checksum == "" {
		return fmt.Errorf("library, archive and checksum are required")
	}
	source, err := os.Open(library)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("shared library must be a regular file")
	}
	out, err := os.Create(archive)
	if err != nil {
		return err
	}
	hash := sha256.New()
	writer := zip.NewWriter(io.MultiWriter(out, hash))
	entry, err := writer.CreateHeader(&zip.FileHeader{Name: filepath.Base(library), Method: zip.Deflate})
	if err == nil {
		_, err = io.Copy(entry, source)
	}
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	line := hex.EncodeToString(hash.Sum(nil)) + "  " + filepath.Base(archive) + "\n"
	return os.WriteFile(checksum, []byte(line), 0644)
}
