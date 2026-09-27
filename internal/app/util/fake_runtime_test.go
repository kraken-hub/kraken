package util

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/runtime"
)

// fakeRuntime implements runtime.Runtime with overridable function fields.
// Any method invoked without its function field set panics loudly so
// unexpected calls fail the test instead of silently returning zero values.
type fakeRuntime struct {
	ContainerCreateFn       func(ctx context.Context, cfg *runtime.ContainerOptions) (string, error)
	ContainerStartFn        func(ctx context.Context, id string) error
	ContainerStopFn         func(ctx context.Context, id string, timeout *time.Duration) error
	ContainerRemoveFn       func(ctx context.Context, id string, force, removeVolumes bool) error
	ContainerRestartFn      func(ctx context.Context, id string, timeout *time.Duration) error
	ContainerListFn         func(ctx context.Context, all bool, labels map[string]string) ([]runtime.ContainerInfo, error)
	ContainerInspectFn      func(ctx context.Context, id string) (*runtime.ContainerInfo, error)
	ContainerExistsFn       func(ctx context.Context, nameOrID string) (bool, error)
	ContainerGetRunConfigFn func(ctx context.Context, nameOrID string) (*runtime.ContainerRunConfig, error)
	ContainerNetworkIPFn    func(ctx context.Context, nameOrID, networkName string) (string, error)
	ContainerLogsFn         func(ctx context.Context, id string, opts runtime.LogsOptions) (io.ReadCloser, error)
	ContainerExecFn         func(ctx context.Context, id string, cfg runtime.ExecOptions, in io.ReadCloser, out, errW io.Writer) (int, error)
	ContainerWaitFn         func(ctx context.Context, id string) (<-chan int64, <-chan error)
	ContainerRunFn          func(ctx context.Context, id string, tty bool, in io.ReadCloser, out, errW io.Writer) (int, error)
	ContainerResizeFn       func(ctx context.Context, id string, isExec bool, height, width uint) error
	ImagePullFn             func(ctx context.Context, image, platform string, out io.Writer) error
	ImageBuildFn            func(ctx context.Context, cfg *runtime.BuildOptions, out io.Writer) error
	ImageExistsFn           func(ctx context.Context, image string) (bool, error)
	ImageVolumesFn          func(ctx context.Context, image string) ([]string, error)
	ImageRemoveFn           func(ctx context.Context, image string, force bool) error
	NetworkCreateFn         func(ctx context.Context, name string, labels map[string]string) (string, error)
	NetworkRemoveFn         func(ctx context.Context, id string) error
	NetworkExistsFn         func(ctx context.Context, name string) (bool, string, error)
	NetworkConnectFn        func(ctx context.Context, networkID, containerID string, aliases ...string) error
	VolumeCreateFn          func(ctx context.Context, opts runtime.VolumeCreateOptions) error
	VolumeRemoveFn          func(ctx context.Context, name string, force bool) error
	VolumeExistsFn          func(ctx context.Context, name string) (bool, error)
	VolumeListFn            func(ctx context.Context, namePrefix string) ([]string, error)
	ServerVersionFn         func(ctx context.Context) (string, error)
}

func (f *fakeRuntime) ContainerCreate(ctx context.Context, cfg *runtime.ContainerOptions) (string, error) {
	if f.ContainerCreateFn == nil {
		panic("ContainerCreate not implemented")
	}
	return f.ContainerCreateFn(ctx, cfg)
}

func (f *fakeRuntime) ContainerStart(ctx context.Context, id string) error {
	if f.ContainerStartFn == nil {
		panic("ContainerStart not implemented")
	}
	return f.ContainerStartFn(ctx, id)
}

func (f *fakeRuntime) ContainerStop(ctx context.Context, id string, timeout *time.Duration) error {
	if f.ContainerStopFn == nil {
		panic("ContainerStop not implemented")
	}
	return f.ContainerStopFn(ctx, id, timeout)
}

func (f *fakeRuntime) ContainerRemove(ctx context.Context, id string, force bool, removeVolumes bool) error {
	if f.ContainerRemoveFn == nil {
		panic("ContainerRemove not implemented")
	}
	return f.ContainerRemoveFn(ctx, id, force, removeVolumes)
}

