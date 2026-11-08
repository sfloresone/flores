package provider

import (
	"bufio"
	"errors"
	"io"
)

// ErrStreamEnded is returned by ReadSSE when the body ends while the handler
// still wanted more events: the provider never said it had finished.
var ErrStreamEnded = errors.New("stream ended before the provider finished")

// ReadSSE calls handle with the data of each server-sent event until handle
// returns false or the stream ends. It is the framing shared by every
// adapter; what the data means is the adapter's business.
//
// Lines can be any length: a single text delta may be megabytes long
// (layer B: huge-output). Only "data:" lines matter; comments, "event:",
// "id:" and "retry:" are skipped.
func ReadSSE(body io.Reader, handle func(data []byte) (more bool, err error)) error {
	r := bufio.NewReaderSize(body, 64<<10)
	var (
		data    []byte
		pending bool // a data: line was seen since the last dispatch
	)
	for {
		line, err := r.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			// Longer than the buffer: keep reading the rest of the line.
			long := append([]byte(nil), line...)
			for errors.Is(err, bufio.ErrBufferFull) {
				line, err = r.ReadSlice('\n')
				long = append(long, line...)
			}
			line = long
		}
		if err != nil && !(errors.Is(err, io.EOF) && len(line) > 0) {
			if errors.Is(err, io.EOF) {
				return ErrStreamEnded
			}
			return err
		}
		line = trimEOL(line)
		switch {
		case len(line) == 0: // a blank line ends the event
			if !pending {
				continue
			}
			more, herr := handle(data)
			data, pending = data[:0], false
			if herr != nil || !more {
				return herr
			}
		case line[0] == ':': // comment
		default:
			if v, ok := field(line, "data"); ok {
				if pending {
					data = append(data, '\n')
				}
				data, pending = append(data, v...), true
			}
		}
		if errors.Is(err, io.EOF) {
			return ErrStreamEnded
		}
	}
}

func trimEOL(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}
	if n := len(b); n > 0 && b[n-1] == '\r' {
		b = b[:n-1]
	}
	return b
}

// field returns the value of "name: value" (one optional space after ':').
func field(line []byte, name string) ([]byte, bool) {
	if len(line) <= len(name) || string(line[:len(name)]) != name || line[len(name)] != ':' {
		return nil, false
	}
	v := line[len(name)+1:]
	if len(v) > 0 && v[0] == ' ' {
		v = v[1:]
	}
	return v, true
}
