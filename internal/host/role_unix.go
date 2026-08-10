//go:build !windows

package host

import "os"

func windowsProductName() string { return "" }

func desktopEnvironmentIndicators() []string {
	candidates := []struct {
		name string
		path string
	}{
		{name: "X11 sessions", path: "/usr/share/xsessions"},
		{name: "Wayland sessions", path: "/usr/share/wayland-sessions"},
		{name: "GNOME Shell", path: "/usr/bin/gnome-shell"},
		{name: "KDE Plasma", path: "/usr/bin/plasmashell"},
		{name: "XFCE", path: "/usr/bin/xfce4-session"},
		{name: "Cinnamon", path: "/usr/bin/cinnamon-session"},
		{name: "MATE", path: "/usr/bin/mate-session"},
		{name: "LXDE", path: "/usr/bin/lxsession"},
		{name: "Sway", path: "/usr/bin/sway"},
	}
	indicators := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate.path); err == nil {
			indicators = append(indicators, candidate.name)
		}
	}
	return indicators
}
