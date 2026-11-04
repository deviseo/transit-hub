#!/usr/bin/env bash
# 打包在线更新用的发布文件，放在 dist/release/<版本>/：
#   transithub_<版本>_linux_amd64.tar.gz、transithub_<版本>_linux_arm64.tar.gz、checksums.txt
# 每个包的根目录是服务端程序 transithub-api 和前端文件 public/。发布时把这三个文件上传到 GitHub Release：
# 在线更新只从这个仓库的 Release 下载，并用 checksums.txt 校验。
#
# 用法：scripts/build-release.sh v1.0.0
set -euo pipefail

version="${1:-}"
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "usage: $0 vX.Y.Z" >&2
  exit 1
fi
root="$(cd "$(dirname "$0")/.." && pwd)"

# 程序里写死的版本号必须和发布的版本一致：在线更新靠它判断新版本有没有启动起来。
code_version="$(sed -n 's/^[[:space:]]*defaultAppVersion[[:space:]]*=[[:space:]]*"\(.*\)"[[:space:]]*$/\1/p' "$root/backend/internal/config/config.go")"
if [[ "$code_version" != "$version" ]]; then
  echo "backend/internal/config/config.go has defaultAppVersion \"$code_version\", expected \"$version\"" >&2
  exit 1
fi

out="$root/dist/release/$version"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
rm -rf "$out"
mkdir -p "$out"
# 包里文件的时间统一用最近一次提交的时间，同样的代码打出同样的包。
mtime="$(git -C "$root" log -1 --format=%ct)"

echo "==> frontend"
(cd "$root/frontend" && VITE_API_BASE_URL=/api npm run build -- --outDir "$work/public" --emptyOutDir)

for arch in amd64 arm64; do
  echo "==> backend linux/$arch"
  (cd "$root/backend" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags="-s -w" -o "$work/transithub-api-$arch" ./cmd/api)
  python3 -I - "$work/transithub-api-$arch" "$work/public" "$out/transithub_${version}_linux_${arch}.tar.gz" "$mtime" <<'PY'
# 只放普通文件和目录（在线更新拒绝其他类型），属主统一为 0，不带 macOS 的扩展属性。
import gzip
import os
import sys
import tarfile

binary, public, target, mtime = sys.argv[1], sys.argv[2], sys.argv[3], int(sys.argv[4])


def entry(name, kind, mode, size=0):
    info = tarfile.TarInfo(name)
    info.type, info.mode, info.size, info.mtime = kind, mode, size, mtime
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    return info


def add_file(archive, path, name, mode):
    if os.path.islink(path) or not os.path.isfile(path):
        raise SystemExit(f"not a regular file: {path}")
    with open(path, "rb") as handle:
        archive.addfile(entry(name, tarfile.REGTYPE, mode, os.path.getsize(path)), handle)


with open(target, "wb") as raw, gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=mtime) as compressed:
    with tarfile.open(fileobj=compressed, mode="w", format=tarfile.PAX_FORMAT) as archive:
        add_file(archive, binary, "transithub-api", 0o755)
        archive.addfile(entry("public", tarfile.DIRTYPE, 0o755))
        for folder, dirs, files in os.walk(public):
            dirs.sort()
            relative = os.path.relpath(folder, public)
            prefix = "public" if relative == "." else "public/" + relative.replace(os.sep, "/")
            for name in dirs:
                if os.path.islink(os.path.join(folder, name)):
                    raise SystemExit(f"symlink in public: {name}")
                archive.addfile(entry(f"{prefix}/{name}", tarfile.DIRTYPE, 0o755))
            for name in sorted(files):
                if name == ".DS_Store" or name.startswith("._"):
                    continue
                add_file(archive, os.path.join(folder, name), f"{prefix}/{name}", 0o644)
PY
done

(cd "$out" && shasum -a 256 transithub_*.tar.gz > checksums.txt)
echo "==> $out"
ls -l "$out"
cat "$out/checksums.txt"
