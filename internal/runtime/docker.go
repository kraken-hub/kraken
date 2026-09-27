package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/docker/go-units"
	"github.com/moby/term"

	"github.com/kraken-hub/kraken/pkg/docker"
)

// DockerRuntime implements Runtime using the Docker SDK.
// It works transparently with Podman because Podman exposes a
// Docker-compatible REST API; only the socket path differs.
type DockerRuntime struct {
	cli *client.Client
}

// newDockerRuntime creates a DockerRuntime. socketPath may be "" to use the
// SDK default (reads DOCKER_HOST or falls back to /var/run/docker.sock).
func newDockerRuntime(socketPath string) (*DockerRuntime, error) {
	apiVersion := "1.41"
	if v, ok := os.LookupEnv("KRAKEN_DOCKER_API_VERSION"); ok {
		apiVersion = v
	}
	var opts []client.Opt
	if apiVersion == "" {
		opts = append(opts, client.WithAPIVersionNegotiation())
	} else {
		opts = append(opts, client.WithVersion(apiVersion))
	}
	if socketPath != "" {
		opts = append(opts, client.WithHost("unix://"+socketPath))
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &DockerRuntime{cli: cli}, nil
}

// ---- Container lifecycle ----

func (r *DockerRuntime) ContainerCreate(ctx context.Context, cfg *ContainerOptions) (string, error) {
	// Env
	var envSlice []string
	for k, v := range cfg.Env {
		envSlice = append(envSlice, k+"="+v)
	}

	// Ports
	portSet := nat.PortSet{}
	portBindings := nat.PortMap{}
	for hostSpec, containerSpec := range cfg.Ports {
		cp := nat.Port(containerSpec)
		if !strings.Contains(string(cp), "/") {
			cp = nat.Port(containerSpec + "/tcp")
		}
		portSet[cp] = struct{}{}
		// hostSpec may be "port" or "ip:port"
		hostIP := "0.0.0.0"
		hostPort := hostSpec
		if idx := strings.LastIndex(hostSpec, ":"); idx >= 0 {
			hostIP = hostSpec[:idx]
			hostPort = hostSpec[idx+1:]
		}
		portBindings[cp] = []nat.PortBinding{{HostIP: hostIP, HostPort: hostPort}}
	}

	// Mounts
	var mounts []mount.Mount
	for _, vm := range cfg.Volumes {
		if vm.Source == "" {
			// Anonymous volume: Docker creates a named volume automatically
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeVolume,
				Target:   vm.Target,
				ReadOnly: vm.ReadOnly,
			})
		} else if isNamedVolume(vm.Source) {
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeVolume,
				Source:   vm.Source,
				Target:   vm.Target,
				ReadOnly: vm.ReadOnly,
			})
		} else {
			src := vm.Source
			if !filepath.IsAbs(src) {
				if abs, err := filepath.Abs(src); err == nil {
					src = abs
				}
			}
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   src,
				Target:   vm.Target,
				ReadOnly: vm.ReadOnly,
			})
		}
	}

	// Restart policy
	restartPolicy := container.RestartPolicy{}
	switch cfg.Restart {
	case "always":
		restartPolicy.Name = "always"
	case "unless-stopped":
		restartPolicy.Name = "unless-stopped"
	case "on-failure":
		restartPolicy.Name = "on-failure"
		restartPolicy.MaximumRetryCount = cfg.RestartMaxRetry
	}

	// Merge system and user labels
	mergedLabels := make(map[string]string, len(cfg.Labels)+len(cfg.UserLabels))
	for k, v := range cfg.Labels {
		mergedLabels[k] = v
	}
	for k, v := range cfg.UserLabels {
		mergedLabels[k] = v
	}

	// Logging
	logConfig := container.LogConfig{}
	if cfg.Logging.Driver != "" {
		logConfig.Type = cfg.Logging.Driver
		logConfig.Config = cfg.Logging.Options
	}

	cCfg := &container.Config{
		Image:        cfg.Image,
		Env:          envSlice,
		ExposedPorts: portSet,
		Labels:       mergedLabels,
		Hostname:     cfg.Hostname,
		WorkingDir:   cfg.WorkDir,
		Tty:          cfg.Tty,
		OpenStdin:    cfg.OpenStdin,
		StdinOnce:    false,
	}
	if len(cfg.Entrypoint) > 0 {
		cCfg.Entrypoint = cfg.Entrypoint
	}
	if len(cfg.Cmd) > 0 {
		cCfg.Cmd = cfg.Cmd
	}

	// pidsLimit := int64(-1) // override systemd DefaultTasksMax
	hCfg := &container.HostConfig{
		PortBindings:  portBindings,
		Mounts:        mounts,
		RestartPolicy: restartPolicy,
		NetworkMode:   container.NetworkMode(cfg.NetworkMode),
		ExtraHosts:    cfg.ExtraHosts,
		DNS:           cfg.DNS,
		DNSSearch:     cfg.DNSSearch,
		DNSOptions:    cfg.DNSOptions,
		VolumesFrom:   cfg.VolumesFrom,
		LogConfig:     logConfig,
		AutoRemove:    cfg.AutoRemove,
		// Resources: container.Resources{
		// 	PidsLimit: &pidsLimit,
		// 	Ulimits:   []*container.Ulimit{{Name: "nproc", Soft: -1, Hard: -1}},
		// },
	}

	netCfg := &network.NetworkingConfig{}

	resp, err := r.cli.ContainerCreate(ctx, cCfg, hCfg, netCfg, nil, cfg.Name)
	if err != nil {
		return "", fmt.Errorf("create container %q: %w", cfg.Name, err)
	}
	return resp.ID, nil
}

