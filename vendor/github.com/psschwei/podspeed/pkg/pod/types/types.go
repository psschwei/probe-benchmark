package types

import (
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/psschwei/podspeed/pkg/pod/template"
	corev1 "k8s.io/api/core/v1"
)

//go:embed manifests
var fs embed.FS

const folder = "manifests"

func Names() ([]string, error) {
	files, err := fs.ReadDir(folder)
	if err != nil {
		return nil, fmt.Errorf("failed to read built in types: %w", err)
	}

	names := make([]string, 0, len(files))
	for _, file := range files {
		if !file.IsDir() {
			names = append(names, fileNameWithoutExtension(file.Name()))
		}
	}
	sort.Strings(names)
	return names, nil
}

func GetConstructor(name string) (func(string, string) *corev1.Pod, error) {
	file, err := fs.Open(filepath.Join(folder, name+".yaml"))
	if err != nil {
		return nil, fmt.Errorf("failed to open built in template: %w", err)
	}
	defer file.Close()

	return template.PodConstructorFromYAML(file)
}

func fileNameWithoutExtension(fileName string) string {
	if pos := strings.LastIndexByte(fileName, '.'); pos != -1 {
		return fileName[:pos]
	}
	return fileName
}
