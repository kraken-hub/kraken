package runtime

import (
	"context"
	"io"
	"time"
)

// VolumeMount represents a single volume binding for a container.
type VolumeMount struct {
	Source   string // host path, named volume name, or "" for anonymous
	Target   string // container path
	ReadOnly bool
}

// BuildFile holds the content and optional file mode for an inline build file.
type BuildFile struct {
	Content string
	Mode    int64 // unix permission bits, e.g. 0o775; 0 means use default (0644)
}

// BuildOptions holds all build: section configuration parsed from kraken.yaml.
type BuildOptions struct {
	Context     string               // build context path (default: app dir)
	File        string               // Dockerfile path within context (default: "Dockerfile")
	InlineFile  string               // inline Dockerfile content (written to File path in context)
	InlineFiles map[string]BuildFile // additional inline files: filename -> content+mode
	Args        map[string]string    // --build-arg values
	Target      string               // multi-stage build target
	Policy      string               // always | missing (default: missing)
	PullParent  bool                 // pull base image before build (set by EnsureImage from PullPolicy)
	Tags        []string             // full image tag list (primary tag prepended by EnsureImage)
	Labels      map[string]string    // image labels
	Platform    string               // target platform, e.g. "linux/amd64" (set from service-level platform)
	CacheFrom   []string             // cache source images
	ShmSize     string               // /dev/shm size, e.g. "128mb"
	Network     string               // network mode during build, e.g. "host"
	NoCache     bool                 // disable layer cache
}

// LoggingOptions holds logging driver configuration.
type LoggingOptions struct {
	Driver  string
	Options map[string]string
}

// VolumeCreateOptions holds all options for creating a named volume.
type VolumeCreateOptions struct {
	Name       string
	Driver     string
	DriverOpts map[string]string
	Labels     map[string]string
}

// ContainerOptions holds all configuration needed to create a container.
type ContainerOptions struct {
	Name            string
	Image           string
	Env             map[string]string
	Volumes         []VolumeMount
	Ports           map[string]string // "hostIP:hostPort" or "hostPort" -> "containerPort/proto"
	Labels          map[string]string // system labels (kraken.app, kraken.service)
	UserLabels      map[string]string // user-defined labels from yaml
	ExtraHosts      []string          // ["hostname:ip", ...]
	DNS             []string          // custom DNS servers
	DNSSearch       []string          // DNS search domains
	DNSOptions      []string          // DNS resolver options, e.g. ["ndots:5"]
	VolumesFrom     []string          // ["containerName", "containerName:ro", ...]
	Logging         LoggingOptions
	Entrypoint      []string
	Cmd             []string
	WorkDir         string
	Restart         string
	RestartMaxRetry int
	NetworkMode     string
	Hostname        string
	AutoRemove      bool
	Tty             bool
	OpenStdin       bool
	Platform        string        // container platform, e.g. "linux/amd64"
	PullPolicy      string        // always | missing | never (default: missing)
	Build           *BuildOptions // nil when no build: section
}

// ContainerRunConfig holds the config of an existing container that can be used
// as a template for a one-off run container.
type ContainerRunConfig struct {
	Image       string
	Env         []string // "KEY=VALUE" format
	Entrypoint  []string
	WorkDir     string
	Volumes     []VolumeMount
	VolumesFrom []string
	ExtraHosts  []string
}

// ContainerInfo holds information about a container returned by the runtime.
type ContainerInfo struct {
	ID      string
	Name    string
	Image   string
	Status  string // human-readable status, e.g. "Up 2 hours"
	State   string // "running", "exited", "created", etc.
	IP      string // primary IP address on the container's network, empty if not running
	Labels  map[string]string
	Created int64 // Unix timestamp
	Ports   []PortBinding
	Tty     bool // true when the container was created with a TTY
}

// PortBinding represents a single published port.
type PortBinding struct {
	HostIP        string
	HostPort      string
	ContainerPort string
	Protocol      string
}

// ExecOptions holds configuration for running a command inside an existing container.
type ExecOptions struct {
	Cmd         []string
	Env         []string
	Tty         bool
	AttachStdin bool
	Detach      bool
	WorkDir     string
}

// LogsOptions controls log retrieval behaviour.
type LogsOptions struct {
	Follow     bool
	Since      string // timestamp or relative duration, e.g. "42m"
	Tail       string // "all" or number as string
	Timestamps bool
	Stdout     bool
	Stderr     bool
}