func (r *DockerRuntime) ContainerStart(ctx context.Context, id string) error {
	return r.cli.ContainerStart(ctx, id, container.StartOptions{})
}

func (r *DockerRuntime) ContainerStop(ctx context.Context, id string, timeout *time.Duration) error {
	opts := container.StopOptions{}
	if timeout != nil {
		secs := int(timeout.Seconds())
		opts.Timeout = &secs
	}
	return r.cli.ContainerStop(ctx, id, opts)
}

func (r *DockerRuntime) ContainerRemove(ctx context.Context, id string, force bool, removeVolumes bool) error {
	return r.cli.ContainerRemove(ctx, id, container.RemoveOptions{
		Force:         force,
		RemoveVolumes: removeVolumes,
	})
}

func (r *DockerRuntime) ContainerRestart(ctx context.Context, id string, timeout *time.Duration) error {
	opts := container.StopOptions{}
	if timeout != nil {
		secs := int(timeout.Seconds())
		opts.Timeout = &secs
	}
	return r.cli.ContainerRestart(ctx, id, opts)
}

func (r *DockerRuntime) ContainerList(ctx context.Context, all bool, labels map[string]string) ([]ContainerInfo, error) {
	f := filters.NewArgs()
	for k, v := range labels {
		f.Add("label", k+"="+v)
	}
	containers, err := r.cli.ContainerList(ctx, container.ListOptions{
		All:     all,
		Filters: f,
	})
	if err != nil {
		return nil, err
	}

	result := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		var ports []PortBinding
		for _, p := range c.Ports {
			ports = append(ports, PortBinding{
				HostIP:        p.IP,
				HostPort:      fmt.Sprintf("%d", p.PublicPort),
				ContainerPort: fmt.Sprintf("%d", p.PrivatePort),
				Protocol:      p.Type,
			})
		}
		result = append(result, ContainerInfo{
			ID:      shortID(c.ID),
			Name:    name,
			Image:   c.Image,
			Status:  c.Status,
			State:   c.State,
			Labels:  c.Labels,
			Created: c.Created,
			Ports:   ports,
		})
	}
	return result, nil
}

