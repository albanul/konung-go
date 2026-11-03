package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

// DecodeRLEFrame decodes a Konung Type 5 RLE-compressed sprite frame into an RGBA image.
// Format:
//   Width:       uint16 (Little-Endian)
//   Height:      uint16 (Little-Endian)
//   RowSizes:    [Height]uint16 (Little-Endian) - byte size of each compressed row
//   RowData:     Byte stream of RLE commands per row
//     cmd == 0:  End of row
//     cmd < 0:   Transparent run: skip (cmd & 0x7F) pixels
//     cmd > 0:   Opaque run: read `cmd` 16-bit RGB 555 pixels (0RRRRRGGGGGBBBBB)
func DecodeRLEFrame(frameData []byte) (*image.RGBA, error) {
	if len(frameData) < 4 {
		return nil, fmt.Errorf("данные кадра слишком малы для заголовка")
	}

	width := int(binary.LittleEndian.Uint16(frameData[0:2]))
	height := int(binary.LittleEndian.Uint16(frameData[2:4]))

	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("недопустимые размеры кадра: %dx%d", width, height)
	}

	tableOffset := 4
	tableSize := height * 2
	if len(frameData) < tableOffset+tableSize {
		return nil, fmt.Errorf("данные кадра меньше размера таблицы строк")
	}

	rowSizes := make([]int, height)
	for i := 0; i < height; i++ {
		rowSizes[i] = int(binary.LittleEndian.Uint16(frameData[tableOffset+i*2 : tableOffset+i*2+2]))
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	dataOffset := tableOffset + tableSize
	for y := 0; y < height; y++ {
		sz := rowSizes[y]
		if dataOffset+sz > len(frameData) {
			return nil, fmt.Errorf("строка %d выходит за границы данных (смещение %d, размер %d, всего %d)",
				y, dataOffset, sz, len(frameData))
		}

		rowData := frameData[dataOffset : dataOffset+sz]
		dataOffset += sz

		x := 0
		idx := 0
		for idx < len(rowData) {
			cmd := int8(rowData[idx])
			idx++

			if cmd == 0 {
				// Конец строки
				break
			} else if cmd < 0 {
				// Пропуск прозрачных пикселей: младшие 7 бит
				skip := int(cmd & 0x7F)
				x += skip
			} else {
				// Непрозрачные пиксели: count пикселей по 2 байта (RGB 555)
				count := int(cmd)
				for i := 0; i < count; i++ {
					if idx+2 > len(rowData) {
						break
					}
					val := binary.LittleEndian.Uint16(rowData[idx : idx+2])
					idx += 2

					// Формат 15-bit RGB 555 (0_RRRRR_GGGGG_BBBBB):
					// Красный: биты 14..10 (5 бит) -> [0..255]
					// Зеленый: биты  9..5  (5 бит) -> [0..255]
					// Синий:   биты  4..0  (5 бит) -> [0..255]
					r := uint8(((val >> 10) & 0x1F) * 255 / 31)
					g := uint8(((val >> 5) & 0x1F) * 255 / 31)
					b := uint8((val & 0x1F) * 255 / 31)

					if x < width {
						img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
					}
					x++
				}
			}
		}
	}

	return img, nil
}

// UnpackGraphicsRES распаковывает графические ресурсы Type 5 (FLAMES.RES, MENU640R.RES и др.)
func UnpackGraphicsRES(file *os.File, outputDir string) error {
	var frameIndex int

	for {
		var header struct {
			Tag  uint32 // 0x00000005
			Size uint32 // Размер сжатых данных кадра (Little-Endian)
		}

		if err := binary.Read(file, binary.BigEndian, &header.Tag); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("ошибка чтения тега кадра %d: %w", frameIndex, err)
		}

		if err := binary.Read(file, binary.LittleEndian, &header.Size); err != nil {
			return fmt.Errorf("ошибка чтения размера кадра %d: %w", frameIndex, err)
		}

		if header.Tag != 5 {
			return fmt.Errorf("кадр %d: неожиданный тег 0x%X (ожидался 5)", frameIndex, header.Tag)
		}

		frameData := make([]byte, header.Size)
		if _, err := io.ReadFull(file, frameData); err != nil {
			return fmt.Errorf("ошибка чтения данных кадра %d (%d байт): %w", frameIndex, header.Size, err)
		}

		img, err := DecodeRLEFrame(frameData)
		if err != nil {
			return fmt.Errorf("ошибка декодирования кадра %d: %w", frameIndex, err)
		}

		outName := fmt.Sprintf("frame_%03d.png", frameIndex)
		outPath := filepath.Join(outputDir, outName)

		outFile, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("не удалось создать файл %s: %w", outPath, err)
		}

		if err := png.Encode(outFile, img); err != nil {
			outFile.Close()
			return fmt.Errorf("не удалось сохранить PNG %s: %w", outPath, err)
		}
		outFile.Close()

		bounds := img.Bounds()
		fmt.Printf("Кадр %2d: %dx%d (сжатый размер: %6d байт) -> %s\n",
			frameIndex, bounds.Dx(), bounds.Dy(), header.Size, outName)

		frameIndex++
	}

	fmt.Printf("Успешно распаковано %d кадров в папку %s\n", frameIndex, outputDir)
	return nil
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Использование: go run cmd/res-parser/main.go <путь_к_архиву.RES> <директория_вывода>")
		return
	}

	resPath := os.Args[1]
	outDir := os.Args[2]

	file, err := os.Open(resPath)
	if err != nil {
		fmt.Printf("Не удалось открыть файл: %v\n", err)
		return
	}
	defer file.Close()

	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("Не удалось создать директорию: %v\n", err)
		return
	}

	fmt.Printf("Начинаем разбор ресурса: %s\n", resPath)
	if err := UnpackGraphicsRES(file, outDir); err != nil {
		fmt.Printf("Ошибка при распаковке: %v\n", err)
	} else {
		fmt.Println("Распаковка успешно завершена!")
	}
}
