package BrowserEnv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	"private_browser_client/Settings"
)

// ListBrowserEnvRebuildCandidates 只读扫描本机环境包目录，返回可重建 SQLite 索引的候选。
//
// 设计来源：
// - SQLite 是本机列表/状态索引，但用户明确提出数据库损坏时需要能从环境包文件恢复；
// - 候选扫描不能写库，避免把不完整或伪造目录自动纳入系统；
// - 每个候选都复用原子包校验器，缺少 profile/binding/proxy/fingerprint/browser-data 时只报告 invalid。
func (s *Service) ListBrowserEnvRebuildCandidates() (*model.BrowserEnvRebuildCandidatesResponse, error) {
	root := filepath.Join(Settings.Conf.ProjectRoot, "data", "browser-envs", "users")
	items := []model.BrowserEnvRebuildCandidate{}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return &model.BrowserEnvRebuildCandidatesResponse{Total: 0, Items: items}, nil
	} else if err != nil {
		return nil, internalError(fmt.Sprintf("读取 rebuild 根目录失败: %v", err))
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "profile.json" {
			return nil
		}
		envPath := filepath.Dir(path)
		candidate := buildRebuildCandidate(envPath)
		items = append(items, candidate)
		return nil
	})
	if err != nil {
		return nil, internalError(fmt.Sprintf("扫描 rebuild 候选失败: %v", err))
	}
	return &model.BrowserEnvRebuildCandidatesResponse{Total: len(items), Items: items}, nil
}

func buildRebuildCandidate(envPath string) model.BrowserEnvRebuildCandidate {
	relativePath := relativeProjectPath(envPath)
	candidate := model.BrowserEnvRebuildCandidate{
		EnvPath: relativePath,
		Status:  rebuildCandidateInvalid,
		Errors:  []string{},
	}
	pkg, err := loadAndValidateAtomicPackage(envPath)
	if err != nil {
		candidate.Errors = append(candidate.Errors, err.Error())
		return candidate
	}
	candidate.EnvID = pkg.Profile.EnvID
	candidate.UserID = pkg.Profile.UserID
	candidate.RPAType = pkg.Profile.RPAType
	candidate.Name = pkg.Profile.Name
	candidate.Status = rebuildCandidateCreatedAtomic
	candidate.Indexed = browserEnvIndexExists(pkg.Profile.EnvID)
	if expected := expectedEnvRelativePath(pkg.Profile); relativePath != expected {
		candidate.Status = rebuildCandidateInvalid
		candidate.Errors = append(candidate.Errors, "环境包路径与 profile.envId/userId/rpaType 不一致")
	}
	if pkg.HasContainerFile {
		candidate.Status = "verified_atomic"
	}
	return candidate
}

func browserEnvIndexExists(envID string) bool {
	_, err := browserEnvDao.NewRuntimeModelHandler().GetBrowserEnvIndexByID(context.Background(), envID)
	return err == nil
}

