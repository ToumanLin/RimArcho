package menubg

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/bits"

	"github.com/woozymasta/bcn"
)

const maxMenuTexture = 128 << 20

// A split BC7 payload starts with one command per block in the entire mip chain.
// Endpoint records follow, grouped by command, then similarly grouped indices.
// Commands 0..4, 7 and 8 retain the two original 64-bit words. Commands 5/6
// separate colour deltas and alpha; 9/10 omit fields implied by opaque endpoints.
var splitOrder = [...]byte{9, 6, 10, 5, 1, 3, 4, 0, 2, 7, 8}
var splitEndpointSize = [...]int{8, 8, 8, 8, 8, 9, 9, 8, 8, 6, 7}
var splitIndexSize = [...]int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 4}

type splitBC7Reader struct {
	ctx                context.Context
	payload            []byte
	commands           []byte
	endpoints, indices [11]int
	next               int
	block              [16]byte
	pending            []byte
}

// readSplitDDS buffers only the bounded transformed payload. Reconstructed BC7
// blocks feed the existing four-row decoder; no full joined DDS is allocated.
func readSplitDDS(ctx context.Context, r io.Reader, size uint64) (*DDS, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if size < 148 || size > maxMenuTexture {
		return nil, errors.New("split DDS entry size is outside the 148-byte to 128-MiB limit")
	}
	var header [148]byte
	if _, err := io.ReadFull(contextReader{ctx, r}, header[:]); err != nil {
		return nil, err
	}
	d, err := ReadDDS(bytes.NewReader(header[:]))
	if err != nil {
		return nil, err
	}
	if string(header[84:88]) != "DX10" || d.format != bcn.FormatBC7 {
		return nil, errors.New("unsupported split DDS format: only DX10 BC7 (DXGI 98/99) is supported")
	}
	mips := binary.LittleEndian.Uint32(header[28:32])
	if mips == 0 || mips > uint32(bits.Len(uint(max(d.Width, d.Height)))) {
		return nil, errors.New("invalid split DDS mip count")
	}
	var count uint64
	for level := uint32(0); level < mips; level++ {
		w, h := max(1, d.Width>>level), max(1, d.Height>>level)
		count += uint64((w+3)/4) * uint64((h+3)/4)
	}
	// Every command needs one byte plus 11..17 record bytes. All arithmetic stays
	// in uint64 until the declared entry bound proves the allocation fits int.
	length := size - uint64(len(header))
	if length < count*12 || length > count*18 {
		return nil, errors.New("split DDS payload size does not match its mip chain")
	}
	payload := make([]byte, int(length))
	sink := splitPayloadWriter{remaining: payload}
	// Copy to EOF, rather than ReadFull: a ZIP checksum error returned together
	// with the final bytes must not be discarded by an exact-length read.
	if _, err := io.Copy(&sink, contextReader{ctx, r}); err != nil {
		return nil, err
	}
	if len(sink.remaining) != 0 {
		return nil, io.ErrUnexpectedEOF
	}
	s := &splitBC7Reader{ctx: ctx, payload: payload, commands: payload[:int(count)]}
	var counts [11]int
	for i, command := range s.commands {
		if i&16383 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if int(command) >= len(counts) {
			return nil, fmt.Errorf("invalid split BC7 command %d at block %d", command, i)
		}
		counts[command]++
	}
	offset := len(s.commands)
	for _, command := range splitOrder {
		s.endpoints[command] = offset
		offset += counts[command] * splitEndpointSize[command]
	}
	for _, command := range splitOrder {
		s.indices[command] = offset
		offset += counts[command] * splitIndexSize[command]
	}
	if offset != len(payload) {
		return nil, errors.New("split BC7 record lengths do not match payload size")
	}
	d.r = s
	return d, nil
}

type splitPayloadWriter struct{ remaining []byte }

func (w *splitPayloadWriter) Write(p []byte) (int, error) {
	if len(p) > len(w.remaining) {
		return 0, errors.New("split DDS payload exceeds declared entry size")
	}
	n := copy(w.remaining, p)
	w.remaining = w.remaining[n:]
	return n, nil
}

