package embedfs

import (
	"io/fs"
	"path"
	"strings"

	"github.com/2comjie/wali/config"
)

var _ config.Source = (*source)(nil)

type source struct {
	fs   fs.FS
	path string
}

// NewSource 创建一个只读的内嵌配置源。
func NewSource(fsys fs.FS, sourcePath string) config.Source {
	return &source{fs: fsys, path: sourcePath}
}

func (s *source) Load() ([]*config.KeyValue, error) {
	info, err := fs.Stat(s.fs, s.path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		value, err := s.loadFile(s.path)
		if err != nil {
			return nil, err
		}
		return []*config.KeyValue{value}, nil
	}

	entries, err := fs.ReadDir(s.fs, s.path)
	if err != nil {
		return nil, err
	}
	values := make([]*config.KeyValue, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		value, err := s.loadFile(path.Join(s.path, entry.Name()))
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func (s *source) Watch() (config.Watcher, error) {
	return nil, config.ErrWatchUnsupported
}

func (s *source) loadFile(filePath string) (*config.KeyValue, error) {
	data, err := fs.ReadFile(s.fs, filePath)
	if err != nil {
		return nil, err
	}
	name := path.Base(filePath)
	return &config.KeyValue{
		Key:    name,
		Format: format(name),
		Value:  data,
	}, nil
}

func format(name string) string {
	if index := strings.LastIndexByte(name, '.'); index >= 0 {
		return name[index+1:]
	}
	return ""
}
