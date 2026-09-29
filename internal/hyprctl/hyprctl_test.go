package hyprctl

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSupportsLuaVersionGate(t *testing.T) {
	tests := []struct {
		version string
		tag     string
		want    bool
	}{
		{"0.54.3", "v0.54.3", false},
		{"0.55.0", "v0.55.0", true},
		{"0.56.2", "v0.56.2", true},
		{"", "v0.55.1", true},
		{"nightly", "", false},
		{"1.0.0", "v1.0.0", true},
	}
	for _, tc := range tests {
		v := VersionInfo{Version: tc.version, Tag: tc.tag}
		if got := v.SupportsLua(); got != tc.want {
			t.Errorf("%+v SupportsLua()=%v, want %v", v, got, tc.want)
		}
	}
}

func TestClientParsesCommands(t *testing.T) {
	bin := writeFakeHyprctl(t)
	c := &Client{Bin: bin, Timeout: time.Second}
	ctx := context.Background()

	v, err := c.Version(ctx)
	if err != nil || v.Version != "0.56.2" || !v.SupportsLua() {
		t.Fatalf("Version()=%+v, err=%v", v, err)
	}

	monitors, err := c.Monitors(ctx)
	if err != nil || len(monitors) != 1 || monitors[0].Name != "eDP-1" || monitors[0].Scale != 1 {
		t.Fatalf("Monitors()=%+v, err=%v", monitors, err)
	}

	devices, err := c.Devices(ctx)
	if err != nil || len(devices.Keyboards) != 1 || devices.Keyboards[0].Layout != "br,us" {
		t.Fatalf("Devices()=%+v, err=%v", devices, err)
	}

	status, err := c.Status(ctx)
	if err != nil || status.ConfigProvider != "lua" {
		t.Fatalf("Status()=%+v, err=%v", status, err)
	}

	option, err := c.GetOption(ctx, "decoration.blur.size")
	if err != nil || option.Kind != "int" || option.Value != float64(5) || !option.Set {
		t.Fatalf("GetOption()=%+v, err=%v", option, err)
	}
	options, err := c.GetOptions(ctx, []string{"general.border_size", "missing.option"})
	if err != nil || options["general.border_size"].Value != float64(2) || options["missing.option"].Kind != "" {
		t.Fatalf("GetOptions()=%+v err=%v", options, err)
	}

	errs, err := c.ConfigErrors(ctx)
	if err != nil || len(errs) != 1 || errs[0] != "invalid config" {
		t.Fatalf("ConfigErrors()=%v, err=%v", errs, err)
	}
	if err := c.Eval(ctx, "hl.config({})"); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if err := c.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
}

func TestGetOptionRejectsUnknownOption(t *testing.T) {
	bin := writeFakeHyprctl(t)
	c := &Client{Bin: bin}
	if _, err := c.GetOption(context.Background(), "missing.value"); err == nil {
		t.Fatal("expected unknown-option error")
	}
}

func writeFakeHyprctl(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hyprctl")
	script := `#!/bin/sh
case "$1" in
  version) printf '{"version":"0.56.2","tag":"v0.56.2","branch":"v0.56.2"}\n' ;;
  monitors) printf '[{"id":0,"name":"eDP-1","width":1920,"height":1200,"refreshRate":165,"x":0,"y":0,"scale":1,"transform":0,"availableModes":["1920x1200@165.00Hz"]}]\n' ;;
  devices) printf '{"keyboards":[{"name":"kbd","layout":"br,us","variant":",intl"}],"mice":[],"tablets":[],"touch":[],"switches":[]}\n' ;;
  status) printf '{"configProvider":"lua","backend":"drm"}\n' ;;
  getoption)
    if [ "$2" = "missing:value" ]; then printf 'no such option\n'; exit 0; fi
    printf '{"option":"decoration:blur:size","int":5,"set":true}\n' ;;
  --batch) printf '{"option":"general:border_size","int":2,"set":true}\n\nno such option\n' ;;
  configerrors) printf '["", "invalid config"]\n' ;;
  eval|reload) printf 'ok\n' ;;
  *) printf 'unexpected command: %s\n' "$1" >&2; exit 2 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
