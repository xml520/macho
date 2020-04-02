// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package macho

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/blacktop/go-macho/types"
)

// A FatFile is a Mach-O universal binary that contains at least one architecture.
type FatFile struct {
	Magic  types.Magic
	Arches []FatArch
	closer io.Closer
}

// A FatArchHeader represents a fat header for a specific image architecture.
type FatArchHeader struct {
	CPU    types.CPU
	SubCPU types.CPUSubtype
	Offset uint32
	Size   uint32
	Align  uint32
}

const fatArchHeaderSize = 5 * 4

// A FatArch is a Mach-O File inside a FatFile.
type FatArch struct {
	FatArchHeader
	*File
}

// ErrNotFat is returned from NewFatFile or OpenFat when the file is not a
// universal binary but may be a thin binary, based on its magic number.
var ErrNotFat = &FormatError{0, "not a fat Mach-O file", nil}

// NewFatFile creates a new FatFile for accessing all the Mach-O images in a
// universal binary. The Mach-O binary is expected to start at position 0 in
// the ReaderAt.
func NewFatFile(r io.ReaderAt) (*FatFile, error) {
	var ff FatFile
	sr := io.NewSectionReader(r, 0, 1<<63-1)

	// Read the fat_header struct, which is always in big endian.
	// Start with the magic number.
	err := binary.Read(sr, binary.BigEndian, &ff.Magic)
	if err != nil {
		return nil, &FormatError{0, "error reading magic number", nil}
	} else if ff.Magic != types.MagicFat {
		// See if this is a Mach-O file via its magic number. The magic
		// must be converted to little endian first though.
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], ff.Magic.Int())
		leMagic := binary.LittleEndian.Uint32(buf[:])
		if leMagic == types.Magic32.Int() || leMagic == types.Magic64.Int() {
			return nil, ErrNotFat
		}
		return nil, &FormatError{0, "invalid magic number", nil}

	}
	offset := int64(4)

	// Read the number of FatArchHeaders that come after the fat_header.
	var narch uint32
	err = binary.Read(sr, binary.BigEndian, &narch)
	if err != nil {
		return nil, &FormatError{offset, "invalid fat_header", nil}
	}
	offset += 4

	if narch < 1 {
		return nil, &FormatError{offset, "file contains no images", nil}
	}

	// Combine the Cpu and SubCpu (both uint32) into a uint64 to make sure
	// there are not duplicate architectures.
	seenArches := make(map[uint64]bool, narch)
	// Make sure that all images are for the same MH_ type.
	var machoType types.HeaderType

	// Following the fat_header comes narch fat_arch structs that index
	// Mach-O images further in the file.
	ff.Arches = make([]FatArch, narch)
	for i := uint32(0); i < narch; i++ {
		fa := &ff.Arches[i]
		err = binary.Read(sr, binary.BigEndian, &fa.FatArchHeader)
		if err != nil {
			return nil, &FormatError{offset, "invalid fat_arch header", nil}
		}
		offset += fatArchHeaderSize

		fr := io.NewSectionReader(r, int64(fa.Offset), int64(fa.Size))
		fa.File, err = NewFile(fr)
		if err != nil {
			return nil, err
		}

		// Make sure the architecture for this image is not duplicate.
		seenArch := (uint64(fa.CPU) << 32) | uint64(fa.SubCPU)
		if o, k := seenArches[seenArch]; o || k {
			return nil, &FormatError{offset, fmt.Sprintf("duplicate architecture cpu=%v, subcpu=%#x", fa.CPU, fa.SubCPU), nil}
		}
		seenArches[seenArch] = true

		// Make sure the Mach-O type matches that of the first image.
		if i == 0 {
			machoType = fa.HeaderType
		} else {
			if fa.HeaderType != machoType {
				return nil, &FormatError{offset, fmt.Sprintf("Mach-O type for architecture #%d (type=%#x) does not match first (type=%#x)", i, fa.HeaderType, machoType), nil}
			}
		}
	}

	return &ff, nil
}

// OpenFat opens the named file using os.Open and prepares it for use as a Mach-O
// universal binary.
func OpenFat(name string) (*FatFile, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	ff, err := NewFatFile(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	ff.closer = f
	return ff, nil
}

// Close with close the Mach-O Fat file.
func (ff *FatFile) Close() error {
	var err error
	if ff.closer != nil {
		err = ff.closer.Close()
		ff.closer = nil
	}
	return err
}
