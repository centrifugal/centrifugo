// +build windows

package node

// not implemented for Windows.
func cpuUsage() (int64, error) {
	return 0, nil
}

// not implemented for Windows.
func cpuUsageSeparated() (int64, int64, error) {
	return 0, 0, nil
}
