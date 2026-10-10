package ports

import "testing"

func TestClassify(t *testing.T) {
	cases := map[string]Kind{
		"/usr/libexec/rapportd": KindSystem,
		"/System/Library/CoreServices/ControlCenter.app/Contents/MacOS/ControlCenter":                   KindSystem,
		"/Applications/Spotify.app/Contents/MacOS/Spotify":                                              KindApp,
		"/Library/Frameworks/Python.framework/Versions/3.12/Resources/Python.app/Contents/MacOS/Python": KindDev,
		"/opt/homebrew/bin/postgres":                                                                    KindDev,
		"/private/var/folders/xy/T/go-build123/b001/exe/main":                                           KindDev,
	}
	for exe, want := range cases {
		if got := classify(exe); got != want {
			t.Errorf("classify(%q) = %v, want %v", exe, got, want)
		}
	}
}