func (r *DockerRuntime) ContainerInspect(ctx context.Context, id string) (*ContainerInfo, error) {
	c, err := r.cli.ContainerInspect(ctx, id)
	if err != nil {
		return nil, err
	}
	state := "stopped"
	if c.State != nil && c.State.Running {
		state = "running"
	}
	status := ""
	if c.State != nil {
		status = c.State.Status
	}
	labels := map[string]string{}
	tty := false
	if c.Config != nil {
		labels = c.Config.Labels
		tty = c.Config.Tty
	}
	ip := ""
	if c.NetworkSettings != nil {
		if c.NetworkSettings.IPAddress != "" {
			ip = c.NetworkSettings.IPAddress
		} else {
			for _, endpoint := range c.NetworkSettings.Networks {
				if endpoint.IPAddress != "" {
					ip = endpoint.IPAddress
					break
				}
			}
		}
	}

	return &ContainerInfo{
		ID:     shortID(c.ID),
		Name:   strings.TrimPrefix(c.Name, "/"),
		Image:  c.Image,
		Status: status,
		State:  state,
		IP:     ip,
		Labels: labels,
		Tty:    tty,
	}, nil
}

func (r *DockerRuntime) ContainerExists(ctx context.Context, nameOrID string) (bool, error) {
	_, err := r.cli.ContainerInspect(ctx, nameOrID)
	if err != nil {
		if client.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *DockerRuntime) ContainerGetRunConfig(ctx context.Context, nameOrID string) (*ContainerRunConfig, error) {
	c, err := r.cli.ContainerInspect(ctx, nameOrID)
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	var volumes []VolumeMount
	for _, m := range c.Mounts {
		switch m.Type {
		case "bind":
			volumes = append(volumes, VolumeMount{Source: m.Source, Target: m.Destination, ReadOnly: !m.RW})
		case "volume":
			if m.Name != "" {
				volumes = append(volumes, VolumeMount{Source: m.Name, Target: m.Destination, ReadOnly: !m.RW})
			}
		}
	}

	cfg := &ContainerRunConfig{}
	if c.Config != nil {
		cfg.Image = c.Config.Image
		cfg.Env = c.Config.Env
		cfg.Entrypoint = c.Config.Entrypoint
		cfg.WorkDir = c.Config.WorkingDir
	}
	if c.HostConfig != nil {
		cfg.VolumesFrom = c.HostConfig.VolumesFrom
		cfg.ExtraHosts = c.HostConfig.ExtraHosts
	}
	cfg.Volumes = volumes
	return cfg, nil
}

func (r *DockerRuntime) ContainerNetworkIP(ctx context.Context, nameOrID, networkName string) (string, error) {
	c, err := r.cli.ContainerInspect(ctx, nameOrID)
	if err != nil {
		return "", err
	}
	if c.NetworkSettings != nil {
		if ep, ok := c.NetworkSettings.Networks[networkName]; ok && ep.IPAddress != "" {
			return ep.IPAddress, nil
		}
		// Fall back to any network that has an IP address
		for _, ep := range c.NetworkSettings.Networks {
			if ep.IPAddress != "" {
				return ep.IPAddress, nil
			}
		}
	}
	return "", nil
}

func (r *DockerRuntime) ContainerLogs(ctx context.Context, id string, opts LogsOptions) (io.ReadCloser, error) {
	return r.cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: opts.Stdout,
		ShowStderr: opts.Stderr,
		Follow:     opts.Follow,
		Since:      opts.Since,
		Tail:       opts.Tail,
		Timestamps: opts.Timestamps,
	})
}

