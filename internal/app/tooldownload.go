package app

import (
	"io"
	"net/http"
)

type downloadCounter struct {
	done, total int64
	progress    func(int64, int64)
}

func (c *downloadCounter) Write(b []byte) (int, error) {
	c.done += int64(len(b))
	c.progress(c.done, c.total)
	return len(b), nil
}

func readToolDownload(resp *http.Response, progress func(int64, int64)) ([]byte, error) {
	var r io.Reader = io.LimitReader(resp.Body, 64<<20)
	if progress != nil {
		progress(0, resp.ContentLength)
		r = io.TeeReader(r, &downloadCounter{total: resp.ContentLength, progress: progress})
	}
	return io.ReadAll(r)
}
