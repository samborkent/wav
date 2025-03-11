// https://www.mmsp.ece.mcgill.ca/Documents/AudioFormats/WAVE/WAVE.html
package wav

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	FactChunkSize             = 4
	FormatChunkSizePCM        = 16
	FormatChunkSizeNonPCM     = 18
	FormatChunkSizeExtensible = 40
)

const (
	FormatUnknown    = 0x0000
	FormatPCM        = 0x0001
	FormatIEEEFloat  = 0x0003
	FormatALaw       = 0x0006
	FormatMuLaw      = 0x0007
	FormatMP3        = 0x0055
	FormatAAC        = 0x00FF
	FormatOpus       = 0x704F
	FormatMPEG4      = 0xA106
	FormatFLAC       = 0xF1AC
	FormatExtensible = 0xFFFE
)

const (
	ExtensionSizeZero       = 0
	ExtensionSizeExtensible = 22
)

var (
	ErrDataTooLarge      = errors.New("data exceeds wav length limit of 4 GiB")
	ErrBitDepthTooHigh   = errors.New("bit depth exceeds wav limit of 2^16")
	ErrFloatNotSupported = errors.New("floating point representation is currently not supported")
	ErrInvalidBitDepth   = errors.New("invalid bit depth")
	ErrSampleRateTooHigh = errors.New("sample rate exceeds wav limit of 2^32")
	ErrTooManyChannels   = errors.New("number of channels exceeds wav limit of 2^16")
)

var (
	ErrDecodeRIFFID                   = errors.New("riff chunk id does not match 'RIFF'")
	ErrDecodeRIFFSize                 = errors.New("riff chunk does not match data sub-chunk size")
	ErrDecodeRIFFFormat               = errors.New("riff chunk format does not match 'WAVE'")
	ErrDecodeFormatID                 = errors.New("format sub-chunk id does not match 'fmt '")
	ErrDecodeFormatSize               = errors.New("format sub-chunk size other than 16 (PCM) is not supported")
	ErrDecodeFormat                   = errors.New("format sub-chunk audio format is not supported")
	ErrDecodeFormatBitsPerSample      = errors.New("format sub-chunk bits per sample must be divisible by 8")
	ErrDecodeFormatExtensionSize      = errors.New("format sub-chunk extension size invalid for this format type")
	ErrDecodeFormatValidBitsPerSample = errors.New("format sub-chunk valid bits per sample cannot exceed bits per sample")
	ErrDecodeFormatSubFormat          = errors.New("format sub-chunk sub-format first two bytes do not match format")
	ErrDecodeFactID                   = errors.New("fact sub-chunk id does not match 'fact'")
	ErrDecodeFactSize                 = errors.New("fact sub-chunk size must be 4 bytes")
	ErrDecodeDataID                   = errors.New("data sub-chunk id does not match 'data'")
)

type WAVEFileFormat struct {
	RIFFChunk
	FormatChunk
	FactChunk // Optional
	DataChunk
}

type Chunk struct {
	ID   [4]byte // Big endian
	Size [4]byte // Little endian
}

type RIFFChunk struct {
	Chunk
	Identifier [4]byte // Big endian
}

type FormatChunk struct {
	Chunk
	Format             [2]byte  // Little endian
	NumChannels        [2]byte  // Little endian
	SampleRate         [4]byte  // Little endian
	ByteRate           [4]byte  // Little endian
	BlockAlign         [2]byte  // Little endian
	BitsPerSample      [2]byte  // Little endian
	ExtensionSize      [2]byte  // Little endian, optional
	ValidBitsPerSample [2]byte  // Little endian, optional
	ChannelMask        [4]byte  // Little endian, optional
	SubFormat          [16]byte // Big endian, optional
}

type FactChunk struct {
	Chunk
	SampleLength [4]byte // Little endian
}

type DataChunk struct {
	Chunk
	Data        []byte // Little endian
	PaddingByte byte   // Optional
}

type Config struct {
	Channels      int
	SampleRate    int
	BitDepth      int
	FloatingPoint bool
}

