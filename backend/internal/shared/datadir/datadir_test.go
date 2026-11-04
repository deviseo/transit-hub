package datadir

import "testing"

// 取自真实容器的 /proc/self/mountinfo（节选）。
const withVolume = `655 573 0:45 / / rw,relatime master:9 - overlay overlay rw,lowerdir=/var/lib/docker/overlay2/l/A
656 655 0:49 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw
661 655 254:1 /docker/volumes/app /app/data rw,relatime - ext4 /dev/vda1 rw
662 655 254:1 /docker/containers/x/hostname /etc/hostname rw,relatime - ext4 /dev/vda1 rw`

const withoutVolume = `655 573 0:45 / / rw,relatime master:9 - overlay overlay rw,lowerdir=/var/lib/docker/overlay2/l/A
656 655 0:49 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw
662 655 254:1 /docker/containers/x/hostname /etc/hostname rw,relatime - ext4 /dev/vda1 rw`

const hostInstall = `22 1 8:1 / / rw,relatime shared:1 - ext4 /dev/sda1 rw
25 22 0:22 / /proc rw,nosuid,nodev,noexec,relatime shared:12 - proc proc rw`

const spacedMount = `655 573 0:45 / / rw,relatime - overlay overlay rw
661 655 254:1 /x /srv/my\040data rw,relatime - virtiofs data rw`

func TestPersistentIn(t *testing.T) {
	cases := []struct {
		name      string
		mountinfo string
		dir       string
		want      bool
	}{
		{"volume mounted", withVolume, "/app/data", true},
		{"subdirectory of a volume", withVolume, "/app/data/updates", true},
		{"container without the volume", withoutVolume, "/app/data", false},
		{"a different directory in the same container", withVolume, "/tmp/data", false},
		{"installed directly on a host", hostInstall, "/var/lib/transithub", true},
		{"escaped mount point", spacedMount, "/srv/my data/app", true},
		{"no mountinfo", "", "/app/data", true},
	}
	for _, tc := range cases {
		if got := persistentIn(tc.mountinfo, tc.dir); got != tc.want {
			t.Errorf("%s: persistentIn(%q) = %v, want %v", tc.name, tc.dir, got, tc.want)
		}
	}
}
