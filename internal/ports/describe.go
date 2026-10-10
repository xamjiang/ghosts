package ports

import (
	"path/filepath"
	"regexp"
	"strings"
)

// interpreters maps executable names to the runtime label we show. The real
// "what is this" answer for these lives in their arguments.
var interpreters = map[string]string{
	"node": "node", "nodejs": "node", "bun": "bun", "deno": "deno",
	"python": "python", "python3": "python", "ruby": "ruby", "php": "php",
	"perl": "perl", "java": "java", "dotnet": "dotnet", "elixir": "elixir",
	"beam.smp": "erlang",
}

var pythonVersioned = regexp.MustCompile(`^python3?(\.\d+)?$`)

// Flags whose next argument is a value, not the script being run.
var flagsWithValue = map[string]bool{
	"-r": true, "--require": true, "--import": true, "--loader": true,
	"--experimental-loader": true, "-W": true, "-X": true, "--inspect-port": true,
	"-cp": true, "-classpath": true, "--env-file": true,
}

func describe(l *Listener) {
	l.Kind = classify(l.Exe)
	l.Name, l.Runtime = friendlyName(l)
	l.Project = projectDir(l)
}

func friendlyName(l *Listener) (name, runtime string) {
	base := filepath.Base(l.Exe)
	if l.Exe == "" {
		base = l.Comm
	}
	if len(l.Args) > 0 && !strings.Contains(l.Args[0], "/") && l.Args[0] != "" {
		// argv[0] is often nicer than the resolved path (e.g. "node" vs "node22").
		if _, ok := interpreters[strings.ToLower(l.Args[0])]; ok {
			base = l.Args[0]
		}
	}
	lower := strings.ToLower(base)
	rt, isInterp := interpreters[lower]
	if !isInterp && pythonVersioned.MatchString(lower) {
		rt, isInterp = "python", true
	}
	if !isInterp {
		if strings.HasSuffix(l.Exe, ".app/Contents/MacOS/"+base) || l.Kind != KindDev {
			return appName(l.Exe, base), ""
		}
		return base, ""
	}

	loader := ""
	args := l.Args
	if len(args) > 0 {
		args = args[1:]
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-m" && i+1 < len(args): // python -m http.server
			return args[i+1], rt
		case a == "-c" || a == "-e" || a == "--eval":
			return rt, ""
		case flagsWithValue[a] && i+1 < len(args):
			if pkg := nodeModulesPackage(args[i+1]); pkg != "" {
				loader = pkg
			}
			i++
		case strings.HasPrefix(a, "-"):
			if eq := strings.IndexByte(a, '='); eq > 0 && flagsWithValue[a[:eq]] {
				if pkg := nodeModulesPackage(a[eq+1:]); pkg != "" {
					loader = pkg
				}
			}
		default:
			if pkg := nodeModulesPackage(a); pkg != "" {
				return pkg, rt
			}
			if loader != "" {
				rt = loader
			}
			return filepath.Base(a), rt
		}
	}
	return rt, ""
}

// nodeModulesPackage extracts "vite" from ".../node_modules/.bin/vite" or
// ".../node_modules/vite/bin/vite.js", and "@scope/pkg" for scoped packages.
func nodeModulesPackage(p string) string {
	p = strings.TrimPrefix(p, "file://")
	i := strings.LastIndex(p, "node_modules/")
	if i < 0 {
		return ""
	}
	rest := p[i+len("node_modules/"):]
	parts := strings.Split(rest, "/")
	switch {
	case parts[0] == ".bin" && len(parts) > 1:
		return parts[1]
	case strings.HasPrefix(parts[0], "@") && len(parts) > 1:
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}

// appName turns ".../Visual Studio Code.app/Contents/Frameworks/Code Helper.app/..."
// into "Visual Studio Code" — the outermost bundle is what users recognise.
func appName(exe, fallback string) string {
	if i := strings.Index(exe, ".app/"); i >= 0 {
		return filepath.Base(exe[:i])
	}
	return fallback
}

// projectDir guesses which project a process belongs to: its working directory
// when meaningful, otherwise the directory holding the script it runs.
func projectDir(l *Listener) string {
	if l.Cwd != "" && l.Cwd != "/" {
		return l.Cwd
	}
	for _, a := range l.Args[min(1, len(l.Args)):] {
		a = strings.TrimPrefix(a, "file://")
		if !strings.HasPrefix(a, "/") {
			continue
		}
		if i := strings.Index(a, "/node_modules/"); i > 0 {
			return a[:i]
		}
	}
	return ""
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
