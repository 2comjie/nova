package file

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/2comjie/nova/config"
)

type source struct {
	path string
}

func NewSource(sourcePath string) config.Source {
	return &source{path: sourcePath}
}

func (s *source) Load() ([]config.Document, error) {
	info, err := os.Stat(s.path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		document, err := loadFile(s.path, "")
		if err != nil {
			return nil, err
		}
		return []config.Document{document}, nil
	}

	documents := make([]config.Document, 0)
	err = filepath.WalkDir(s.path, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if currentPath != s.path && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if !entryInfo.Mode().IsRegular() {
			return nil
		}
		relativePath, err := filepath.Rel(s.path, currentPath)
		if err != nil {
			return err
		}
		document, err := loadFile(currentPath, filepath.ToSlash(relativePath))
		if err != nil {
			return err
		}
		documents = append(documents, document)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return documents, nil
}

func (s *source) Watch() (config.Watcher, error) {
	return newWatcher(s.path)
}

func loadFile(filePath, relativePath string) (config.Document, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return config.Document{}, err
	}
	extension := filepath.Ext(filePath)
	return config.Document{
		Path:   relativePath,
		Format: strings.ToLower(strings.TrimPrefix(extension, ".")),
		Data:   data,
	}, nil
}