// TODO: implement extension
func New(cfg Config, data []byte) (*WAVEFileFormat, error) {
	if cfg.Channels > math.MaxUint16 {
		return nil, ErrTooManyChannels
	}

	if cfg.SampleRate > math.MaxUint32 {
		return nil, ErrSampleRateTooHigh
	}

	if cfg.BitDepth%8 != 0 {
		return nil, ErrInvalidBitDepth
	}

	bytesPerSample := uint16(cfg.BitDepth) / 8

	if len(data)+36 > math.MaxUint32 {
		return nil, ErrDataTooLarge
	}

	var chunkSize [4]byte
	var numChannels [2]byte
	var sampleRate [4]byte
	var byteRate [4]byte
	var blockAlign [2]byte
	var bitsPerSample [2]byte
	var dataChunkSize [4]byte

	binary.LittleEndian.PutUint32(chunkSize[:], uint32(4+(8+FormatChunkSizePCM)+(8+len(data))))
	binary.LittleEndian.PutUint16(numChannels[:], uint16(cfg.Channels))
	binary.LittleEndian.PutUint32(sampleRate[:], uint32(cfg.SampleRate))
	binary.LittleEndian.PutUint32(byteRate[:], uint32(uint16(cfg.Channels)*bytesPerSample)*uint32(cfg.SampleRate))
	binary.LittleEndian.PutUint16(blockAlign[:], uint16(cfg.Channels)*bytesPerSample)
	binary.LittleEndian.PutUint16(bitsPerSample[:], uint16(cfg.BitDepth))
	binary.LittleEndian.PutUint32(dataChunkSize[:], uint32(len(data)))

	if cfg.FloatingPoint {
		var sampleLength [4]byte

		// TODO: check if this should be sampled per channel, or total number of samples
		binary.LittleEndian.PutUint32(sampleLength[:], uint32(len(data))/(uint32(cfg.Channels)*uint32(cfg.BitDepth)))

		return &WAVEFileFormat{
			RIFFChunk: RIFFChunk{
				Chunk: Chunk{
					ID:   [4]byte{'R', 'I', 'F', 'F'},
					Size: chunkSize,
				},
				Identifier: [4]byte{'W', 'A', 'V', 'E'},
			},
			FormatChunk: FormatChunk{
				Chunk: Chunk{
					ID:   [4]byte{'f', 'm', 't', ' '},
					Size: [4]byte{FormatChunkSizeNonPCM, 0, 0, 0},
				},
				Format:        [2]byte{FormatIEEEFloat, 0},
				NumChannels:   numChannels,
				SampleRate:    sampleRate,
				ByteRate:      byteRate,
				BlockAlign:    blockAlign,
				BitsPerSample: bitsPerSample,
				ExtensionSize: [2]byte{ExtensionSizeZero, 0},
			},
			FactChunk: FactChunk{
				Chunk: Chunk{
					ID:   [4]byte{'f', 'a', 'c', 't'},
					Size: [4]byte{FactChunkSize, 0, 0, 0},
				},
				SampleLength: sampleLength,
			},
			DataChunk: DataChunk{
				Chunk: Chunk{
					ID:   [4]byte{'d', 'a', 't', 'a'},
					Size: dataChunkSize,
				},
				Data: data,
			},
		}, nil
	} else {
		return &WAVEFileFormat{
			RIFFChunk: RIFFChunk{
				Chunk: Chunk{
					ID:   [4]byte{'R', 'I', 'F', 'F'},
					Size: chunkSize,
				},
				Identifier: [4]byte{'W', 'A', 'V', 'E'},
			},
			FormatChunk: FormatChunk{
				Chunk: Chunk{
					ID:   [4]byte{'f', 'm', 't', ' '},
					Size: [4]byte{FormatChunkSizePCM, 0, 0, 0},
				},
				Format:        [2]byte{FormatPCM, 0},
				NumChannels:   numChannels,
				SampleRate:    sampleRate,
				ByteRate:      byteRate,
				BlockAlign:    blockAlign,
				BitsPerSample: bitsPerSample,
			},
			DataChunk: DataChunk{
				Chunk: Chunk{
					ID:   [4]byte{'d', 'a', 't', 'a'},
					Size: dataChunkSize,
				},
				Data: data,
			},
		}, nil
	}
}