func (f *fakeRuntime) ContainerRestart(ctx context.Context, id string, timeout *time.Duration) error {
	if f.ContainerRestartFn == nil {
		panic("ContainerRestart not implemented")
	}
	return f.ContainerRestartFn(ctx, id, timeout)
}

func (f *fakeRuntime) ContainerList(ctx context.Context, all bool, labels map[string]string) ([]runtime.ContainerInfo, error) {
	if f.ContainerListFn == nil {
		panic("ContainerList not implemented")
	}
	return f.ContainerListFn(ctx, all, labels)
}

func (f *fakeRuntime) ContainerInspect(ctx context.Context, id string) (*runtime.ContainerInfo, error) {
	if f.ContainerInspectFn == nil {
		panic("ContainerInspect not implemented")
	}
	return f.ContainerInspectFn(ctx, id)
}

func (f *fakeRuntime) ContainerExists(ctx context.Context, nameOrID string) (bool, error) {
	if f.ContainerExistsFn == nil {
		panic("ContainerExists not implemented")
	}
	return f.ContainerExistsFn(ctx, nameOrID)
}

func (f *fakeRuntime) ContainerGetRunConfig(ctx context.Context, nameOrID string) (*runtime.ContainerRunConfig, error) {
	if f.ContainerGetRunConfigFn == nil {
		panic("ContainerGetRunConfig not implemented")
	}
	return f.ContainerGetRunConfigFn(ctx, nameOrID)
}

func (f *fakeRuntime) ContainerNetworkIP(ctx context.Context, nameOrID, networkName string) (string, error) {
	if f.ContainerNetworkIPFn == nil {
		panic("ContainerNetworkIP not implemented")
	}
	return f.ContainerNetworkIPFn(ctx, nameOrID, networkName)
}

func (f *fakeRuntime) ContainerLogs(ctx context.Context, id string, opts runtime.LogsOptions) (io.ReadCloser, error) {
	if f.ContainerLogsFn == nil {
		panic("ContainerLogs not implemented")
	}
	return f.ContainerLogsFn(ctx, id, opts)
}

func (f *fakeRuntime) ContainerExec(ctx context.Context, id string, cfg runtime.ExecOptions, in io.ReadCloser, out, errW io.Writer) (int, error) {
	if f.ContainerExecFn == nil {
		panic("ContainerExec not implemented")
	}
	return f.ContainerExecFn(ctx, id, cfg, in, out, errW)
}

func (f *fakeRuntime) ContainerWait(ctx context.Context, id string) (<-chan int64, <-chan error) {
	if f.ContainerWaitFn == nil {
		panic("ContainerWait not implemented")
	}
	return f.ContainerWaitFn(ctx, id)
}

func (f *fakeRuntime) ContainerRun(ctx context.Context, id string, tty bool, in io.ReadCloser, out, errW io.Writer) (int, error) {
	if f.ContainerRunFn == nil {
		panic("ContainerRun not implemented")
	}
	return f.ContainerRunFn(ctx, id, tty, in, out, errW)
}

func (f *fakeRuntime) ContainerResize(ctx context.Context, id string, isExec bool, height, width uint) error {
	if f.ContainerResizeFn == nil {
		panic("ContainerResize not implemented")
	}
	return f.ContainerResizeFn(ctx, id, isExec, height, width)
}

func (f *fakeRuntime) ImagePull(ctx context.Context, image, platform string, out io.Writer) error {
	if f.ImagePullFn == nil {
		panic("ImagePull not implemented")
	}
	return f.ImagePullFn(ctx, image, platform, out)
}

func (f *fakeRuntime) ImageBuild(ctx context.Context, cfg *runtime.BuildOptions, out io.Writer) error {
	if f.ImageBuildFn == nil {
		panic("ImageBuild not implemented")
	}
	return f.ImageBuildFn(ctx, cfg, out)
}

func (f *fakeRuntime) ImageExists(ctx context.Context, image string) (bool, error) {
	if f.ImageExistsFn == nil {
		panic("ImageExists not implemented")
	}
	return f.ImageExistsFn(ctx, image)
}

func (f *fakeRuntime) ImageVolumes(ctx context.Context, image string) ([]string, error) {
	if f.ImageVolumesFn == nil {
		panic("ImageVolumes not implemented")
	}
	return f.ImageVolumesFn(ctx, image)
}

