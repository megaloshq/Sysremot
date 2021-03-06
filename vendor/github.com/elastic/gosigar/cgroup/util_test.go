package cgroup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const dockerTestData = "testdata/docker.zip"

func TestMain(m *testing.M) {
	err := extractTestData(dockerTestData)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// extractTestData from zip file and write it in the same dir as the zip file.
func extractTestData(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()

	dest := filepath.Dir(path)

	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		path := filepath.Join(dest, f.Name)
		if found, err := exists(path); err != nil || found {
			return err
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
		} else {
			destFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(0700))
			if err != nil {
				return err
			}
			defer destFile.Close()

			_, err = io.Copy(destFile, rc)
			if err != nil {
				return err
			}

			os.Chmod(path, f.Mode())
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}

// exists returns whether the given file or directory exists or not
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func TestSupportedSubsystems(t *testing.T) {
	subsystems, err := SupportedSubsystems("testdata/docker")
	if err != nil {
		t.Fatal(err)
	}

	assert.Len(t, subsystems, 11)
	assertContains(t, subsystems, "cpuset")
	assertContains(t, subsystems, "cpu")
	assertContains(t, subsystems, "cpuacct")
	assertContains(t, subsystems, "blkio")
	assertContains(t, subsystems, "memory")
	assertContains(t, subsystems, "devices")
	assertContains(t, subsystems, "freezer")
	assertContains(t, subsystems, "net_cls")
	assertContains(t, subsystems, "perf_event")
	assertContains(t, subsystems, "net_prio")
	assertContains(t, subsystems, "pids")

	_, found := subsystems["hugetlb"]
	assert.False(t, found, "hugetlb should be missing because it's disabled")
}

func TestSupportedSubsystemsErrCgroupsMissing(t *testing.T) {
	_, err := SupportedSubsystems("testdata/doesnotexist")
	if err != ErrCgroupsMissing {
		t.Fatal("expected ErrCgroupsMissing, but got %v", err)
	}
}

func TestSubsystemMountpoints(t *testing.T) {
	subsystems := map[string]struct{}{}
	subsystems["blkio"] = struct{}{}
	subsystems["cpu"] = struct{}{}
	subsystems["cpuacct"] = struct{}{}
	subsystems["cpuset"] = struct{}{}
	subsystems["devices"] = struct{}{}
	subsystems["freezer"] = struct{}{}
	subsystems["hugetlb"] = struct{}{}
	subsystems["memory"] = struct{}{}
	subsystems["perf_event"] = struct{}{}

	mountpoints, err := SubsystemMountpoints("testdata/docker", subsystems)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "testdata/docker/sys/fs/cgroup/blkio", mountpoints["blkio"])
	assert.Equal(t, "testdata/docker/sys/fs/cgroup/cpu", mountpoints["cpu"])
	assert.Equal(t, "testdata/docker/sys/fs/cgroup/cpuacct", mountpoints["cpuacct"])
	assert.Equal(t, "testdata/docker/sys/fs/cgroup/cpuset", mountpoints["cpuset"])
	assert.Equal(t, "testdata/docker/sys/fs/cgroup/devices", mountpoints["devices"])
	assert.Equal(t, "testdata/docker/sys/fs/cgroup/freezer", mountpoints["freezer"])
	assert.Equal(t, "testdata/docker/sys/fs/cgroup/hugetlb", mountpoints["hugetlb"])
	assert.Equal(t, "testdata/docker/sys/fs/cgroup/memory", mountpoints["memory"])
	assert.Equal(t, "testdata/docker/sys/fs/cgroup/perf_event", mountpoints["perf_event"])
}

func TestProcessCgroupPaths(t *testing.T) {
	paths, err := ProcessCgroupPaths("testdata/docker", 985)
	if err != nil {
		t.Fatal(err)
	}

	path := "/docker/b29faf21b7eff959f64b4192c34d5d67a707fe8561e9eaa608cb27693fba4242"
	assert.Equal(t, path, paths["blkio"])
	assert.Equal(t, path, paths["cpu"])
	assert.Equal(t, path, paths["cpuacct"])
	assert.Equal(t, path, paths["cpuset"])
	assert.Equal(t, path, paths["devices"])
	assert.Equal(t, path, paths["freezer"])
	assert.Equal(t, path, paths["memory"])
	assert.Equal(t, path, paths["net_cls"])
	assert.Equal(t, path, paths["net_prio"])
	assert.Equal(t, path, paths["perf_event"])
	assert.Len(t, paths, 10)
}

func assertContains(t testing.TB, m map[string]struct{}, key string) {
	_, contains := m[key]
	if !contains {
		t.Errorf("map is missing key %v, map=%+v", key, m)
	}
}

func TestParseMountinfoLine(t *testing.T) {
	lines := []string{
		"30 24 0:25 / /sys/fs/cgroup/blkio rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw,blkio",
		"30 24 0:25 / /sys/fs/cgroup/blkio rw,nosuid,nodev,noexec,relatime shared:13 - cgroup cgroup rw,blkio",
		"30 24 0:25 / /sys/fs/cgroup/blkio rw,nosuid,nodev,noexec,relatime shared:13 master:1 - cgroup cgroup rw,blkio",
		"30 24 0:25 / /sys/fs/cgroup/blkio rw,nosuid,nodev,noexec,relatime shared:13 - cgroup cgroup rw,name=blkio",
	}

	for _, line := range lines {
		mount, err := parseMountinfoLine(line)
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, "/sys/fs/cgroup/blkio", mount.mountpoint)
		assert.Equal(t, "cgroup", mount.filesystemType)
		assert.Len(t, mount.superOptions, 2)
	}
}