func (f *WAVEFileFormat) Data() []byte {
	return f.DataChunk.Data
}

func (f *WAVEFileFormat) DataSize() int {
	return int(binary.LittleEndian.Uint32(f.DataChunk.Chunk.Size[:]))
}

func (f *WAVEFileFormat) Decode(reader io.Reader) error {
	// RIFF chuck ID
	n, err := reader.Read(f.RIFFChunk.Chunk.ID[:])
	if err != nil {
		return fmt.Errorf("reading riff chunk: id: %w", err)
	} else if n != len(f.RIFFChunk.Chunk.ID) {
		return fmt.Errorf("reading riff chunk: id: %w", io.ErrShortBuffer)
	}

	if f.RIFFChunk.Chunk.ID != [4]byte{'R', 'I', 'F', 'F'} {
		return ErrDecodeRIFFID
	}

	// RIFF chuck size
	n, err = reader.Read(f.RIFFChunk.Chunk.Size[:])
	if err != nil {
		return fmt.Errorf("reading riff chunk: size: %w", err)
	} else if n != len(f.RIFFChunk.Chunk.Size) {
		return fmt.Errorf("reading riff chunk: size: %w", io.ErrShortBuffer)
	}

	chuckSize := binary.LittleEndian.Uint32(f.RIFFChunk.Chunk.Size[:])

	// RIFF format
	n, err = reader.Read(f.RIFFChunk.Identifier[:])
	if err != nil {
		return fmt.Errorf("reading riff chunk: identifier: %w", err)
	} else if n != len(f.RIFFChunk.Identifier) {
		return fmt.Errorf("reading riff chunk: identifier: %w", io.ErrShortBuffer)
	}

	if f.RIFFChunk.Identifier != [4]byte{'W', 'A', 'V', 'E'} {
		return ErrDecodeRIFFFormat
	}

	// Format sub-chunk ID
	n, err = reader.Read(f.FormatChunk.Chunk.ID[:])
	if err != nil {
		return fmt.Errorf("reading format sub-chunk: id: %w", err)
	} else if n != len(f.FormatChunk.Chunk.ID) {
		return fmt.Errorf("reading format sub-chunk: id: %w", io.ErrShortBuffer)
	}

	if f.FormatChunk.Chunk.ID != [4]byte{'f', 'm', 't', ' '} {
		return ErrDecodeFormatID
	}

	// Format sub-chunk size
	n, err = reader.Read(f.FormatChunk.Chunk.Size[:])
	if err != nil {
		return fmt.Errorf("reading format sub-chunk: size: %w", err)
	} else if n != len(f.FormatChunk.Chunk.Size) {
		return fmt.Errorf("reading format sub-chunk: size: %w", io.ErrShortBuffer)
	}

	formatSize := binary.LittleEndian.Uint32(f.FormatChunk.Chunk.Size[:])

	// Format sub-chunk audio format
	n, err = reader.Read(f.FormatChunk.Format[:])
	if err != nil {
		return fmt.Errorf("reading format sub-chunk: audio format: %w", err)
	} else if n != len(f.FormatChunk.Format) {
		return fmt.Errorf("reading format sub-chunk: audio format: %w", io.ErrShortBuffer)
	}

	// Format sub-chunk number of channels
	n, err = reader.Read(f.FormatChunk.NumChannels[:])
	if err != nil {
		return fmt.Errorf("reading format sub-chunk: number of channels: %w", err)
	} else if n != len(f.FormatChunk.NumChannels) {
		return fmt.Errorf("reading format sub-chunk: number of channels: %w", io.ErrShortBuffer)
	}

	// Format sub-chunk sample rate
	n, err = reader.Read(f.FormatChunk.SampleRate[:])
	if err != nil {
		return fmt.Errorf("reading format sub-chunk: sample rate: %w", err)
	} else if n != len(f.FormatChunk.SampleRate) {
		return fmt.Errorf("reading format sub-chunk: sample rate: %w", io.ErrShortBuffer)
	}

	// Format sub-chunk byte rate
	n, err = reader.Read(f.FormatChunk.ByteRate[:])
	if err != nil {
		return fmt.Errorf("reading format sub-chunk: byte rate: %w", err)
	} else if n != len(f.FormatChunk.ByteRate) {
		return fmt.Errorf("reading format sub-chunk: byte rate: %w", io.ErrShortBuffer)
	}

	// Format sub-chunk block align
	n, err = reader.Read(f.FormatChunk.BlockAlign[:])
	if err != nil {
		return fmt.Errorf("reading format sub-chunk: block align: %w", err)
	} else if n != len(f.FormatChunk.BlockAlign) {
		return fmt.Errorf("reading format sub-chunk: block align: %w", io.ErrShortBuffer)
	}

	// Format sub-chunk bits per sample
	n, err = reader.Read(f.FormatChunk.BitsPerSample[:])
	if err != nil {
		return fmt.Errorf("reading format sub-chunk: bits per sample: %w", err)
	} else if n != len(f.FormatChunk.BitsPerSample) {
		return fmt.Errorf("reading format sub-chunk: bits per sample: %w", io.ErrShortBuffer)
	}

	if binary.LittleEndian.Uint16(f.FormatChunk.BitsPerSample[:])%8 != 0 {
		return ErrDecodeFormatBitsPerSample
	}

	switch binary.LittleEndian.Uint16(f.FormatChunk.Format[:]) {
	case FormatUnknown:
		return ErrDecodeFormat
	case FormatPCM:
		// PCM
		if formatSize != FormatChunkSizePCM {
			return ErrDecodeFormatSize
		}
	case FormatExtensible:
		// Extensible
		if formatSize != FormatChunkSizeExtensible {
			return ErrDecodeFormatSize
		}

		// Format sub-chunk extension size
		n, err = reader.Read(f.FormatChunk.ExtensionSize[:])
		if err != nil {
			return fmt.Errorf("reading format sub-chunk: extension size: %w", err)
		} else if n != len(f.FormatChunk.ExtensionSize) {
			return fmt.Errorf("reading format sub-chunk: extension size: %w", io.ErrShortBuffer)
		}

		if binary.LittleEndian.Uint16(f.FormatChunk.ExtensionSize[:]) != ExtensionSizeExtensible {
			return ErrDecodeFormatExtensionSize
		}

		// Format sub-chunk valid bits per sample
		n, err = reader.Read(f.FormatChunk.ValidBitsPerSample[:])
		if err != nil {
			return fmt.Errorf("reading format sub-chunk: valid bits per sample: %w", err)
		} else if n != len(f.FormatChunk.ValidBitsPerSample) {
			return fmt.Errorf("reading format sub-chunk: valid bits per sample: %w", io.ErrShortBuffer)
		}

		if binary.LittleEndian.Uint16(f.FormatChunk.ValidBitsPerSample[:]) > binary.LittleEndian.Uint16(f.FormatChunk.BitsPerSample[:]) {
			return ErrDecodeFormatValidBitsPerSample
		}

		// Format sub-chunk channel mask
		n, err = reader.Read(f.FormatChunk.ChannelMask[:])
		if err != nil {
			return fmt.Errorf("reading format sub-chunk: channel mask: %w", err)
		} else if n != len(f.FormatChunk.ChannelMask) {
			return fmt.Errorf("reading format sub-chunk: channel mask: %w", io.ErrShortBuffer)
		}

		// Format sub-chunk sub-format
		n, err = reader.Read(f.FormatChunk.SubFormat[:])
		if err != nil {
			return fmt.Errorf("reading format sub-chunk: sub-format: %w", err)
		} else if n != len(f.FormatChunk.SubFormat) {
			return fmt.Errorf("reading format sub-chunk: sub-format: %w", io.ErrShortBuffer)
		}

		if binary.LittleEndian.Uint16(f.FormatChunk.SubFormat[:2]) != binary.LittleEndian.Uint16(f.FormatChunk.Format[:]) {
			return ErrDecodeFormatSubFormat
		}

		// Fact sub-chunk ID
		n, err = reader.Read(f.FactChunk.Chunk.ID[:])
		if err != nil {
			return fmt.Errorf("reading fact sub-chunk: id: %w", err)
		} else if n != len(f.FormatChunk.Chunk.ID) {
			return fmt.Errorf("reading fact sub-chunk: id: %w", io.ErrShortBuffer)
		}

		if f.FactChunk.Chunk.ID != [4]byte{'f', 'a', 'c', 't'} {
			return ErrDecodeFactID
		}

		// Fact sub-chunk size
		n, err = reader.Read(f.FactChunk.Chunk.Size[:])
		if err != nil {
			return fmt.Errorf("reading fact sub-chunk: size: %w", err)
		} else if n != len(f.FormatChunk.Chunk.Size) {
			return fmt.Errorf("reading fact sub-chunk: size: %w", io.ErrShortBuffer)
		}

		if binary.LittleEndian.Uint32(f.FactChunk.Chunk.Size[:]) != FactChunkSize {
			return ErrDecodeFactSize
		}

		// Fact sub-chunk sample length
		n, err = reader.Read(f.FactChunk.SampleLength[:])
		if err != nil {
			return fmt.Errorf("reading fact sub-chunk: sample length: %w", err)
		} else if n != len(f.FactChunk.SampleLength) {
			return fmt.Errorf("reading fact sub-chunk: sample length: %w", io.ErrShortBuffer)
		}
	default:
		// Non-PCM
		if formatSize != FormatChunkSizeNonPCM {
			return ErrDecodeFormatSize
		}

		// Format sub-chunk extension size
		n, err = reader.Read(f.FormatChunk.ExtensionSize[:])
		if err != nil {
			return fmt.Errorf("reading format sub-chunk: extension size: %w", err)
		} else if n != len(f.FormatChunk.ExtensionSize) {
			return fmt.Errorf("reading format sub-chunk: extension size: %w", io.ErrShortBuffer)
		}

		if binary.LittleEndian.Uint16(f.FormatChunk.ExtensionSize[:]) != ExtensionSizeZero {
			return ErrDecodeFormatExtensionSize
		}

		// Fact sub-chunk ID
		n, err = reader.Read(f.FactChunk.Chunk.ID[:])
		if err != nil {
			return fmt.Errorf("reading fact sub-chunk: id: %w", err)
		} else if n != len(f.FormatChunk.Chunk.ID) {
			return fmt.Errorf("reading fact sub-chunk: id: %w", io.ErrShortBuffer)
		}

		if f.FactChunk.Chunk.ID != [4]byte{'f', 'a', 'c', 't'} {
			return ErrDecodeFactID
		}

		// Fact sub-chunk size
		n, err = reader.Read(f.FactChunk.Chunk.Size[:])
		if err != nil {
			return fmt.Errorf("reading fact sub-chunk: size: %w", err)
		} else if n != len(f.FormatChunk.Chunk.Size) {
			return fmt.Errorf("reading fact sub-chunk: size: %w", io.ErrShortBuffer)
		}

		if binary.LittleEndian.Uint32(f.FactChunk.Chunk.Size[:]) != FactChunkSize {
			return ErrDecodeFactSize
		}

		// Fact sub-chunk sample length
		n, err = reader.Read(f.FactChunk.SampleLength[:])
		if err != nil {
			return fmt.Errorf("reading fact sub-chunk: sample length: %w", err)
		} else if n != len(f.FactChunk.SampleLength) {
			return fmt.Errorf("reading fact sub-chunk: sample length: %w", io.ErrShortBuffer)
		}
	}

	// Data sub-chunk ID
	n, err = reader.Read(f.DataChunk.Chunk.ID[:])
	if err != nil {
		return fmt.Errorf("reading data sub-chunk: id: %w", err)
	} else if n != len(f.DataChunk.Chunk.ID) {
		return fmt.Errorf("reading data sub-chunk: id: %w", io.ErrShortBuffer)
	}

	if f.DataChunk.Chunk.ID != [4]byte{'d', 'a', 't', 'a'} {
		return ErrDecodeDataID
	}

	// Data sub-chunk size
	n, err = reader.Read(f.DataChunk.Chunk.Size[:])
	if err != nil {
		return fmt.Errorf("reading data sub-chunk: size: %w", err)
	} else if n != len(f.DataChunk.Chunk.Size) {
		return fmt.Errorf("reading data sub-chunk: size: %w", io.ErrShortWrite)
	}

	dataChunkSize := binary.LittleEndian.Uint32(f.DataChunk.Chunk.Size[:])

	if chuckSize != 4+(8+FormatChunkSizePCM)+(8+dataChunkSize) {
		return ErrDecodeRIFFSize
	}

	f.DataChunk.Data = make([]byte, dataChunkSize)

	// Data sub-chunk audio data
	n, err = reader.Read(f.DataChunk.Data)
	if err != nil {
		return fmt.Errorf("reading data sub-chunk: audio data: %w", err)
	} else if n != len(f.DataChunk.Data) {
		return fmt.Errorf("reading data sub-chunk: audio data: %w", io.ErrShortWrite)
	}

	if f.DataChunk.Data[len(f.DataChunk.Data)-1] == 0 {
		f.DataChunk.PaddingByte = 1
		// Discard last byte
		f.DataChunk.Data = f.DataChunk.Data[:len(f.DataChunk.Data)-1]
	}

	return nil
}