func (r *DockerRuntime) ContainerExec(ctx context.Context, id string, cfg ExecOptions, in io.ReadCloser, out, errW io.Writer) (int, error) {
	execCfg := container.ExecOptions{
		Cmd:          cfg.Cmd,
		Env:          cfg.Env,
		Tty:          cfg.Tty,
		AttachStdin:  cfg.AttachStdin,
		AttachStdout: true,
		AttachStderr: true,
		Detach:       cfg.Detach,
		WorkingDir:   cfg.WorkDir,
	}

	exec, err := r.cli.ContainerExecCreate(ctx, id, execCfg)
	if err != nil {
		return 0, fmt.Errorf("exec create: %w", err)
	}

	if cfg.Detach {
		return 0, r.cli.ContainerExecStart(ctx, exec.ID, container.ExecStartOptions{Detach: true})
	}

	resp, err := r.cli.ContainerExecAttach(ctx, exec.ID, container.ExecStartOptions{Tty: cfg.Tty})
	if err != nil {
		return 0, fmt.Errorf("exec attach: %w", err)
	}
	defer resp.Close()

	if cfg.Tty && in != nil {
		if fd, isTerm := term.GetFdInfo(in); isTerm {
			if prev, e := term.SetRawTerminal(fd); e == nil {
				defer term.RestoreTerminal(fd, prev)
			}
		}
	}

	copyDone := make(chan error, 1)
	go func() {
		if cfg.Tty {
			_, copyErr := io.Copy(out, resp.Reader)
			copyDone <- copyErr
		} else {
			_, copyErr := stdcopy.StdCopy(out, errW, resp.Reader)
			copyDone <- copyErr
		}
	}()

	if cfg.AttachStdin && in != nil {
		go func() { _, _ = io.Copy(resp.Conn, in) }()
	}

	if err := <-copyDone; err != nil {
		return 0, err
	}

	inspect, err := r.cli.ContainerExecInspect(ctx, exec.ID)
	if err != nil {
		return 0, fmt.Errorf("exec inspect: %w", err)
	}
	return inspect.ExitCode, nil
}

func (r *DockerRuntime) ContainerWait(ctx context.Context, id string) (<-chan int64, <-chan error) {
	statusCh, errCh := r.cli.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	exitCh := make(chan int64, 1)
	outErr := make(chan error, 1)
	go func() {
		select {
		case s := <-statusCh:
			exitCh <- s.StatusCode
		case e := <-errCh:
			outErr <- e
		}
	}()
	return exitCh, outErr
}