func (f *fakeRuntime) ImageRemove(ctx context.Context, image string, force bool) error {
	if f.ImageRemoveFn == nil {
		panic("ImageRemove not implemented")
	}
	return f.ImageRemoveFn(ctx, image, force)
}

func (f *fakeRuntime) NetworkCreate(ctx context.Context, name string, labels map[string]string) (string, error) {
	if f.NetworkCreateFn == nil {
		panic("NetworkCreate not implemented")
	}
	return f.NetworkCreateFn(ctx, name, labels)
}

func (f *fakeRuntime) NetworkRemove(ctx context.Context, id string) error {
	if f.NetworkRemoveFn == nil {
		panic("NetworkRemove not implemented")
	}
	return f.NetworkRemoveFn(ctx, id)
}

func (f *fakeRuntime) NetworkExists(ctx context.Context, name string) (bool, string, error) {
	if f.NetworkExistsFn == nil {
		panic("NetworkExists not implemented")
	}
	return f.NetworkExistsFn(ctx, name)
}

func (f *fakeRuntime) NetworkConnect(ctx context.Context, networkID, containerID string, aliases ...string) error {
	if f.NetworkConnectFn == nil {
		panic("NetworkConnect not implemented")
	}
	return f.NetworkConnectFn(ctx, networkID, containerID, aliases...)
}

func (f *fakeRuntime) VolumeCreate(ctx context.Context, opts runtime.VolumeCreateOptions) error {
	if f.VolumeCreateFn == nil {
		panic("VolumeCreate not implemented")
	}
	return f.VolumeCreateFn(ctx, opts)
}

func (f *fakeRuntime) VolumeRemove(ctx context.Context, name string, force bool) error {
	if f.VolumeRemoveFn == nil {
		panic("VolumeRemove not implemented")
	}
	return f.VolumeRemoveFn(ctx, name, force)
}

func (f *fakeRuntime) VolumeExists(ctx context.Context, name string) (bool, error) {
	if f.VolumeExistsFn == nil {
		panic("VolumeExists not implemented")
	}
	return f.VolumeExistsFn(ctx, name)
}

func (f *fakeRuntime) VolumeList(ctx context.Context, namePrefix string) ([]string, error) {
	if f.VolumeListFn == nil {
		panic("VolumeList not implemented")
	}
	return f.VolumeListFn(ctx, namePrefix)
}

func (f *fakeRuntime) ServerVersion(ctx context.Context) (string, error) {
	if f.ServerVersionFn == nil {
		panic("ServerVersion not implemented")
	}
	return f.ServerVersionFn(ctx)
}

var _ runtime.Runtime = (*fakeRuntime)(nil)

// testKraken bundles a *app.Kraken wired to a fake runtime, with its stdout
// and stderr captured to temp files for assertions.
type testKraken struct {
	*app.Kraken
	outFile *os.File
	errFile *os.File
}

// stdout returns everything written to kraken.Out() so far.
func (tk *testKraken) stdout() string {
	tk.outFile.Sync()
	data, err := os.ReadFile(tk.outFile.Name())
	if err != nil {
		return ""
	}
	return string(data)
}

// stderr returns everything written to kraken.Err() so far.
func (tk *testKraken) stderr() string {
	tk.errFile.Sync()
	data, err := os.ReadFile(tk.errFile.Name())
	if err != nil {
		return ""
	}
	return string(data)
}

// newTestKraken builds a *app.Kraken with the real construction path
// (app.New) but with stdout/stderr redirected to temp files and the
// container runtime replaced by rt, so no real Docker daemon is required.
func newTestKraken(t *testing.T, rt runtime.Runtime) *testKraken {
	t.Helper()
	dir := t.TempDir()
	outFile, err := os.Create(dir + "/stdout")
	if err != nil {
		t.Fatal(err)
	}
	errFile, err := os.Create(dir + "/stderr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		outFile.Close()
		errFile.Close()
	})

	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outFile, errFile
	k := app.New("kraken-test", "0.0.0-test")
	os.Stdout, os.Stderr = oldOut, oldErr

	k.SetRuntime(rt)

	return &testKraken{Kraken: k, outFile: outFile, errFile: errFile}
}
