package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/rs/zerolog/log"
)

type MediaTarget struct {
	Location tg.InputFileLocationClass
	FileName string
	Size     int64
	MimeType string
}

// 1. Struct disinkronkan persis dengan permintaan dispatcher
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

func New(client *tg.Client) *Downloader {
	d := downloader.NewDownloader().WithPartSize(512 * 1024)
	return &Downloader{
		client:     client,
		downloader: d,
	}
}

// ResolveMedia mengekstrak info file & lokasi
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

		if fileName == "" {
			fileName = fmt.Sprintf("doc_%d.bin", doc.ID)
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

// 2. Signature fungsi disinkronkan menjadi 3 argumen (tanpa outputDir string)
func (d *Downloader) DownloadStreams(ctx context.Context, target *MediaTarget, progress func(p DownloadProgress)) (string, error) {
	if target == nil || target.Location == nil {
		return "", errors.New("target media invalid")
	}

	outputDir := "downloads"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("gagal membuat direktori download: %w", err)
	}

	fileName := target.FileName
	if fileName == "" {
		fileName = fmt.Sprintf("file_%d.bin", time.Now().Unix())
	}

	destPath := filepath.Join(outputDir, fileName)

	log.Info().
		Str("filename", fileName).
		Int64("size", target.Size).
		Msg("Memulai stream download...")

	builder := d.downloader.Download(d.client, target.Location)

	// Goroutine Async Poller untuk kalkulasi SpeedBytesSec dan Percentage
	done := make(chan struct{})
	if progress != nil {
		go func() {
			startTime := time.Now()
			for {
				select {
				case <-done:
					return
				case <-time.After(1 * time.Second):
					info, err := os.Stat(destPath)
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

	if _, err := builder.ToPath(ctx, destPath); err != nil {
		close(done)
		log.Error().Err(err).Str("file", destPath).Msg("Stream download terputus")
		return "", err
	}
	close(done)

	if progress != nil {
		progress(DownloadProgress{
			DownloadedBytes: target.Size,
			TotalBytes:      target.Size,
			Percentage:      100.0,
			SpeedBytesSec:   0,
		})
	}

	log.Info().Str("path", destPath).Msg("Download selesai")
	return destPath, nil
}
