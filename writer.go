package macho

import (
	"bytes"
	"fmt"
	"io"
)

func (f *File) ToWrite(writer io.Writer) (err error) {

	var buf bytes.Buffer

	if err = f.FileHeader.Write(&buf, f.ByteOrder); err != nil {
		return fmt.Errorf("failed to write file header to buffer: %v", err)
	}

	if err = f.writeLoadCommands(&buf); err != nil {
		return fmt.Errorf("failed to write load commands: %v", err)
	}

	endOfLoadsOffset := uint64(buf.Len())
	_, err = writer.Write(buf.Bytes())
	if err != nil {
		return err
	}

	// Write out segment data to buffer
	for _, seg := range f.Segments() {
		if seg.Filesz > 0 {
			switch seg.Name {
			case "__TEXT":
				dat := make([]byte, seg.Filesz)
				if _, err := f.cr.ReadAtAddr(dat, seg.Addr); err != nil {
					return fmt.Errorf("failed to read segment %s data: %v", seg.Name, err)
				}
				if _, err := writer.Write(dat[endOfLoadsOffset:]); err != nil {
					return fmt.Errorf("failed to write segment %s to export buffer: %v", seg.Name, err)
				}
			case "__LINKEDIT":
				if f.ledata != nil && f.ledata.Len() > 0 && f.CodeSignature() != nil {
					if _, err := writer.Write(f.ledata.Bytes()); err != nil {
						return fmt.Errorf("failed to write segment %s to export buffer: %v", seg.Name, err)
					}
				} else {
					dat := make([]byte, seg.Filesz)
					if _, err := f.cr.ReadAtAddr(dat, seg.Addr); err != nil {
						return fmt.Errorf("failed to read segment %s data: %v", seg.Name, err)
					}
					if _, err := writer.Write(dat); err != nil {
						return fmt.Errorf("failed to write segment %s to export buffer: %v", seg.Name, err)
					}
				}
			default:
				dat := make([]byte, seg.Filesz)
				if _, err := f.cr.ReadAtAddr(dat, seg.Addr); err != nil {
					return fmt.Errorf("failed to read segment %s data: %v", seg.Name, err)
				}
				if _, err := writer.Write(dat); err != nil {
					return fmt.Errorf("failed to write segment %s to export buffer: %v", seg.Name, err)
				}
			}
		}
	}

	//_, err = io.Copy(writer, &buf)
	return err
}