// Runtime abstracts all container operations so that Docker and Podman
// (or any other OCI-compatible runtime) can be substituted without changing
// command code.  All container commands must use only this interface;
// they must never import the Docker SDK directly.
type Runtime interface {
	// --- Container lifecycle ---

	// ContainerCreate creates (but does not start) a container.
	ContainerCreate(ctx context.Context, cfg *ContainerOptions) (id string, err error)

	// ContainerStart starts a created or stopped container.
	ContainerStart(ctx context.Context, id string) error

	// ContainerStop sends a stop signal to a running container.
	// timeout=nil uses the daemon default.
	ContainerStop(ctx context.Context, id string, timeout *time.Duration) error

	// ContainerRemove removes a container.
	ContainerRemove(ctx context.Context, id string, force bool, removeVolumes bool) error

	// ContainerRestart stops and then starts a container.
	ContainerRestart(ctx context.Context, id string, timeout *time.Duration) error

	// --- Inspection ---

	// ContainerList lists containers.  labels is an optional label filter (AND).
	ContainerList(ctx context.Context, all bool, labels map[string]string) ([]ContainerInfo, error)

	// ContainerInspect returns detailed information about one container.
	ContainerInspect(ctx context.Context, id string) (*ContainerInfo, error)

	// ContainerExists returns true when a container with the given name or id exists.
	ContainerExists(ctx context.Context, nameOrID string) (bool, error)

	// ContainerGetRunConfig returns the config of an existing container suitable
	// for cloning as a one-off run container. Returns nil (no error) when the
	// container does not exist.
	ContainerGetRunConfig(ctx context.Context, nameOrID string) (*ContainerRunConfig, error)

	// ContainerNetworkIP returns the IP address of a container on a specific network.
	// Returns empty string if the container is not connected to that network.
	ContainerNetworkIP(ctx context.Context, nameOrID, networkName string) (string, error)

	// --- I/O ---

	// ContainerLogs returns a stream of log output.  The caller must close the reader.
	ContainerLogs(ctx context.Context, id string, opts LogsOptions) (io.ReadCloser, error)

	// ContainerExec runs a command inside an existing container and connects the
	// provided streams.  It blocks until the command exits unless cfg.Detach is true.
	// Returns the exit code of the command (0 for detached execs).
	ContainerExec(ctx context.Context, id string, cfg ExecOptions, in io.ReadCloser, out, errW io.Writer) (int, error)

	// ContainerWait waits for a container to reach a non-running state.
	// It returns channels for the exit code and any error.
	ContainerWait(ctx context.Context, id string) (<-chan int64, <-chan error)

	// ContainerRun attaches to a container's I/O streams, starts it, and blocks
	// until the container exits. Attach happens before start so no output is lost.
	// Returns the container exit code.
	ContainerRun(ctx context.Context, id string, tty bool, in io.ReadCloser, out, errW io.Writer) (int, error)

	// ContainerResize resizes the TTY of a running container or exec session.
	ContainerResize(ctx context.Context, id string, isExec bool, height, width uint) error

	// --- Images ---

	// ImagePull pulls an image from a registry, streaming progress to out.
	ImagePull(ctx context.Context, name, platform string, out io.Writer) error

	// ImageBuild builds an image from a BuildOptions and streams output to out.
	ImageBuild(ctx context.Context, cfg *BuildOptions, out io.Writer) error

	// ImageExists returns true when the named image is present locally.
	ImageExists(ctx context.Context, name string) (bool, error)

	// ImageVolumes returns the list of volume paths declared in the image's Dockerfile (VOLUME directives).
	ImageVolumes(ctx context.Context, name string) ([]string, error)

	// ImageRemove removes a local image.
	ImageRemove(ctx context.Context, name string, force bool) error

	// --- Networks ---

	// NetworkCreate creates a network and returns its ID.
	NetworkCreate(ctx context.Context, name string, labels map[string]string) (string, error)

	// NetworkRemove removes a network.
	NetworkRemove(ctx context.Context, id string) error

	// NetworkExists returns (true, networkID, nil) when the network exists.
	NetworkExists(ctx context.Context, name string) (bool, string, error)

	// NetworkConnect attaches a container to a network.
	// aliases registers additional DNS names for the container on that network.
	NetworkConnect(ctx context.Context, networkID, containerID string, aliases ...string) error

	// --- Volumes ---

	// VolumeCreate creates a named volume with the given options.
	VolumeCreate(ctx context.Context, opts VolumeCreateOptions) error

	// VolumeRemove removes a named volume.
	VolumeRemove(ctx context.Context, name string, force bool) error

	// VolumeExists returns true when the named volume exists.
	VolumeExists(ctx context.Context, name string) (bool, error)

	// VolumeList returns names of all volumes whose name starts with namePrefix.
	VolumeList(ctx context.Context, namePrefix string) ([]string, error)

	// --- Metadata ---

	// ServerVersion returns the runtime server version string.
	ServerVersion(ctx context.Context) (string, error)
}
