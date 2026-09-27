package util

const (
	LabelApp      = "kraken.app"
	LabelService  = "kraken.service"
	GlobalNetwork = "kraken_net"
)

// AppNetwork returns the per-app network name: kraken_net.{appName}
func AppNetwork(appName string) string {
	return "kraken_net." + appName
}

// ContainerName returns the Docker container name for an app service.
// Format: {serviceName}.{appName}
func ContainerName(appName, serviceName string) string {
	return serviceName + "." + appName
}

// AppLabels returns the label filter that identifies all containers of an app.
func AppLabels(appName string) map[string]string {
	return map[string]string{LabelApp: appName}
}

// BuiltImageName returns the name for an image built by kraken: kraken_{appName}_{serviceName}
func BuiltImageName(appName, serviceName string) string {
	return "kraken_" + appName + "_" + serviceName
}
