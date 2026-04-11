//go:build !windows

package platform

func SupportsAutostart() bool {
	return false
}

func IsAutostartEnabled() bool {
	return false
}

func SetAutostart(bool) error {
	return nil
}
