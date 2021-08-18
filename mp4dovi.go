package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
)

type FourCC [4]byte
type BoxType FourCC

var (
	MoovBoxType = BoxType{'m', 'o', 'o', 'v'}
	TrakBoxType = BoxType{'t', 'r', 'a', 'k'}
	MdiaBoxType = BoxType{'m', 'd', 'i', 'a'}
	MinfBoxType = BoxType{'m', 'i', 'n', 'f'}
	StblBoxType = BoxType{'s', 't', 'b', 'l'}
	StsdBoxType = BoxType{'s', 't', 's', 'd'}
	DvheBoxType = BoxType{'d', 'v', 'h', 'e'}
	Dvh1BoxType = BoxType{'d', 'v', 'h', '1'}
)

const HeaderSize = 8

type Header struct {
	Size uint32
	Type BoxType
}

func findHeader(r io.ReadSeeker, boxType BoxType, limit int64) (header *Header, err error) {
	var h Header
	for offset := int64(0); limit < 0 || offset < limit; offset += int64(h.Size) {
		if err = binary.Read(r, binary.BigEndian, &h); err != nil {
			return nil, fmt.Errorf(`failed reading box header: %w`, err)
		}
		if h.Type == boxType {
			return &h, nil
		}
		if _, err = r.Seek(int64(h.Size-HeaderSize), io.SeekCurrent); err != nil {
			return nil, fmt.Errorf(`failed seeking after box "%s": %s`, h.Type, err)
		}
	}
	return nil, fmt.Errorf(`cannot find box "%s"`, boxType)
}

func forEachBox(r io.ReadSeeker, limit int64, fn func(header Header) error) (err error) {
	var h Header
	var start int64
	if start, err = r.Seek(0, io.SeekCurrent); err != nil {
		return fmt.Errorf(`failed to get current offset with seek: %w`, err)
	}
	for offset := start; limit < 0 || offset < start+limit; offset += int64(h.Size) {
		if _, err = r.Seek(offset, io.SeekStart); err != nil {
			return fmt.Errorf(`failed to seek to offset: %w`, err)
		}
		if err = binary.Read(r, binary.BigEndian, &h); err != nil {
			return fmt.Errorf(`failed reading box header: %w`, err)
		}
		if err = fn(h); err != nil {
			return fmt.Errorf(`callback failed: %w`, err)
		}
	}
	return
}

func sampleEntryHandler(rw *os.File) func(Header) error {
	return func(h Header) (err error) {
		if h.Type == DvheBoxType {
			if _, err = rw.Seek(-4, io.SeekCurrent); err != nil {
				return fmt.Errorf(`failed to seek back: %w`, err)
			}
			if err = binary.Write(rw, binary.BigEndian, Dvh1BoxType); err != nil {
				return fmt.Errorf(`failed to write box header type "%s": %w`, Dvh1BoxType, err)
			}
			fmt.Println(`Changed Dolby Vision codec from dvhe to dvh1`)
		}
		return
	}
}

func trakHandler(rw *os.File) func(Header) error {
	return func(trak Header) (err error) {
		var h *Header
		var sampleEntryCount uint32

		if trak.Type != TrakBoxType {
			return
		}

		if h, err = findHeader(rw, MdiaBoxType, int64(trak.Size-HeaderSize)); err != nil {
			return fmt.Errorf(`failed finding box "%s": %w`, MdiaBoxType, err)
		}

		if h, err = findHeader(rw, MinfBoxType, int64(h.Size-HeaderSize)); err != nil {
			return fmt.Errorf(`failed finding box "%s": %w`, MinfBoxType, err)
		}

		if h, err = findHeader(rw, StblBoxType, int64(h.Size-HeaderSize)); err != nil {
			return fmt.Errorf(`failed finding box "%s": %w`, StblBoxType, err)
		}

		if h, err = findHeader(rw, StsdBoxType, int64(h.Size-HeaderSize)); err != nil {
			return fmt.Errorf(`failed finding box "%s": %w`, StsdBoxType, err)
		}

		if _, err = rw.Seek(4, io.SeekCurrent); err != nil {
			return fmt.Errorf(`failed to seek: %w`, err)
		}

		if err = binary.Read(rw, binary.BigEndian, &sampleEntryCount); err != nil {
			return fmt.Errorf(`failed to read sampleEntryCount: %w`, err)
		}

		if err = forEachBox(rw, int64(h.Size-HeaderSize-8), sampleEntryHandler(rw)); err != nil {
			return fmt.Errorf(`failed processing sample entry list: %w`, err)
		}

		return
	}
}

func run(mp4file string) (err error) {
	var (
		rw *os.File
		h  *Header
	)
	if rw, err = os.OpenFile(mp4file, os.O_RDWR, 0); err != nil {
		return fmt.Errorf(`cannot open file "%s": %w`, mp4file, err)
	}

	if _, err = rw.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf(`failed to seek: %w`, err)
	}

	if h, err = findHeader(rw, MoovBoxType, -1); err != nil {
		return fmt.Errorf(`failed finding box "%s": %w`, MoovBoxType, err)
	}

	if err = forEachBox(rw, int64(h.Size-HeaderSize), trakHandler(rw)); err != nil {
		return fmt.Errorf(`failed processing moov children: %w`, err)
	}
	return
}

func help() {
	fmt.Println(`Usage: mp4dovi <file>`)
}

func main() {

	if len(os.Args) != 2 {
		help()
		os.Exit(1)
	}

	if err := run(os.Args[1]); err != nil {
		log.Fatal(err)
	}
}
