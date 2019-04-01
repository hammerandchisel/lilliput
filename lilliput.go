// Package lilliput resizes and encodes images from
// compressed images
package lilliput

import (
	"bytes"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidImage     = errors.New("unrecognized image format")
	ErrDecodingFailed   = errors.New("failed to decode image")
	ErrBufTooSmall      = errors.New("buffer too small to hold image")
	ErrFrameBufNoPixels = errors.New("Framebuffer contains no pixels")

	gif87Magic   = []byte("GIF87a")
	gif89Magic   = []byte("GIF89a")
	mp42Magic    = []byte("ftypmp42")
	mp4IsomMagic = []byte("ftypisom")
	pngMagic     = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

	pngActlChunkType = []byte{0x61, 0x63, 0x54, 0x4c}
	pngFctlChunkType = []byte{0x66, 0x63, 0x54, 0x4c}
	pngFdatChunkType = []byte{0x66, 0x64, 0x41, 0x54}
)

// A Decoder decompresses compressed image data.
type Decoder interface {
	// Header returns basic image metadata from the image.
	// This is done lazily, reading only the first part of the image and not
	// a full decode.
	Header() (*ImageHeader, error)

	// Close releases any resources associated with the Decoder
	Close()

	// Description returns a string description of the image type, such as
	// "PNG"
	Description() string

	// Duration returns the duration of the content. This property is 0 for
	// static images and animated GIFs.
	Duration() time.Duration

	// DecodeTo fully decodes the image pixel data into f. Generally users should
	// prefer instead using the ImageOps object to decode images.
	DecodeTo(f *Framebuffer) error
}

// An Encoder compresses raw pixel data into a well-known image type.
type Encoder interface {
	// Encode encodes the pixel data in f into the dst provided to NewEncoder. Encode quality
	// options can be passed into opt, such as map[int]int{lilliput.JpegQuality: 80}
	Encode(f *Framebuffer, opt map[int]int) ([]byte, error)

	// Close releases any resources associated with the Encoder
	Close()
}

func isGIF(maybeGIF []byte) bool {
	return bytes.HasPrefix(maybeGIF, gif87Magic) || bytes.HasPrefix(maybeGIF, gif89Magic)
}

func isMP4(maybeMP4 []byte) bool {
	if len(maybeMP4) < 12 {
		return false
	}

	magic := maybeMP4[4:]
	return bytes.HasPrefix(magic, mp42Magic) || bytes.HasPrefix(magic, mp4IsomMagic)
}

func DeanimateAPNG(maybeApng []byte) {
	if !bytes.HasPrefix(maybeApng, pngMagic) {
		return
	}

	offset := len(pngMagic)
	for {
		if offset+8 > len(maybeApng) {
			return
		}
		chunkSize := binary.BigEndian.Uint32(maybeApng[offset:])
		chunkType := maybeApng[offset+4 : offset+8]
		if bytes.Equal(chunkType, pngActlChunkType) || bytes.Equal(chunkType, pngFctlChunkType) || bytes.Equal(chunkType, pngFdatChunkType) {
			fullChunkSize := (int)(chunkSize) + 12
			copy(maybeApng[offset:], maybeApng[offset+fullChunkSize:])
			maybeApng = maybeApng[:len(maybeApng)-fullChunkSize]
			continue
		}
		offset += (int)(chunkSize) + 12
	}
}

// NewDecoder returns a Decoder which can be used to decode
// image data provided in buf. If the first few bytes of buf do not
// point to a valid magic string, an error will be returned.
func NewDecoder(buf []byte) (Decoder, error) {
	// Check buffer length before accessing it
	if len(buf) == 0 {
		return nil, ErrInvalidImage
	}

	isBufGIF := isGIF(buf)
	if isBufGIF {
		return newGifDecoder(buf)
	}

	maybeDecoder, err := newOpenCVDecoder(buf)
	if err == nil {
		return maybeDecoder, nil
	}

	return newAVCodecDecoder(buf)
}

// NewEncoder returns an Encode which can be used to encode Framebuffer
// into compressed image data. ext should be a string like ".jpeg" or
// ".png". decodedBy is optional and can be the Decoder used to make
// the Framebuffer. dst is where an encoded image will be written.
func NewEncoder(ext string, decodedBy Decoder, dst []byte) (Encoder, error) {
	if strings.ToLower(ext) == ".gif" {
		return newGifEncoder(decodedBy, dst)
	}

	if strings.ToLower(ext) == ".mp4" || strings.ToLower(ext) == ".webm" {
		return nil, errors.New("Encoder cannot encode into video types")
	}

	return newOpenCVEncoder(ext, decodedBy, dst)
}