// TODO: implement extension encoding
func (f *WAVEFileFormat) Encode(writer io.Writer) error {
	// RIFF chuck ID
	n, err := writer.Write(f.RIFFChunk.Chunk.ID[:])
	if err != nil {
		return fmt.Errorf("writing riff chunk: id: %w", err)
	} else if n != len(f.RIFFChunk.Chunk.ID) {
		return fmt.Errorf("writing riff chunk: id: %w", io.ErrShortWrite)
	}

	// RIFF chuck size
	n, err = writer.Write(f.RIFFChunk.Chunk.Size[:])
	if err != nil {
		return fmt.Errorf("writing riff chunk: size: %w", err)
	} else if n != len(f.RIFFChunk.Chunk.Size) {
		return fmt.Errorf("writing riff chunk: size: %w", io.ErrShortWrite)
	}

	// RIFF identifier
	n, err = writer.Write(f.RIFFChunk.Identifier[:])
	if err != nil {
		return fmt.Errorf("writing riff chunk: identifier: %w", err)
	} else if n != len(f.RIFFChunk.Identifier) {
		return fmt.Errorf("writing riff chunk: identifier: %w", io.ErrShortWrite)
	}

	// Format sub-chunk ID
	n, err = writer.Write(f.FormatChunk.Chunk.ID[:])
	if err != nil {
		return fmt.Errorf("writing format sub-chunk: id: %w", err)
	} else if n != len(f.FormatChunk.Chunk.ID) {
		return fmt.Errorf("writing format sub-chunk: id: %w", io.ErrShortWrite)
	}

	// Format sub-chunk size
	n, err = writer.Write(f.FormatChunk.Chunk.Size[:])
	if err != nil {
		return fmt.Errorf("writing format sub-chunk: size: %w", err)
	} else if n != len(f.FormatChunk.Chunk.Size) {
		return fmt.Errorf("writing format sub-chunk: size: %w", io.ErrShortWrite)
	}

	formatSize := binary.LittleEndian.Uint32(f.FormatChunk.Chunk.Size[:])

	// Format sub-chunk audio format
	n, err = writer.Write(f.FormatChunk.Format[:])
	if err != nil {
		return fmt.Errorf("writing format sub-chunk: audio format: %w", err)
	} else if n != len(f.FormatChunk.Format) {
		return fmt.Errorf("writing format sub-chunk: audio format: %w", io.ErrShortWrite)
	}

	// Format sub-chunk number of channels
	n, err = writer.Write(f.FormatChunk.NumChannels[:])
	if err != nil {
		return fmt.Errorf("writing format sub-chunk: number of channels: %w", err)
	} else if n != len(f.FormatChunk.NumChannels) {
		return fmt.Errorf("writing format sub-chunk: number of channels: %w", io.ErrShortWrite)
	}

	// Format sub-chunk sample rate
	n, err = writer.Write(f.FormatChunk.SampleRate[:])
	if err != nil {
		return fmt.Errorf("writing format sub-chunk: sample rate: %w", err)
	} else if n != len(f.FormatChunk.SampleRate) {
		return fmt.Errorf("writing format sub-chunk: sample rate: %w", io.ErrShortWrite)
	}

	// Format sub-chunk byte rate
	n, err = writer.Write(f.FormatChunk.ByteRate[:])
	if err != nil {
		return fmt.Errorf("writing format sub-chunk: byte rate: %w", err)
	} else if n != len(f.FormatChunk.ByteRate) {
		return fmt.Errorf("writing format sub-chunk: byte rate: %w", io.ErrShortWrite)
	}

	// Format sub-chunk block align
	n, err = writer.Write(f.FormatChunk.BlockAlign[:])
	if err != nil {
		return fmt.Errorf("writing format sub-chunk: block align: %w", err)
	} else if n != len(f.FormatChunk.BlockAlign) {
		return fmt.Errorf("writing format sub-chunk: block align: %w", io.ErrShortWrite)
	}

	// Format sub-chunk bits per sample
	n, err = writer.Write(f.FormatChunk.BitsPerSample[:])
	if err != nil {
		return fmt.Errorf("writing format sub-chunk: bits per sample: %w", err)
	} else if n != len(f.FormatChunk.BitsPerSample) {
		return fmt.Errorf("writing format sub-chunk: bits per sample: %w", io.ErrShortWrite)
	}

	switch binary.LittleEndian.Uint16(f.FormatChunk.Format[:]) {
	case FormatUnknown:
		panic("unknown audio format")
	case FormatPCM:
		// PCM
		if formatSize != FormatChunkSizePCM {
			return ErrDecodeFormatSize
		}
	case FormatExtensible:
		// Extensible
		if formatSize != FormatChunkSizeExtensible {
			return ErrDecodeFormatSize
		}

		// Format sub-chunk extension size
		n, err = writer.Write(f.FormatChunk.ExtensionSize[:])
		if err != nil {
			return fmt.Errorf("writing format sub-chunk: extension size: %w", err)
		} else if n != len(f.FormatChunk.ExtensionSize) {
			return fmt.Errorf("writing format sub-chunk: extension size: %w", io.ErrShortWrite)
		}

		// Format sub-chunk valid bits per sample
		n, err = writer.Write(f.FormatChunk.ValidBitsPerSample[:])
		if err != nil {
			return fmt.Errorf("writing format sub-chunk: valid bits per sample: %w", err)
		} else if n != len(f.FormatChunk.ValidBitsPerSample) {
			return fmt.Errorf("writing format sub-chunk: valid bits per sample: %w", io.ErrShortWrite)
		}

		// Format sub-chunk channel mask
		n, err = writer.Write(f.FormatChunk.ChannelMask[:])
		if err != nil {
			return fmt.Errorf("writing format sub-chunk: channel mask: %w", err)
		} else if n != len(f.FormatChunk.ChannelMask) {
			return fmt.Errorf("writing format sub-chunk: channel mask: %w", io.ErrShortWrite)
		}

		// Format sub-chunk sub-format
		n, err = writer.Write(f.FormatChunk.SubFormat[:])
		if err != nil {
			return fmt.Errorf("writing format sub-chunk: sub-format: %w", err)
		} else if n != len(f.FormatChunk.SubFormat) {
			return fmt.Errorf("writing format sub-chunk: sub-format: %w", io.ErrShortWrite)
		}

		// Fact sub-chunk ID
		n, err = writer.Write(f.FactChunk.Chunk.ID[:])
		if err != nil {
			return fmt.Errorf("writing fact sub-chunk: id: %w", err)
		} else if n != len(f.FormatChunk.Chunk.ID) {
			return fmt.Errorf("writing fact sub-chunk: id: %w", io.ErrShortWrite)
		}

		// Fact sub-chunk size
		n, err = writer.Write(f.FactChunk.Chunk.Size[:])
		if err != nil {
			return fmt.Errorf("writing fact sub-chunk: size: %w", err)
		} else if n != len(f.FormatChunk.Chunk.Size) {
			return fmt.Errorf("writing fact sub-chunk: size: %w", io.ErrShortWrite)
		}

		// Fact sub-chunk sample length
		n, err = writer.Write(f.FactChunk.SampleLength[:])
		if err != nil {
			return fmt.Errorf("writing fact sub-chunk: sample length: %w", err)
		} else if n != len(f.FactChunk.SampleLength) {
			return fmt.Errorf("writing fact sub-chunk: sample length: %w", io.ErrShortWrite)
		}
	default:
		// Non-PCM
		if formatSize != FormatChunkSizeNonPCM {
			return ErrDecodeFormatSize
		}

		// Format sub-chunk extension size
		n, err = writer.Write(f.FormatChunk.ExtensionSize[:])
		if err != nil {
			return fmt.Errorf("writer format sub-chunk: extension size: %w", err)
		} else if n != len(f.FormatChunk.ExtensionSize) {
			return fmt.Errorf("writer format sub-chunk: extension size: %w", io.ErrShortWrite)
		}

		// Fact sub-chunk ID
		n, err = writer.Write(f.FactChunk.Chunk.ID[:])
		if err != nil {
			return fmt.Errorf("writer fact sub-chunk: id: %w", err)
		} else if n != len(f.FormatChunk.Chunk.ID) {
			return fmt.Errorf("writer fact sub-chunk: id: %w", io.ErrShortWrite)
		}

		// Fact sub-chunk size
		n, err = writer.Write(f.FactChunk.Chunk.Size[:])
		if err != nil {
			return fmt.Errorf("writer fact sub-chunk: size: %w", err)
		} else if n != len(f.FormatChunk.Chunk.Size) {
			return fmt.Errorf("writer fact sub-chunk: size: %w", io.ErrShortWrite)
		}

		// Fact sub-chunk sample length
		n, err = writer.Write(f.FactChunk.SampleLength[:])
		if err != nil {
			return fmt.Errorf("writer fact sub-chunk: sample length: %w", err)
		} else if n != len(f.FactChunk.SampleLength) {
			return fmt.Errorf("writer fact sub-chunk: sample length: %w", io.ErrShortWrite)
		}
	}

	// Data sub-chunk ID
	n, err = writer.Write(f.DataChunk.Chunk.ID[:])
	if err != nil {
		return fmt.Errorf("writing data sub-chunk: id: %w", err)
	} else if n != len(f.DataChunk.Chunk.ID) {
		return fmt.Errorf("writing data sub-chunk: id: %w", io.ErrShortWrite)
	}

	// Data sub-chunk size
	n, err = writer.Write(f.DataChunk.Chunk.Size[:])
	if err != nil {
		return fmt.Errorf("writing data sub-chunk: size: %w", err)
	} else if n != len(f.DataChunk.Chunk.Size) {
		return fmt.Errorf("writing data sub-chunk: size: %w", io.ErrShortWrite)
	}

	// Data sub-chunk audio data
	n, err = writer.Write(f.DataChunk.Data)
	if err != nil {
		return fmt.Errorf("writing data sub-chunk: audio data: %w", err)
	} else if n != len(f.DataChunk.Data) {
		return fmt.Errorf("writing data sub-chunk: audio data: %w", io.ErrShortWrite)
	}

	return nil
}

func (f *WAVEFileFormat) Size() int {
	return int(binary.LittleEndian.Uint32(f.RIFFChunk.Chunk.Size[:]))
}