func (s *splitBC7Reader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	written := 0
	for len(p) != 0 {
		if s.next&255 == 0 || written == 0 {
			if err := s.ctx.Err(); err != nil {
				return written, err
			}
		}
		if len(s.pending) == 0 {
			if s.next == len(s.commands) {
				if written != 0 {
					return written, nil
				}
				return 0, io.EOF
			}
			command := s.commands[s.next]
			ep, ix := s.endpoints[command], s.indices[command]
			endpoints := s.payload[ep : ep+splitEndpointSize[command]]
			indices := s.payload[ix : ix+splitIndexSize[command]]
			block, err := restoreSplitBlock(command, endpoints, indices)
			if err != nil {
				return written, fmt.Errorf("split BC7 block %d: %w", s.next, err)
			}
			s.block = block
			s.endpoints[command] += len(endpoints)
			s.indices[command] += len(indices)
			s.next++
			s.pending = s.block[:]
		}
		n := copy(p, s.pending)
		p, s.pending = p[n:], s.pending[n:]
		written += n
	}
	return written, nil
}

// Colour records store G0, the G1 delta, and the R/B deltas from the
// corresponding G endpoint. Each component is arithmetic modulo 128.
func splitColours(record []byte) ([6]byte, error) {
	var result [6]byte
	for _, v := range record[:6] {
		if v > 127 {
			return result, errors.New("split BC7 colour exceeds seven bits")
		}
	}
	g0, g1 := record[2], (record[2]+record[3])&127
	result = [6]byte{(g0 + record[0]) & 127, (g1 + record[1]) & 127, g0, g1, (g0 + record[4]) & 127, (g1 + record[5]) & 127}
	return result, nil
}

// packBC7 appends fields in the bit order defined by BC7, across the word
// boundary. Using field widths keeps the two alpha layouts independent.
type packBC7 struct {
	low, high uint64
	used      uint
}

func (p *packBC7) put(value uint64, width uint) {
	if p.used < 64 {
		p.low |= value << p.used
		if p.used+width > 64 {
			p.high |= value >> (64 - p.used)
		}
	} else {
		p.high |= value << (p.used - 64)
	}
	p.used += width
}

func restoreSplitBlock(command byte, endpoints, indices []byte) ([16]byte, error) {
	var out [16]byte
	if command != 5 && command != 6 && command != 9 && command != 10 {
		copy(out[:8], endpoints)
		copy(out[8:], indices)
		return out, nil
	}
	var packed packBC7
	colourRecord := endpoints
	if command == 5 {
		colourRecord = endpoints[1:]
	}
	colours, err := splitColours(colourRecord)
	if err != nil {
		return out, err
	}
	switch command {
	case 6, 9:
		packed.put(1<<6, 7)
	case 5, 10:
		packed.put(1<<5, 6)
		rotation := byte(0)
		if command == 5 {
			rotation = endpoints[0]
			if rotation > 3 {
				return out, errors.New("invalid split BC7 rotation")
			}
		}
		packed.put(uint64(rotation), 2)
	}
	for _, value := range colours {
		packed.put(uint64(value), 7)
	}
	switch command {
	case 6:
		if endpoints[6] > 127 || endpoints[7] > 127 || endpoints[8] > 1 {
			return out, errors.New("invalid split BC7 alpha or endpoint bit")
		}
		packed.put(uint64(endpoints[6]), 7)
		packed.put(uint64(endpoints[7]), 7)
		packed.put(uint64(endpoints[8]), 1)
		packed.put(binary.LittleEndian.Uint64(indices), 64)
	case 9:
		packed.put(127, 7)
		packed.put(127, 7)
		packed.put(1, 1)
		packed.put(binary.LittleEndian.Uint64(indices), 64)
	case 5, 10:
		alpha0, alpha1 := byte(255), endpoints[len(endpoints)-1]
		if command == 5 {
			alpha0 = endpoints[7]
		}
		packed.put(uint64(alpha0), 8)
		packed.put(uint64(alpha1), 8)
		for stream := 0; stream < 2; stream++ {
			var padded uint32
			if stream == 0 || command == 5 {
				padded = binary.LittleEndian.Uint32(indices[stream*4:])
			}
			if padded&2 != 0 {
				return out, errors.New("invalid split BC7 index padding")
			}
			packed.put(uint64(padded&1|(padded>>1)&0x7ffffffe), 31)
		}
	}
	binary.LittleEndian.PutUint64(out[:8], packed.low)
	binary.LittleEndian.PutUint64(out[8:], packed.high)
	return out, nil
}
