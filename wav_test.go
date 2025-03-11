package wav_test

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/samborkent/wav"
)

func TestWav(t *testing.T) {
	file, err := os.OpenFile("../../assets/break.wav", os.O_RDONLY, 0)
	if err != nil {
		t.Errorf("opening file: %s", err.Error())
		return
	}

	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("closing file: %s", err.Error())
		}
	}()

	waveFile := &wav.WAVEFileFormat{}

	if err := waveFile.Decode(file); err != nil {
		t.Errorf("decoding wav file: %s", err.Error())
		return
	}

	outputFile := new(bytes.Buffer)

	if err := waveFile.Encode(outputFile); err != nil {
		t.Errorf("encoding wav file: %s", err.Error())
		return
	}

	outputBuffer := make([]byte, outputFile.Len())

	if _, err := outputFile.Read(outputBuffer); err != nil {
		t.Errorf("reading output audio file: %s", err.Error())
		return
	}

	if err := os.WriteFile(fmt.Sprintf("../../../tmp/rec_%s.wav", time.Now().Format("2006-01-02-15-04-05")), outputBuffer, 0o644); err != nil {
		t.Errorf("writing audio file: %s", err.Error())
		return
	}
}
