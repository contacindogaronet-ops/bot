package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type MediaTarget struct {
	Location tg.InputFileLocationClass
	FileName string
	Size     int64
	MimeType string
}

type DownloadProgress struct {
	DownloadedBytes int64
	TotalBytes      int64
	Percentage      float64
	SpeedBytesSec   int64
}

type Downloader struct {
	client     *tg.Client
	downloader *downloader.Downloader
}

func New(client *tg.Client, outputDir string, threads int, partSize int, logger zerolog.Logger) (*Downloader, error) {
	d := downloader.NewDownloader().WithPartSize(512 * 1024)
	return &Downloader{
		client:     client,
		downloader: d,
	}, nil
}

func (d *Downloader) ResolveMedia(media tg.MessageMediaClass) (*MediaTarget, error) {
	if media == nil {
		return nil, errors.New("pesan tidak memiliki media")
	}

	switch m := media.(type) {
	case *tg.MessageMediaDocument:
		doc, ok := m.Document.(*tg.Document)
		if !ok {
			return nil, errors.New("format dokumen kosong")
		}

		var fileName string
		for _, attr := range doc.Attributes {
			if fn, ok := attr.(*tg.DocumentAttributeFilename); ok {
				fileName = fn.FileName
				break
			}
		}

		// Fix Ekstensi: Deteksi MimeType jika nama file disembunyikan Telegram
		if fileName == "" {
			ext := ".bin"
			if doc.MimeType == "video/mp4" {
				ext = ".mp4"
			} else if doc.MimeType == "video/webm" {
				ext = ".webm"
			} else if strings.HasPrefix(doc.MimeType, "image/") {
				ext = ".jpg"
			}
			fileName = fmt.Sprintf("vid_%d%s", doc.ID, ext)
		}

		return &MediaTarget{
			Location: &tg.InputDocumentFileLocation{
				ID:            doc.ID,
				AccessHash:    doc.AccessHash,
				FileReference: doc.FileReference,
				ThumbSize:     "",
			},
			FileName: fileName,
			Size:     doc.Size,
			MimeType: doc.MimeType,
		}, nil

	case *tg.MessageMediaPhoto:
		photo, ok := m.Photo.(*tg.Photo)
		if !ok {
			return nil, errors.New("format foto kosong")
		}

		thumbType := "y"
		for _, s := range photo.Sizes {
			switch sz := s.(type) {
			case *tg.PhotoSize:
				thumbType = sz.Type
			case *tg.PhotoSizeProgressive:
				thumbType = sz.Type
			}
		}

		return &MediaTarget{
			Location: &tg.InputPhotoFileLocation{
				ID:            photo.ID,
				AccessHash:    photo.AccessHash,
				FileReference: photo.FileReference,
				ThumbSize:     thumbType,
			},
			FileName: fmt.Sprintf("photo_%d.jpg", photo.ID),
			Size:     0,
			MimeType: "image/jpeg",
		}, nil

	default:
		return nil, errors.New("tipe media tidak didukung untuk bypass")
	}
}

func (d *Downloader) DownloadStreams(ctx context.Context, target *MediaTarget, progress func(p DownloadProgress)) (string, error) {
	if target == nil || target.Location == nil {
		return "", errors.New("target media invalid")
	}

	// 1. Integrasi .env dengan Fallback
	outputDir := os.Getenv("TG_DOWNLOAD_DIR")
	if outputDir == "" {
		outputDir = "downloads"
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("gagal membuat direktori download: %w", err)
	}

	fileName := target.FileName
	finalPath := filepath.Join(outputDir, fileName)
	
	// 2. Sistem Part/Cache: Tambahkan ekstensi .part selama download
	partPath := finalPath + ".part"

	log.Info().
		Str("filename", fileName).
		Str("dir", outputDir).
		Int64("size", target.Size).
		Msg("Memulai stream download caching...")

	builder := d.downloader.Download(d.client, target.Location)

	done := make(chan struct{})
	if progress != nil {
		go func() {
			startTime := time.Now()
			for {
				select {
				case <-done:
					return
				case <-time.After(1 * time.Second):
					info, err := os.Stat(partPath)
					if err == nil {
						dl := info.Size()
						elapsed := time.Since(startTime).Seconds()
						var speed int64
						if elapsed > 0 {
							speed = int64(float64(dl) / elapsed)
						}
						var pct float64
						if target.Size > 0 {
							pct = (float64(dl) / float64(target.Size)) * 100.0
						}
						progress(DownloadProgress{
							DownloadedBytes: dl,
							TotalBytes:      target.Size,
							Percentage:      pct,
							SpeedBytesSec:   speed,
						})
					}
				}
			}
		}()
	}

	// Stream I/O langsung ke file .part
	if _, err := builder.ToPath(ctx, partPath); err != nil {
		close(done)
		os.Remove(partPath) // Hapus cache .part jika koneksi putus
		log.Error().Err(err).Str("file", partPath).Msg("Stream terputus")
		return "", err
	}
	close(done)

	// 3. Konversi Cache: Rename .part ke file final secara instan
	if err := os.Rename(partPath, finalPath); err != nil {
		log.Error().Err(err).Msg("Gagal menyimpan cache ke file final")
		return "", err
	}

	if progress != nil {
		progress(DownloadProgress{
			DownloadedBytes: target.Size,
			TotalBytes:      target.Size,
			Percentage:      100.0,
			SpeedBytesSec:   0,
		})
	}

	log.Info().Str("path", finalPath).Msg("Download selesai dan cache dihapus")
	return finalPath, nil
}
