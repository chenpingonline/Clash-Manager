// Package appupdate verifies fnOS update packages before handing them to App Center.
package appupdate

import (
	"archive/tar"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
)

const Repository = "chenpingonline/Clash-for-fnos"
const AppName = "clash-for-fnos"
const MaxPackageSize int64 = 512 << 20

var versionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func Version(value string) (string, error) {
	if !versionPattern.MatchString(value) {
		return "", errors.New("更新版本号无效")
	}
	return strings.TrimPrefix(value, "v"), nil
}

func Newer(target, current string) bool {
	a, err := Version(target)
	if err != nil {
		return false
	}
	b, err := Version(current)
	if err != nil {
		return false
	}
	x, y := strings.Split(a, "."), strings.Split(b, ".")
	for i := range x {
		left, e1 := strconv.ParseUint(x[i], 10, 64)
		right, e2 := strconv.ParseUint(y[i], 10, 64)
		if e1 != nil || e2 != nil {
			return false
		}
		if left != right {
			return left > right
		}
	}
	return false
}

func Manifest(body string) (map[string]string, error) {
	result := map[string]string{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, errors.New("FPK manifest 格式无效")
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("FPK manifest 字段重复: %s", key)
		}
		result[key] = value
	}
	return result, nil
}

// Verify never extracts or executes archive contents. The SHA-256 comes from
// GitHub's Release asset metadata, independently of the downloaded archive.
func Verify(filename, digest, target, arch string) error {
	expected, err := hex.DecodeString(strings.TrimPrefix(digest, "sha256:"))
	if err != nil || len(expected) != sha256.Size || !strings.HasPrefix(digest, "sha256:") {
		return errors.New("更新包缺少有效的 SHA-256 校验信息")
	}
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	sha := sha256.New()
	n, err := io.Copy(sha, io.LimitReader(f, MaxPackageSize+1))
	if err != nil {
		return err
	}
	if n > MaxPackageSize {
		return errors.New("更新包超过大小限制")
	}
	if hex.EncodeToString(sha.Sum(nil)) != hex.EncodeToString(expected) {
		return errors.New("更新包 SHA-256 校验失败")
	}
	if _, err = f.Seek(0, 0); err != nil {
		return err
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	expanded := &io.LimitedReader{R: gz, N: MaxPackageSize*2 + 1}
	reader := tar.NewReader(expanded)
	var manifest map[string]string
	var payload string
	seen := map[string]bool{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(header.Name, "./")
		clean := path.Clean(name)
		if clean == "." && header.Typeflag == tar.TypeDir {
			continue
		}
		if path.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, "../") || clean != strings.TrimSuffix(name, "/") {
			return errors.New("FPK 包含不安全路径")
		}
		if seen[clean] {
			return errors.New("FPK 包含重复文件")
		}
		seen[clean] = true
		if len(seen) > 4096 {
			return errors.New("FPK 包含过多文件")
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return errors.New("FPK 包含不支持的文件类型")
		}
		if header.Size < 0 || header.Size > MaxPackageSize {
			return errors.New("FPK 文件超过大小限制")
		}
		switch clean {
		case "manifest":
			if header.Typeflag != tar.TypeReg || header.Size > 64<<10 {
				return errors.New("FPK manifest 无效")
			}
			body, err := io.ReadAll(reader)
			if err != nil {
				return err
			}
			manifest, err = Manifest(string(body))
			if err != nil {
				return err
			}
		case "app.tgz":
			if header.Typeflag != tar.TypeReg {
				return errors.New("FPK app.tgz 无效")
			}
			hash := md5.New()
			if _, err = io.Copy(hash, reader); err != nil {
				return err
			}
			payload = hex.EncodeToString(hash.Sum(nil))
		}
	}
	// Consume the gzip footer too, so truncation/checksum errors cannot be hidden
	// behind tar's end-of-archive marker.
	if _, err = io.Copy(io.Discard, expanded); err != nil {
		return err
	}
	if expanded.N == 0 {
		return errors.New("FPK 解压后超过大小限制")
	}
	v, err := Version(target)
	if err != nil {
		return err
	}
	if manifest["appname"] != AppName || manifest["version"] != v {
		return errors.New("FPK 应用名称或版本不匹配")
	}
	platform := map[string]string{"amd64": "x86", "arm64": "arm"}[arch]
	if platform == "" || (manifest["platform"] != platform && manifest["platform"] != "all") {
		return errors.New("FPK 不支持当前架构")
	}
	if payload == "" || manifest["checksum"] != payload {
		return errors.New("FPK 内容校验失败")
	}
	return nil
}
