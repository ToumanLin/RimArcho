package updatechannel

import (
	"errors"
	"io"

	"github.com/ToumanLin/RimArcho/internal/lanzou"
)

func readChecksum(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, lanzou.MaxChecksum+1))
	if err != nil {
		return nil, err
	}
	if len(b) > lanzou.MaxChecksum {
		return nil, errors.New("checksum exceeds 64 KiB")
	}
	return b, nil
}
