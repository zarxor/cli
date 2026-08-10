//go:build windows

package host

import "golang.org/x/sys/windows/registry"

func windowsProductName() string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, err := key.GetStringValue("ProductName")
	if err != nil {
		return ""
	}
	return value
}

func desktopEnvironmentIndicators() []string { return nil }
