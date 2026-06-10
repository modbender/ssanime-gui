package download

import "golang.org/x/sys/windows"

// freeDiskBytes returns the bytes available to the caller on the volume holding
// dir. GetDiskFreeSpaceEx reports the caller-available figure, which already
// accounts for per-user quotas.
func freeDiskBytes(dir string) (int64, error) {
	pathp, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var freeToCaller, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(pathp, &freeToCaller, &total, &totalFree); err != nil {
		return 0, err
	}
	return int64(freeToCaller), nil
}
