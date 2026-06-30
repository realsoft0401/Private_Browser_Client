package BrowserEnv

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	common "private_browser_client/Repository/Common"
	edgeService "private_browser_client/Service/Edge"
	"private_browser_client/Settings"
)

const maxImportPackageBytes = 20 << 30

type loadedPackage struct {
	EnvPath      string
	Profile      model.ProfileFile
	Binding      model.BindingFile
	Container    model.ContainerFile
	HasContainer bool
	ProxyConfig  string
}

func loadBrowserEnvIndexOrFail(envID string) (*model.BrowserEnvIndex, error) {
	index, err := browserEnvDao.NewRuntimeModelHandler().GetBrowserEnvIndexByID(envID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil, notFoundError("环境包不存在")
		}
		return nil, internalError(err.Error())
	}
	return index, nil
}

func resolveManagedEnvPath(index *model.BrowserEnvIndex) (string, error) {
	if index == nil {
		return "", internalError("环境包索引不能为空")
	}
	if Settings.Conf.ProjectRoot == "" {
		return "", internalError("project root 不能为空")
	}
	absolute := filepath.Clean(filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(index.EnvPath)))
	root := filepath.Clean(filepath.Join(Settings.Conf.ProjectRoot, "data", "browser-envs"))
	if absolute != root && !strings.HasPrefix(absolute, root+string(os.PathSeparator)) {
		return "", conflictError("envPath 不在受控 browser-envs 目录内")
	}
	if filepath.Base(absolute) != index.EnvID {
		return "", conflictError("envPath 最后一层必须等于 envId")
	}
	return absolute, nil
}

func managedBackupArchivePath(index *model.BrowserEnvIndex) (string, string, error) {
	if index == nil {
		return "", "", internalError("环境包索引不能为空")
	}
	relative := filepath.ToSlash(filepath.Join("data", "browser-envs", "users", index.UserID, index.RPAType, buildBackupArchiveFileName(index.EnvID, time.Now().Unix())))
	return filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(relative)), relative, nil
}

func buildBackupArchiveFileName(envID string, unixSeconds int64) string {
	return fmt.Sprintf("%s-backup-%d.tar.gz", envID, unixSeconds)
}

func resolveManagedBackupPath(index *model.BrowserEnvIndex) (string, error) {
	if index == nil {
		return "", internalError("环境包索引不能为空")
	}
	if index.BackupPath == nil || strings.TrimSpace(*index.BackupPath) == "" {
		return "", conflictError("环境包没有可恢复的备份包")
	}
	absolute := filepath.Clean(filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(*index.BackupPath)))
	root := filepath.Clean(filepath.Join(Settings.Conf.ProjectRoot, "data", "browser-envs"))
	if absolute != root && !strings.HasPrefix(absolute, root+string(os.PathSeparator)) {
		return "", conflictError("backupPath 不在受控目录内")
	}
	return absolute, nil
}

func verifyBackupArchiveFile(index *model.BrowserEnvIndex, backupAbs string) error {
	stat, err := os.Stat(backupAbs)
	if err != nil {
		return internalError(fmt.Sprintf("读取备份包失败: %v", err))
	}
	if stat.IsDir() || stat.Size() <= 0 {
		return invalidError("备份包不是有效文件")
	}
	if index != nil && index.BackupChecksum != nil && strings.TrimSpace(*index.BackupChecksum) != "" {
		sum, err := fileSHA256(backupAbs)
		if err != nil {
			return internalError(fmt.Sprintf("计算备份包 checksum 失败: %v", err))
		}
		if sum != *index.BackupChecksum {
			return conflictError("备份包 checksum 不匹配，拒绝恢复")
		}
	}
	return nil
}

// isDockerImageAlreadyMissingError 识别 Docker remove image 的“镜像已不存在”受控结果。
//
// 设计来源：
// - `/del` 的目标是把本机不再需要的运行镜像清干净；
// - 如果镜像原本就不存在，业务上应视为已经达成“无需再删”的结果，而不是系统失败；
// - 这里单独收口字符串判断，是为了避免把 Docker 文案解析散落到 BrowserEnv Service 主链里。
//
// 职责边界：
// - 这里只识别“镜像已不存在”这类可接受结果；
// - 镜像仍被引用、Docker 不可达、权限不足等都不在这里吞掉，必须继续向上报错。
func isDockerImageAlreadyMissingError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "no such image") || strings.Contains(message, "not found")
}