func (r *DockerRuntime) ContainerRun(ctx context.Context, id string, tty bool, in io.ReadCloser, out, errW io.Writer) (int, error) {
	resp, err := r.cli.ContainerAttach(ctx, id, container.AttachOptions{
		Stream: true,
		Stdin:  in != nil,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return -1, fmt.Errorf("attach: %w", err)
	}
	defer resp.Close()

	// Put terminal in raw mode so Ctrl+D closes the session and Ctrl+C is
	// forwarded to the container instead of killing the kraken process.
	if tty && in != nil {
		if fd, isTerm := term.GetFdInfo(in); isTerm {
			if prev, e := term.SetRawTerminal(fd); e == nil {
				defer term.RestoreTerminal(fd, prev)
			}
		}
	}

	// Wire output before starting so no data is lost.
	copyDone := make(chan error, 1)
	go func() {
		if tty {
			_, e := io.Copy(out, resp.Reader)
			copyDone <- e
		} else {
			_, e := stdcopy.StdCopy(out, errW, resp.Reader)
			copyDone <- e
		}
	}()

	if in != nil {
		go func() { _, _ = io.Copy(resp.Conn, in) }()
	}

	// Start wait before start to avoid a race on quick-exiting containers.
	exitCh, waitErrCh := r.ContainerWait(ctx, id)

	if err := r.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return -1, fmt.Errorf("start: %w", err)
	}

	if tty && out != nil {
		docker.MonitorTtySize(ctx, r.cli, out, id, false) //nolint:errcheck
	}

	select {
	case code := <-exitCh:
		<-copyDone
		return int(code), nil
	case err := <-waitErrCh:
		return -1, err
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

func (r *DockerRuntime) ContainerResize(ctx context.Context, id string, isExec bool, height, width uint) error {
	opts := container.ResizeOptions{Height: height, Width: width}
	if isExec {
		return r.cli.ContainerExecResize(ctx, id, opts)
	}
	return r.cli.ContainerResize(ctx, id, opts)
}

// ---- Images ----

func (r *DockerRuntime) ImagePull(ctx context.Context, name, platform string, out io.Writer) error {
	opts := image.PullOptions{Platform: platform}
	if _, auth, found, err := docker.ResolveRegistryAuth(name); err != nil {
		return fmt.Errorf("pull %q: %w", name, err)
	} else if found {
		encoded, err := docker.EncodeAuth(auth)
		if err != nil {
			return fmt.Errorf("pull %q: %w", name, err)
		}
		opts.RegistryAuth = encoded
	}

	rc, err := r.cli.ImagePull(ctx, name, opts)
	if err != nil {
		return fmt.Errorf("pull %q: %w", name, err)
	}
	defer rc.Close()
	if out == nil {
		out = io.Discard
	}
	fd, isTerm := term.GetFdInfo(out)
	return jsonmessage.DisplayJSONMessagesStream(rc, out, fd, isTerm, nil)
}

func (r *DockerRuntime) ImageBuild(ctx context.Context, cfg *BuildOptions, out io.Writer) error {
	var err error

	dockerfile := cfg.File
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}

	var buildCtx io.ReadCloser
	if cfg.InlineFile != "" || len(cfg.InlineFiles) > 0 {
		buildCtx = docker.CreateBuildContext()
		var files []docker.BuildContextFile
		for name, bf := range cfg.InlineFiles {
			mode := bf.Mode
			if mode == 0 {
				mode = 0o644
			}
			files = append(files, docker.BuildContextFile{Name: name, Body: bf.Content, Mode: mode})
		}
		if cfg.InlineFile != "" {
			files = append(files, docker.BuildContextFile{Name: dockerfile, Body: cfg.InlineFile, Mode: 0644})
		}
		buildCtx = docker.AppendFilesToBuildContext(files, buildCtx)
		if cfg.Context != "" {
			dirCtx, dirErr := docker.CreateBuildContextFromPath(cfg.Context, dockerfile)
			if dirErr != nil {
				return fmt.Errorf("build context: %w", dirErr)
			}
			buildCtx, err = docker.MergeBuildContexts(buildCtx, dirCtx)
			if err != nil {
				return fmt.Errorf("build context merge: %w", err)
			}
		}
	} else {
		buildCtx, err = docker.CreateBuildContextFromPath(cfg.Context, dockerfile)
		if err != nil {
			return fmt.Errorf("build context: %w", err)
		}
	}

	buildArgs := make(map[string]*string, len(cfg.Args))
	for k, v := range cfg.Args {
		val := v
		buildArgs[k] = &val
	}

	var shmSize int64
	if cfg.ShmSize != "" {
		shmSize, _ = units.RAMInBytes(cfg.ShmSize)
	}

	resp, err := r.cli.ImageBuild(ctx, buildCtx, types.ImageBuildOptions{
		Tags:        cfg.Tags,
		Dockerfile:  dockerfile,
		NoCache:     cfg.NoCache,
		PullParent:  cfg.PullParent,
		BuildArgs:   buildArgs,
		Target:      cfg.Target,
		Labels:      cfg.Labels,
		CacheFrom:   cfg.CacheFrom,
		NetworkMode: cfg.Network,
		ShmSize:     shmSize,
		Platform:    cfg.Platform,
		AuthConfigs: docker.ResolveAllRegistryAuth(),
		Remove:      true,
	})
	if err != nil {
		return fmt.Errorf("image build: %w", err)
	}
	defer resp.Body.Close()

	if out == nil {
		out = io.Discard
	}
	fd, isTerm := term.GetFdInfo(out)
	return jsonmessage.DisplayJSONMessagesStream(resp.Body, out, fd, isTerm, nil)
}

func (r *DockerRuntime) ImageExists(ctx context.Context, name string) (bool, error) {
	_, _, err := r.cli.ImageInspectWithRaw(ctx, name)
	if err != nil {
		if client.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *DockerRuntime) ImageVolumes(ctx context.Context, name string) ([]string, error) {
	info, _, err := r.cli.ImageInspectWithRaw(ctx, name)
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if info.Config == nil {
		return nil, nil
	}
	paths := make([]string, 0, len(info.Config.Volumes))
	for p := range info.Config.Volumes {
		paths = append(paths, p)
	}
	return paths, nil
}

func (r *DockerRuntime) ImageRemove(ctx context.Context, name string, force bool) error {
	_, err := r.cli.ImageRemove(ctx, name, image.RemoveOptions{Force: force, PruneChildren: false})
	return err
}

// ---- Networks ----

func (r *DockerRuntime) NetworkCreate(ctx context.Context, name string, labels map[string]string) (string, error) {
	resp, err := r.cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
		Labels: labels,
	})
	if err != nil {
		return "", fmt.Errorf("create network %q: %w", name, err)
	}
	return resp.ID, nil
}

