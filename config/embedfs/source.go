package embedfs

import (
	"io/fs"
	"path"
	"strings"

	"github.com/2comjie/wali/config"
)

type source struct {
	fs   fs.FS
	path string
}

func NewSource(fileSystem fs.FS, sourcePath string) config.Source {
	return &source{fs: fileSystem, path: sourcePath}
}

func (s *source) Load() ([]config.Document, error) {
	info, err := fs.Stat(s.fs, s.path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		document, err := s.loadFile(s.path, "")
		if err != nil {
			return nil, err
		}
		return []config.Document{document}, nil
	}

	documents := make([]config.Document, 0)
	err = fs.WalkDir(s.fs, s.path, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if currentPath != s.path && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
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
		root := path.Clean(s.path)
		relativePath := currentPath
		if root != "." {
			relativePath = strings.TrimPrefix(currentPath, root+"/")
			if relativePath == currentPath {
				return fs.ErrInvalid
			}
		}
		document, err := s.loadFile(currentPath, relativePath)
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
	return nil, config.ErrWatchUnsupported
}

func (s *source) loadFile(filePath, relativePath string) (config.Document, error) {
	data, err := fs.ReadFile(s.fs, filePath)
	if err != nil {
		return config.Document{}, err
	}
	extension := path.Ext(filePath)
	return config.Document{
		Path:   relativePath,
		Format: strings.ToLower(strings.TrimPrefix(extension, ".")),
		Data:   data,
	}, nil
}
