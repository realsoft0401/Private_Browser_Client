package Routes

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqliteInfra "private_browser_client/Infrastructures/SQLite"
	"private_browser_client/Settings"
)

func TestMain(m *testing.M) {
	projectRoot, err := prepareTestProjectRoot()
	if err != nil {
		panic(err)
	}
	if err = Settings.Init(projectRoot); err != nil {
		panic(err)
	}
	if err = sqliteInfra.Init(); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = sqliteInfra.Close()
	os.Exit(code)
}

// 这里专门补 Routes 层 API 回归测试。
//
// 设计来源：
// - 当前桌面执行环境不允许真正监听 3300 端口，纯靠手工 curl 无法完成今天的封板回归；
// - 但正式接口至少要验证路由已挂载、静态文档可返回、JSON 包装格式稳定；
// - 因此这里用 httptest 直接打 Routes.Setup()，覆盖“接口入口是否真实可用”这层事实。
func TestCoreHTTPEntrypoints(t *testing.T) {
	router := Setup()

	t.Run("health", func(t *testing.T) {
		recorder := performRequest(router, http.MethodGet, "/health", nil, nil)
		assertStatusCode(t, recorder.Code, http.StatusOK)

		var body map[string]any
		decodeJSONBody(t, recorder.Body.Bytes(), &body)
		if int64(body["code"].(float64)) != 1000 {
			t.Fatalf("unexpected health code: %v", body["code"])
		}
	})

	t.Run("swagger page", func(t *testing.T) {
		recorder := performRequest(router, http.MethodGet, "/swagger", nil, nil)
		assertStatusCode(t, recorder.Code, http.StatusOK)
		if !strings.Contains(recorder.Body.String(), "swagger-ui") {
			t.Fatalf("swagger page missing swagger-ui marker")
		}
	})

	t.Run("openapi yaml", func(t *testing.T) {
		recorder := performRequest(router, http.MethodGet, "/openapi.yaml", nil, nil)
		assertStatusCode(t, recorder.Code, http.StatusOK)
		if !strings.Contains(recorder.Body.String(), "openapi: 3.0.3") {
			t.Fatalf("openapi yaml missing version header")
		}
	})

	t.Run("slot list", func(t *testing.T) {
		recorder := performRequest(router, http.MethodGet, "/api/v1/edge/slots", nil, nil)
		assertStatusCode(t, recorder.Code, http.StatusOK)

		var body map[string]any
		decodeJSONBody(t, recorder.Body.Bytes(), &body)
		if int64(body["code"].(float64)) != 1000 {
			t.Fatalf("unexpected slot list code: %v", body["code"])
		}
	})

	t.Run("slot detail not found", func(t *testing.T) {
		recorder := performRequest(router, http.MethodGet, "/api/v1/edge/slots/slot001", nil, nil)
		assertStatusCode(t, recorder.Code, http.StatusOK)

		var body map[string]any
		decodeJSONBody(t, recorder.Body.Bytes(), &body)
		if int64(body["code"].(float64)) == 1000 {
			t.Fatalf("expected not found style response for missing slot")
		}
	})

	t.Run("node registration status", func(t *testing.T) {
		recorder := performRequest(router, http.MethodGet, "/api/v1/edge/node-registration", nil, nil)
		assertStatusCode(t, recorder.Code, http.StatusOK)

		var body map[string]any
		decodeJSONBody(t, recorder.Body.Bytes(), &body)
		if int64(body["code"].(float64)) != 1000 {
			t.Fatalf("unexpected node registration status code: %v", body["code"])
		}
	})

	t.Run("node registration assign unauthorized", func(t *testing.T) {
		recorder := performRequest(
			router,
			http.MethodPost,
			"/api/v1/edge/node-registration/assign",
			strings.NewReader(`{"accountId":"906090119","clientId":"9060901190001"}`),
			map[string]string{"Content-Type": "application/json"},
		)
		assertStatusCode(t, recorder.Code, http.StatusOK)

		var body map[string]any
		decodeJSONBody(t, recorder.Body.Bytes(), &body)
		if int64(body["code"].(float64)) == 1000 {
			t.Fatalf("expected unauthorized style response when api key is missing")
		}
	})
}

func performRequest(handler http.Handler, method string, path string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, body)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeJSONBody(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("decode json body failed: %v, body=%s", err, string(body))
	}
}

func assertStatusCode(t *testing.T, actual int, expected int) {
	t.Helper()
	if actual != expected {
		t.Fatalf("unexpected status code: got=%d want=%d", actual, expected)
	}
}

func prepareTestProjectRoot() (string, error) {
	sourceRoot, err := findSourceProjectRoot()
	if err != nil {
		return "", err
	}
	tempRoot, err := os.MkdirTemp("", "private-browser-client-routes-test-*")
	if err != nil {
		return "", err
	}
	for _, name := range []string{"Settings", "docs", "public"} {
		if err = copyTree(filepath.Join(sourceRoot, name), filepath.Join(tempRoot, name)); err != nil {
			return "", err
		}
	}
	configPath := filepath.Join(tempRoot, "Settings", Settings.ConfigFileName)
	body, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	rewritten := strings.ReplaceAll(string(body), "enabled: true", "enabled: false")
	if err = os.WriteFile(configPath, []byte(rewritten), 0o644); err != nil {
		return "", err
	}
	return tempRoot, nil
}

func findSourceProjectRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(current, "Settings", Settings.ConfigFileName)); statErr == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}

func copyTree(sourceDir string, targetDir string) error {
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, relativePath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, body, info.Mode())
	})
}
