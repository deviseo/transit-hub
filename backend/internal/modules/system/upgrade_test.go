package system

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"transithub/backend/internal/config"
)

type fakeUpgradeExecutor struct {
	status     UpgradeStatusResponse
	statusErr  error
	startErr   error
	startCalls int
}

func newUpgradeTestService(executor UpgradeExecutor) *Service {
	restartExecutor := &fakeRestartExecutor{status: RestartStatusResponse{State: RestartStateIdle}}
	return newServiceWithExecutors(config.Config{}, executor, restartExecutor)
}

func (f *fakeUpgradeExecutor) Start(context.Context) error {
	f.startCalls++
	return f.startErr
}

func (f *fakeUpgradeExecutor) Status(context.Context) (UpgradeStatusResponse, error) {
	return f.status, f.statusErr
}

func TestStartUpgradeStartsFixedExecutor(t *testing.T) {
	executor := &fakeUpgradeExecutor{status: UpgradeStatusResponse{State: UpgradeStateIdle}}
	service := newUpgradeTestService(executor)

	response, err := service.StartUpgrade(context.Background())
	if err != nil {
		t.Fatalf("start upgrade: %v", err)
	}
	if executor.startCalls != 1 {
		t.Fatalf("expected one start call, got %d", executor.startCalls)
	}
	if response.State != UpgradeStateStarting || response.RequestedAt == "" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestStartUpgradeRejectsRunningUpgrade(t *testing.T) {
	executor := &fakeUpgradeExecutor{status: UpgradeStatusResponse{State: UpgradeStateRunning}}
	service := newUpgradeTestService(executor)

	_, err := service.StartUpgrade(context.Background())
	if !errors.Is(err, ErrUpgradeInProgress) {
		t.Fatalf("expected ErrUpgradeInProgress, got %v", err)
	}
	if executor.startCalls != 0 {
		t.Fatalf("executor must not start while running, got %d calls", executor.startCalls)
	}
}

func TestStartUpgradeRejectsRunningRestart(t *testing.T) {
	upgradeExecutor := &fakeUpgradeExecutor{status: UpgradeStatusResponse{State: UpgradeStateIdle}}
	restartExecutor := &fakeRestartExecutor{status: RestartStatusResponse{State: RestartStateRunning}}
	service := newServiceWithExecutors(config.Config{}, upgradeExecutor, restartExecutor)

	_, err := service.StartUpgrade(context.Background())
	if !errors.Is(err, ErrRestartInProgress) {
		t.Fatalf("expected ErrRestartInProgress, got %v", err)
	}
	if upgradeExecutor.startCalls != 0 {
		t.Fatalf("upgrade must not start during restart, got %d calls", upgradeExecutor.startCalls)
	}
}

func TestSystemdUpgradeExecutorStartsOnlyFixedUnit(t *testing.T) {
	var gotName string
	var gotArgs []string
	executor := &systemdUpgradeExecutor{
		runCommand: func(_ context.Context, name string, args ...string) ([]byte, error) {
			gotName = name
			gotArgs = append([]string(nil), args...)
			return nil, nil
		},
	}

	if err := executor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if gotName != "systemctl" {
		t.Fatalf("expected systemctl, got %q", gotName)
	}
	wantArgs := []string{"start", "--no-block", upgradeUnitName}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, gotArgs)
	}
}

func TestSystemdUpgradeExecutorReadsFailureLog(t *testing.T) {
	tempDir := t.TempDir()
	statusPath := filepath.Join(tempDir, "upgrade-status.json")
	logPath := filepath.Join(tempDir, "upgrade.log")
	statusJSON := `{"state":"failed","startedAt":"2026-07-31T08:00:00Z","finishedAt":"2026-07-31T08:01:00Z","exitCode":17}`
	if err := os.WriteFile(statusPath, []byte(statusJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("go build failed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &systemdUpgradeExecutor{statusPath: statusPath, logPath: logPath}

	status, err := executor.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.State != UpgradeStateFailed || status.ExitCode == nil || *status.ExitCode != 17 {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.Output != "go build failed\n" {
		t.Fatalf("unexpected output %q", status.Output)
	}
}

func TestSystemdUpgradeExecutorReturnsIdleWhenStatusDoesNotExist(t *testing.T) {
	executor := &systemdUpgradeExecutor{
		statusPath: filepath.Join(t.TempDir(), "missing.json"),
		logPath:    filepath.Join(t.TempDir(), "missing.log"),
	}

	status, err := executor.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.State != UpgradeStateIdle {
		t.Fatalf("expected idle, got %+v", status)
	}
}

func TestUpgradeHandlerStartsAndReturnsStatus(t *testing.T) {
	executor := &fakeUpgradeExecutor{status: UpgradeStatusResponse{State: UpgradeStateIdle}}
	service := newUpgradeTestService(executor)
	mux := http.NewServeMux()
	RegisterRoutes(mux, service)

	startRecorder := httptest.NewRecorder()
	mux.ServeHTTP(startRecorder, httptest.NewRequest(http.MethodPost, "/api/system/upgrade", nil))
	if startRecorder.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", startRecorder.Code, startRecorder.Body.String())
	}
	var startResponse UpgradeStartResponse
	if err := json.Unmarshal(startRecorder.Body.Bytes(), &startResponse); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if startResponse.State != UpgradeStateStarting || startResponse.RequestedAt == "" {
		t.Fatalf("unexpected start response: %+v", startResponse)
	}

	executor.status = UpgradeStatusResponse{State: UpgradeStateFailed, Output: "npm run build failed"}
	statusRecorder := httptest.NewRecorder()
	mux.ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/api/system/upgrade", nil))
	if statusRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", statusRecorder.Code, statusRecorder.Body.String())
	}
	var statusResponse UpgradeStatusResponse
	if err := json.Unmarshal(statusRecorder.Body.Bytes(), &statusResponse); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if statusResponse.State != UpgradeStateFailed || statusResponse.Output != "npm run build failed" {
		t.Fatalf("unexpected status response: %+v", statusResponse)
	}
}

func TestUpgradeHandlerReturnsConflictWhileRunning(t *testing.T) {
	executor := &fakeUpgradeExecutor{status: UpgradeStatusResponse{State: UpgradeStateRunning}}
	service := newUpgradeTestService(executor)
	mux := http.NewServeMux()
	RegisterRoutes(mux, service)

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/system/upgrade", nil))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestUpgradeHandlerReturnsExecutorFailureReason(t *testing.T) {
	executor := &fakeUpgradeExecutor{
		status:   UpgradeStatusResponse{State: UpgradeStateIdle},
		startErr: errors.New("access denied by systemd"),
	}
	service := newUpgradeTestService(executor)
	mux := http.NewServeMux()
	RegisterRoutes(mux, service)

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/system/upgrade", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Message != "启动系统升级失败：access denied by systemd" {
		t.Fatalf("unexpected failure reason: %q", response.Message)
	}
}