// RebuildBrowserEnvIndex 把一个原子完整的本机环境包目录恢复为 SQLite 索引。
//
// 职责边界：
// - 一次只处理一个 envId；
// - 不启动 Docker、不拉镜像、不创建容器、不验证最终网络指纹；
// - 只修正本机运行资源字段：envSequence、CDP/VNC 端口、container.json 运行摘要和 proxyRuntime pending。
func (s *Service) RebuildBrowserEnvIndex(envID string) (*model.BrowserEnvRebuildIndexResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}

	runEnvMu.Lock()
	defer runEnvMu.Unlock()

	if _, err := browserEnvDao.NewRuntimeModelHandler().GetBrowserEnvIndexByID(context.Background(), envID); err == nil {
		return nil, conflictError("envId 已存在 SQLite 索引，不能重复 rebuild-index")
	} else if !errors.Is(err, browserEnvDao.ErrBrowserEnvNotFound) {
		return nil, internalError(err.Error())
	}

	envPath, pkg, err := findRebuildPackageByEnvID(envID)
	if err != nil {
		return nil, err
	}
	if err = ensureNoDockerConflictForAdmission(pkg.Profile.EnvID, containerNameForProfile(pkg.Profile)); err != nil {
		return nil, err
	}

	envSequence, ports, err := rebuildRuntimePorts(pkg.Profile)
	if err != nil {
		return nil, internalError(err.Error())
	}
	now := time.Now().Unix()
	profile := pkg.Profile
	binding := pkg.Binding
	container := pkg.Container
	if !pkg.HasContainerFile {
		container = model.ContainerFile{EnvID: profile.EnvID, Image: profile.Runtime.Image}
	}
	profile.EnvSequence = envSequence
	profile.Ports = ports
	profile.LastRuntime = model.PackageLastRuntime{}
	profile.Package = model.ProfilePackageMetadata{}
	profile.Metadata.UpdatedAt = now
	container.ContainerName = containerNameForProfile(profile)
	container.ContainerID = nil
	container.Image = profile.Runtime.Image
	container.Status = model.BrowserEnvStatusCreated
	container.Ports = ports
	container.Docker.APIURL = Settings.Conf.DockerConfig.APIURL
	container.Docker.DeviceArch = nil
	container.StartedAt = nil
	container.StoppedAt = nil
	container.UpdatedAt = now
	container.Labels = map[string]string{
		"bv.project":       "private-browser-client",
		"bv.role":          "browser-env",
		"bv.envId":         profile.EnvID,
		"bv.userId":        profile.UserID,
		"bv.rpaType":       profile.RPAType,
		"bv.schemaVersion": fmt.Sprintf("%d", model.SchemaVersion),
	}
	binding.RuntimeProtection.TimezoneStatus = "pending"
	binding.RuntimeProtection.RiskStatus = "pending"
	binding.RuntimeProtection.AvailabilityStatus = "pending"
	binding.UpdatedAt = now

	if err = writePackageJSON(envPath, profile.Paths.Profile, profile); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writePackageJSON(envPath, profile.Paths.Binding, binding); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writePackageJSON(envPath, profile.Paths.Container, container); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writeTimezoneProbePending(envPath, profile.Paths.ProxyRuntime); err != nil {
		return nil, internalError(err.Error())
	}

	relativePath := relativeProjectPath(envPath)
	containerName := container.ContainerName
	index := &model.BrowserEnvIndex{
		EnvID:               profile.EnvID,
		UserID:              profile.UserID,
		RPAType:             profile.RPAType,
		Name:                profile.Name,
		EnvSequence:         envSequence,
		CDPPort:             ports.CDP,
		VNCPort:             ports.VNC,
		EnvPath:             relativePath,
		Status:              model.BrowserEnvStatusCreated,
		ContainerName:       &containerName,
		ContainerStatus:     model.BrowserEnvContainerStatusUnknown,
		MonitorStatus:       model.BrowserEnvMonitorStatusUnknown,
		FingerprintRestored: binding.Fingerprint.Restored,
		HasBrowserData:      true,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err = browserEnvDao.NewCreateModelHandler().CreateBrowserEnvIndex(context.Background(), index); err != nil {
		if errors.Is(err, browserEnvDao.ErrDuplicateBrowserEnv) {
			return nil, conflictError("envId 已存在 SQLite 索引，不能重复 rebuild-index")
		}
		return nil, internalError(err.Error())
	}
	return &model.BrowserEnvRebuildIndexResponse{
		EnvID:     profile.EnvID,
		UserID:    profile.UserID,
		RPAType:   profile.RPAType,
		EnvPath:   relativePath,
		Status:    model.BrowserEnvStatusCreated,
		Ports:     ports,
		RebuiltAt: now,
	}, nil
}

func findRebuildPackageByEnvID(envID string) (string, *atomicPackage, error) {
	root := filepath.Join(Settings.Conf.ProjectRoot, "data", "browser-envs", "users")
	var foundPath string
	var foundPackage *atomicPackage
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "profile.json" {
			return nil
		}
		envPath := filepath.Dir(path)
		pkg, err := loadAndValidateAtomicPackage(envPath)
		if err != nil || pkg.Profile.EnvID != envID {
			return nil
		}
		if foundPackage != nil {
			return fmt.Errorf("发现多个相同 envId 的环境包目录")
		}
		foundPath = envPath
		foundPackage = pkg
		return nil
	})
	if err != nil {
		return "", nil, internalError(fmt.Sprintf("扫描 rebuild 目录失败: %v", err))
	}
	if foundPackage == nil {
		return "", nil, notFoundError("没有找到可重建的原子环境包")
	}
	if expected := expectedEnvRelativePath(foundPackage.Profile); relativeProjectPath(foundPath) != expected {
		return "", nil, invalidError("环境包路径与 profile.envId/userId/rpaType 不一致")
	}
	return foundPath, foundPackage, nil
}

func rebuildRuntimePorts(profile model.ProfileFile) (int, model.BrowserEnvPorts, error) {
	if profile.EnvSequence > 0 && profile.Ports.CDP > 0 && profile.Ports.VNC > 0 &&
		ensureTCPPortAvailable(profile.Ports.CDP) == nil && ensureTCPPortAvailable(profile.Ports.VNC) == nil {
		return profile.EnvSequence, profile.Ports, nil
	}
	return nextAvailableEnvSequenceAndPorts()
}

func expectedEnvRelativePath(profile model.ProfileFile) string {
	return filepath.ToSlash(filepath.Join("data", "browser-envs", "users", profile.UserID, profile.RPAType, profile.EnvID))
}

func relativeProjectPath(absPath string) string {
	rel, err := filepath.Rel(Settings.Conf.ProjectRoot, absPath)
	if err != nil {
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(rel)
}

func containerNameForProfile(profile model.ProfileFile) string {
	return "bv-" + strings.ReplaceAll(profile.EnvID, "_", "-")
}
