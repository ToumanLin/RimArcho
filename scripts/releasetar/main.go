// releasetar packages a Linux release with executable permissions, including on Windows.
package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func main() {
	dir := flag.String("dir", "", "release staging directory")
	out := flag.String("out", "", "output tar.gz file outside staging")
	flag.Parse()
	if *dir == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := pack(*dir, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func pack(dir, out string) (err error) {
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, f.Close())
		if err != nil {
			os.Remove(out)
		}
	}()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("unsupported release entry %s", path)
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		name, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(name)
		h.Mode = 0o644
		if d.IsDir() {
			h.Mode = 0o755
			h.Name += "/"
		}
		if name == "archo" {
			h.Mode = 0o755
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		r, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(tw, r)
		return errors.Join(err, r.Close())
	})
	return errors.Join(err, tw.Close(), gz.Close())
}
