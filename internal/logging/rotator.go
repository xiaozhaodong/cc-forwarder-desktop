package logging

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FileRotator manages log file rotation and archival
type FileRotator struct {
	filename    string // Base filename for the log
	maxSize     int64  // Maximum size in bytes before rotation
	maxFiles    int    // Maximum number of rotated files to keep
	compress    bool   // Whether to compress rotated files
	currentFile *os.File
	currentSize int64
	mutex       sync.Mutex
}

// NewFileRotator creates a new file rotator
func NewFileRotator(filename string, maxSize int64, maxFiles int, compress bool) (*FileRotator, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	fr := &FileRotator{
		filename: filename,
		maxSize:  maxSize,
		maxFiles: maxFiles,
		compress: compress,
	}

	// Open initial file
	if err := fr.openFile(); err != nil {
		return nil, err
	}

	return fr, nil
}

// Write implements io.Writer interface
func (fr *FileRotator) Write(p []byte) (int, error) {
	fr.mutex.Lock()
	defer fr.mutex.Unlock()

	// Check if we need to rotate
	if fr.currentSize+int64(len(p)) > fr.maxSize {
		if err := fr.rotate(); err != nil {
			return 0, fmt.Errorf("failed to rotate log file: %w", err)
		}
	}

	// Write to current file
	n, err := fr.currentFile.Write(p)
	if err != nil {
		return n, err
	}

	fr.currentSize += int64(n)
	return n, nil
}

// Close closes the current log file
func (fr *FileRotator) Close() error {
	fr.mutex.Lock()
	defer fr.mutex.Unlock()

	if fr.currentFile != nil {
		return fr.currentFile.Close()
	}
	return nil
}

// Sync syncs the current log file to disk
func (fr *FileRotator) Sync() error {
	fr.mutex.Lock()
	defer fr.mutex.Unlock()

	if fr.currentFile != nil {
		return fr.currentFile.Sync()
	}
	return nil
}

// openFile opens or creates the log file
func (fr *FileRotator) openFile() error {
	file, err := os.OpenFile(fr.filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Get current file size
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return fmt.Errorf("failed to get log file info: %w", err)
	}

	fr.currentFile = file
	fr.currentSize = info.Size()
	return nil
}

// rotate rotates the current log file
func (fr *FileRotator) rotate() error {
	// Close current file
	if fr.currentFile != nil {
		fr.currentFile.Close()
		fr.currentFile = nil
	}

	// Generate timestamp for rotated file
	timestamp := time.Now().Format("2006-01-02-15-04-05")

	// Move current file to rotated name
	rotatedName := fr.filename + "." + timestamp
	if err := os.Rename(fr.filename, rotatedName); err != nil {
		return fmt.Errorf("failed to rename log file: %w", err)
	}

	// Compress rotated file if enabled
	if fr.compress {
		go fr.compressFile(rotatedName)
	}

	// Clean up old files
	go fr.cleanupOldFiles()

	// Open new file
	return fr.openFile()
}

// compressFile compresses a rotated log file
func (fr *FileRotator) compressFile(filename string) {
	src, err := os.Open(filename)
	if err != nil {
		return
	}
	defer src.Close()

	dst, err := os.Create(filename + ".gz")
	if err != nil {
		return
	}
	defer dst.Close()

	gw := gzip.NewWriter(dst)
	defer gw.Close()

	// Copy file content to gzip writer
	if _, err := io.Copy(gw, src); err != nil {
		return
	}

	// Remove original file after successful compression
	os.Remove(filename)
}

// cleanupOldFiles removes old rotated files beyond maxFiles limit
func (fr *FileRotator) cleanupOldFiles() {
	if fr.maxFiles <= 0 {
		return
	}

	dir := filepath.Dir(fr.filename)
	base := filepath.Base(fr.filename)

	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Collect rotated files
	var rotatedFiles []os.DirEntry
	for _, file := range files {
		if strings.HasPrefix(file.Name(), base+".") {
			rotatedFiles = append(rotatedFiles, file)
		}
	}

	// Sort by modification time (newest first)
	sort.Slice(rotatedFiles, func(i, j int) bool {
		infoI, errI := rotatedFiles[i].Info()
		infoJ, errJ := rotatedFiles[j].Info()
		if errI != nil || errJ != nil {
			return false
		}
		return infoI.ModTime().After(infoJ.ModTime())
	})

	// Remove files beyond maxFiles limit
	for i := fr.maxFiles; i < len(rotatedFiles); i++ {
		os.Remove(filepath.Join(dir, rotatedFiles[i].Name()))
	}
}

// ParseSize parses size string like "100MB", "1GB" into bytes
func ParseSize(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(strings.ToUpper(sizeStr))

	var multiplier int64 = 1
	var numStr string

	if strings.HasSuffix(sizeStr, "KB") {
		multiplier = 1024
		numStr = strings.TrimSuffix(sizeStr, "KB")
	} else if strings.HasSuffix(sizeStr, "MB") {
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(sizeStr, "MB")
	} else if strings.HasSuffix(sizeStr, "GB") {
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(sizeStr, "GB")
	} else if strings.HasSuffix(sizeStr, "B") {
		multiplier = 1
		numStr = strings.TrimSuffix(sizeStr, "B")
	} else {
		// No suffix, assume bytes
		numStr = sizeStr
	}

	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size format: %s", sizeStr)
	}

	return num * multiplier, nil
}
