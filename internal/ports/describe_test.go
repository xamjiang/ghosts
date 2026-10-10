package ports

import "testing"

func TestFriendlyName(t *testing.T) {
	cases := []struct {
		exe      string
		args     []string
		name, rt string
	}{
		{"/Users/x/.nvm/versions/node/v24/bin/node", []string{"node", "/p/app/node_modules/.bin/vite"}, "vite", "node"},
		{"/usr/local/bin/node", []string{"node", "/p/node_modules/astro/bin/astro.mjs", "dev"}, "astro", "node"},
		{"/usr/local/bin/node", []string{"node", "/p/node_modules/@angular/cli/bin/ng", "serve"}, "@angular/cli", "node"},
		{"/usr/local/bin/node", []string{"node", "--require", "/p/node_modules/tsx/dist/preflight.cjs", "--import", "file:///p/node_modules/tsx/dist/loader.mjs", "server/index.ts"}, "index.ts", "tsx"},
		{"/Library/Frameworks/Python.framework/Versions/3.12/Resources/Python.app/Contents/MacOS/Python", []string{"python3", "-m", "http.server", "4400"}, "http.server", "python"},
		{"/opt/homebrew/bin/python3.12", []string{"python3.12", "-c", "print(1)"}, "python", ""},
		{"/opt/homebrew/bin/redis-server", []string{"redis-server", "*:6379"}, "redis-server", ""},
		{"/Applications/Visual Studio Code.app/Contents/Frameworks/Code Helper.app/Contents/MacOS/Code Helper", nil, "Visual Studio Code", ""},
	}
	for _, c := range cases {
		l := &Listener{Exe: c.exe, Args: c.args}
		l.Kind = classify(l.Exe)
		name, rt := friendlyName(l)
		if name != c.name || rt != c.rt {
			t.Errorf("%v: got (%q, %q), want (%q, %q)", c.args, name, rt, c.name, c.rt)
		}
	}
}

func TestNameWorkers(t *testing.T) {
	ls := []Listener{
		{PID: 10, Port: 8000, Name: "uvicorn", Runtime: "python"},
		{PID: 11, PPID: 10, Port: 8000, Name: "python"},
	}
	nameWorkers(ls)
	if ls[1].Name != "uvicorn worker" {
		t.Errorf("got %q", ls[1].Name)
	}
}