func loadPackageProfileFromIndex(index *model.BrowserEnvIndex) (string, model.ProfileFile, error) {
	envPath, err := resolveManagedEnvPath(index)
	if err != nil {
		return "", model.ProfileFile{}, err
	}
	var profile model.ProfileFile
	if err = readJSONFile(filepath.Join(envPath, "profile.json"), &profile); err != nil {
		return "", model.ProfileFile{}, internalError(fmt.Sprintf("读取 profile.json 失败: %v", err))
	}
	if profile.EnvID != index.EnvID || profile.UserID != index.UserID || profile.RPAType != index.RPAType {
		return "", model.ProfileFile{}, conflictError("profile 与 browser_envs 索引不一致")
	}
	if err = validateProfilePackagePaths(profile.Paths); err != nil {
		return "", model.ProfileFile{}, invalidError(err.Error())
	}
	return envPath, profile, nil
}

func validateProfilePackagePaths(paths model.PackagePaths) error {
	required := []string{
		paths.Profile,
		paths.Binding,
		paths.Container,
		paths.BrowserData,
		paths.FingerprintSnapshot,
		paths.FingerprintBackup,
		paths.ProxyConfig,
		paths.ProxyRuntime,
	}
	for _, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("profile.paths 存在空字段")
		}
	}
	return nil
}

func buildBindingIdentityFromFacts(envID string, userID string, rpaType string) model.BindingIdentity {
	return model.BindingIdentity{
		EnvID:   envID,
		UserID:  userID,
		RPAType: rpaType,
	}
}

func loadPackage(index *model.BrowserEnvIndex) (*loadedPackage, error) {
	envPath, profile, err := loadPackageProfileFromIndex(index)
	if err != nil {
		return nil, err
	}
	var binding model.BindingFile
	if err = readPackageJSON(envPath, profile.Paths.Binding, &binding); err != nil {
		return nil, invalidError(fmt.Sprintf("读取 binding 失败: %v", err))
	}
	var container model.ContainerFile
	hasContainer := false
	if err = readPackageJSON(envPath, profile.Paths.Container, &container); err == nil {
		hasContainer = true
	}
	proxyConfig := ""
	if profile.Proxy.Enabled {
		body, err := os.ReadFile(filepath.Join(envPath, filepath.FromSlash(profile.Paths.ProxyConfig)))
		if err != nil {
			return nil, invalidError(fmt.Sprintf("读取代理配置失败: %v", err))
		}
		proxyConfig = string(body)
	}
	return &loadedPackage{
		EnvPath:      envPath,
		Profile:      profile,
		Binding:      binding,
		Container:    container,
		HasContainer: hasContainer,
		ProxyConfig:  proxyConfig,
	}, nil
}

func validateAtomicPackage(index *model.BrowserEnvIndex) (*loadedPackage, error) {
	pkg, err := loadPackage(index)
	if err != nil {
		return nil, err
	}
	files := []string{
		pkg.Profile.Paths.Profile,
		pkg.Profile.Paths.Binding,
		pkg.Profile.Paths.ProxyRuntime,
		pkg.Profile.Paths.FingerprintSnapshot,
		pkg.Profile.Paths.FingerprintBackup,
	}
	for _, file := range files {
		if err = requirePackageFile(pkg.EnvPath, file); err != nil {
			return nil, invalidError(err.Error())
		}
	}
	if pkg.Profile.Proxy.Enabled {
		if err = requirePackageFile(pkg.EnvPath, pkg.Profile.Paths.ProxyConfig); err != nil {
			return nil, invalidError(err.Error())
		}
	}
	if err = requirePackageDir(pkg.EnvPath, pkg.Profile.Paths.BrowserData); err != nil {
		return nil, invalidError(err.Error())
	}
	identityHash, err := buildJSONHash(buildBindingIdentityFromFacts(pkg.Profile.EnvID, pkg.Profile.UserID, pkg.Profile.RPAType))
	if err != nil {
		return nil, internalError(fmt.Sprintf("计算 identityHash 失败: %v", err))
	}
	if pkg.Profile.IdentityHash != identityHash || pkg.Binding.IdentityHash != identityHash {
		return nil, conflictError("profile/binding identityHash 不一致")
	}
	return pkg, nil
}

func requirePackageFile(envPath string, relativePath string) error {
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s 缺失", relativePath)
	}
	if stat.IsDir() {
		return fmt.Errorf("%s 必须是文件", relativePath)
	}
	return nil
}

func requirePackageDir(envPath string, relativePath string) error {
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s 缺失", relativePath)
	}
	if !stat.IsDir() {
		return fmt.Errorf("%s 必须是目录", relativePath)
	}
	return nil
}

func readPackageJSON(envPath string, relativePath string, target any) error {
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return err
	}
	return readJSONFile(path, target)
}

func safePackagePath(envPath string, relativePath string) (string, error) {
	base := filepath.Clean(envPath)
	candidate := filepath.Clean(filepath.Join(base, filepath.FromSlash(relativePath)))
	if candidate != base && !strings.HasPrefix(candidate, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("环境包路径越界: %s", relativePath)
	}
	return candidate, nil
}

