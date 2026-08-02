//go:build ignore

// Command generate-icons derives platform packaging icons from ciwi's pixel-art
// favicon. It deliberately uses nearest-neighbour scaling so the source art
// remains crisp at desktop icon sizes.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"

	"golang.org/x/image/draw"
)

func main() {
	sourcePath := flag.String("source", "internal/server/webui/assets/ciwi-favicon.png", "source PNG")
	pngPath := flag.String("png", "packaging/icons/ciwi.png", "output square PNG")
	icoPath := flag.String("ico", "packaging/icons/ciwi.ico", "output Windows ICO")
	flag.Parse()

	sourceFile, err := os.Open(*sourcePath)
	check(err)
	source, err := png.Decode(sourceFile)
	_ = sourceFile.Close()
	check(err)

	check(os.MkdirAll("packaging/icons", 0o755))
	check(writePNG(*pngPath, squareIcon(source, 1024)))
	check(writeICO(*icoPath, source, []int{16, 24, 32, 48, 64, 128, 256}))
}

func squareIcon(source image.Image, size int) *image.NRGBA {
	destination := image.NewNRGBA(image.Rect(0, 0, size, size))
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	scaledWidth, scaledHeight := size, size
	if width > height {
		scaledHeight = height * size / width
	} else {
		scaledWidth = width * size / height
	}
	x := (size - scaledWidth) / 2
	y := (size - scaledHeight) / 2
	draw.NearestNeighbor.Scale(destination, image.Rect(x, y, x+scaledWidth, y+scaledHeight), source, bounds, draw.Src, nil)
	return destination
}

func writePNG(path string, icon image.Image) error {
	output, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(output, icon); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func writeICO(path string, source image.Image, sizes []int) error {
	type encodedIcon struct {
		size int
		data []byte
	}
	icons := make([]encodedIcon, 0, len(sizes))
	for _, size := range sizes {
		file, err := os.CreateTemp("", "ciwi-icon-*.png")
		if err != nil {
			return err
		}
		temporaryPath := file.Name()
		if err := png.Encode(file, squareIcon(source, size)); err != nil {
			_ = file.Close()
			_ = os.Remove(temporaryPath)
			return err
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(temporaryPath)
			return err
		}
		payload, err := os.ReadFile(temporaryPath)
		_ = os.Remove(temporaryPath)
		if err != nil {
			return err
		}
		icons = append(icons, encodedIcon{size: size, data: payload})
	}

	output, err := os.Create(path)
	if err != nil {
		return err
	}
	defer output.Close()
	checkWrite := func(value any) error { return binary.Write(output, binary.LittleEndian, value) }
	if err := checkWrite([3]uint16{0, 1, uint16(len(icons))}); err != nil {
		return err
	}
	offset := uint32(6 + 16*len(icons))
	for _, icon := range icons {
		dimension := uint8(icon.size)
		if icon.size == 256 {
			dimension = 0
		}
		entry := struct {
			Width, Height, ColorCount, Reserved uint8
			Planes, BitCount                    uint16
			BytesInResource, ImageOffset        uint32
		}{dimension, dimension, 0, 0, 1, 32, uint32(len(icon.data)), offset}
		if err := checkWrite(entry); err != nil {
			return err
		}
		offset += uint32(len(icon.data))
	}
	for _, icon := range icons {
		if _, err := output.Write(icon.data); err != nil {
			return err
		}
	}
	return nil
}

func check(err error) {
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
