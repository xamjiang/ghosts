package ports

import (
	"os"
	"path/filepath"
	"strings"
)

func classify(exe string) Kind {
	path := strings.ToLower(filepath.Clean(exe))
	if exe == "" {
		return KindApp
	}
	under := func(env string) bool {
		root := os.Getenv(env)
		return root != "" && strings.HasPrefix(path, strings.ToLower(filepath.Clean(root))+string(filepath.Separator))
	}
	if under("SystemRoot") {
		return KindSystem
	}
	name := strings.TrimSuffix(filepath.Base(path), ".exe")
	if _, ok := interpreters[name]; ok || pythonVersioned.MatchString(name) || serviceNames[name] != "" {
		return KindDev
	}
	if under("ProgramFiles") || under("ProgramFiles(x86)") || strings.Contains(path, `\appdata\local\programs\`) {
		return KindApp
	}
	return KindDev
}