func readJSONFile(path string, target any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func writePackageJSON(envPath string, relativePath string, value any) error {
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return err
	}
	return writeJSONFile(path, value)
}

func writePackageText(envPath string, relativePath string, value string) error {
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return err
	}
	return writeTextFile(path, value)
}

func writeTimezoneProbePending(envPath string, relativePath string) error {
	source := "container-probe"
	return writePackageJSON(envPath, relativePath, model.ProxyRuntimeFile{
		Source: &source,
		Status: "pending",
		Drift:  false,
	})
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func copyFile(source string, target string, mode os.FileMode) error {
	from, err := os.Open(source)
	if err != nil {
		return err
	}
	defer from.Close()
	if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	to, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer to.Close()
	if _, err = io.Copy(to, from); err != nil {
		return err
	}
	return to.Sync()
}

func copyDirectory(source string, target string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, relative)
		if info.IsDir() {
			return os.MkdirAll(dest, info.Mode())
		}
		return copyFile(path, dest, info.Mode())
	})
}

func createTarGzFromDirectory(sourceDir string, rootName string, archivePath string) error {
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return err
	}
	file, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			base := filepath.Base(path)
			if strings.HasPrefix(base, "Singleton") {
				return nil
			}
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			name := filepath.ToSlash(filepath.Join(rootName, relative))
			header, err := tar.FileInfoHeader(info, linkTarget)
			if err != nil {
				return err
			}
			header.Name = name
			return tarWriter.WriteHeader(header)
		}
		name := filepath.ToSlash(filepath.Join(rootName, relative))
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = name
		if info.IsDir() && !strings.HasSuffix(header.Name, "/") {
			header.Name += "/"
		}
		if err = tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()
		_, err = io.Copy(tarWriter, source)
		return err
	})
}

func extractImportTarGz(file io.Reader, targetDir string) error {
	limited := io.LimitReader(file, maxImportPackageBytes)
	gzipReader, err := gzip.NewReader(limited)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		cleanName := filepath.Clean(header.Name)
		targetPath := filepath.Join(targetDir, cleanName)
		if targetPath != targetDir && !strings.HasPrefix(targetPath, targetDir+string(os.PathSeparator)) {
			return fmt.Errorf("tar 路径越界: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err = os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err = io.Copy(out, tarReader); err != nil {
				_ = out.Close()
				return err
			}
			if err = out.Close(); err != nil {
				return err
			}
		}
	}
}

func findImportPackageRoot(stagingRoot string) (string, error) {
	entries, err := os.ReadDir(stagingRoot)
	if err != nil {
		return "", err
	}
	dirs := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(stagingRoot, entry.Name()))
		}
	}
	if len(dirs) != 1 {
		return "", fmt.Errorf("导入包必须只有一个根目录")
	}
	return dirs[0], nil
}

func decodeProxyConfigBase64(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	value = strings.ReplaceAll(value, "\n", "")
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\t", "")
	value = strings.ReplaceAll(value, " ", "")
	if value == "" {
		return "", invalidError("proxy.configBase64 不能为空")
	}
	bytes, err := base64DecodeString(value)
	if err != nil {
		return "", invalidError("proxy.configBase64 不是有效 Base64")
	}
	return string(bytes), nil
}

func base64DecodeString(value string) ([]byte, error) {
	bytes, err := base64.StdEncoding.DecodeString(value)
	if err == nil {
		return bytes, nil
	}
	return base64.RawStdEncoding.DecodeString(value)
}

func normalizeClashMode(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "rule", "global", "direct":
		return value, nil
	default:
		return "", invalidError("mode 仅支持 rule/global/direct")
	}
}

func replaceClashMode(config string, mode string) (string, bool, error) {
	if strings.TrimSpace(config) == "" {
		return "", false, invalidError("代理配置为空，不能切换代理模式")
	}
	re := regexp.MustCompile(`(?m)^(\s*)mode\s*:\s*([A-Za-z_-]+)\s*$`)
	match := re.FindStringSubmatch(config)
	if len(match) == 3 {
		current := strings.ToLower(strings.TrimSpace(match[2]))
		if current == mode {
			return config, false, nil
		}
		return re.ReplaceAllString(config, "${1}mode: "+mode), true, nil
	}
	return "mode: " + mode + "\n" + strings.TrimLeft(config, "\n"), true, nil
}

func optionalString(value string) *string {
	return &value
}

func optionalInt64(value int64) *int64 {
	return &value
}

func maybeRemoveContainer(containerID *string) error {
	if containerID == nil || strings.TrimSpace(*containerID) == "" {
		return nil
	}
	err := edgeService.NewEdgeService().RemoveDockerContainer(*containerID, true)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "404") && !strings.Contains(strings.ToLower(err.Error()), "no such container") {
		return err
	}
	return nil
}