func (r *DockerRuntime) NetworkRemove(ctx context.Context, id string) error {
	return r.cli.NetworkRemove(ctx, id)
}

func (r *DockerRuntime) NetworkExists(ctx context.Context, name string) (bool, string, error) {
	f := filters.NewArgs()
	f.Add("name", name)
	nets, err := r.cli.NetworkList(ctx, network.ListOptions{Filters: f})
	if err != nil {
		return false, "", err
	}
	for _, n := range nets {
		if n.Name == name {
			return true, n.ID, nil
		}
	}
	return false, "", nil
}

func (r *DockerRuntime) NetworkConnect(ctx context.Context, networkID, containerID string, aliases ...string) error {
	return r.cli.NetworkConnect(ctx, networkID, containerID, &network.EndpointSettings{
		Aliases: aliases,
	})
}

// ---- Volumes ----

func (r *DockerRuntime) VolumeCreate(ctx context.Context, opts VolumeCreateOptions) error {
	driver := opts.Driver
	if driver == "" {
		driver = "local"
	}
	_, err := r.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name:       opts.Name,
		Driver:     driver,
		DriverOpts: opts.DriverOpts,
		Labels:     opts.Labels,
	})
	return err
}

func (r *DockerRuntime) VolumeRemove(ctx context.Context, name string, force bool) error {
	return r.cli.VolumeRemove(ctx, name, force)
}

func (r *DockerRuntime) VolumeExists(ctx context.Context, name string) (bool, error) {
	f := filters.NewArgs()
	f.Add("name", name)
	list, err := r.cli.VolumeList(ctx, volume.ListOptions{Filters: f})
	if err != nil {
		return false, err
	}
	for _, v := range list.Volumes {
		if v.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (r *DockerRuntime) VolumeList(ctx context.Context, namePrefix string) ([]string, error) {
	f := filters.NewArgs()
	if namePrefix != "" {
		f.Add("name", namePrefix)
	}
	list, err := r.cli.VolumeList(ctx, volume.ListOptions{Filters: f})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, v := range list.Volumes {
		if strings.HasPrefix(v.Name, namePrefix) {
			names = append(names, v.Name)
		}
	}
	return names, nil
}

// ---- Metadata ----

func (r *DockerRuntime) ServerVersion(ctx context.Context) (string, error) {
	sv, err := r.cli.ServerVersion(ctx)
	if err != nil {
		return "", err
	}
	return sv.Version, nil
}

// ---- helpers ----

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// isNamedVolume returns true for Docker named volumes (no path prefix).
func isNamedVolume(src string) bool {
	return src != "" &&
		!strings.HasPrefix(src, "/") &&
		!strings.HasPrefix(src, ".") &&
		!strings.HasPrefix(src, "~")
}
