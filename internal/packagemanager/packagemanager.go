package packagemanager

// PackageManager defines the interface for package managers
type PackageManager interface {
	GetLatestVersion(packageName string) (string, error)
	SetCustomIndexURL() error
}
