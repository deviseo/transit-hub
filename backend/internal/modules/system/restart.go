package system

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	restartUnitName          = "transithub-restart.service"
	defaultRestartStatusPath = "/var/lib/transithub/restart-status.json"
	defaultRestartLogPath    = "/var/lib/transithub/restart.log"
	maximumRestartLogBytes   = 32 * 1024
)

type systemdRestartExecutor struct {
	statusPath string
	logPath    string
	runCommand commandRunner
}

func newSystemdRestartExecutor() *systemdRestartExecutor {
	return &systemdRestartExecutor{
		statusPath: defaultRestartStatusPath,
		logPath:    defaultRestartLogPath,
		runCommand: runCombinedOutput,
	}
}

func (e *systemdRestartExecutor) Start(ctx context.Context) error {
	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := e.prepareStart(startedAt); err != nil {
		return fmt.Errorf("准备后台服务重启状态失败：%w", err)
	}

	runner := e.runCommand
	if runner == nil {
		runner = runCombinedOutput
	}
	output, err := runner(ctx, "systemctl", "start", "--no-block", restartUnitName)
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(output))
	startErr := err
	if detail != "" {
		startErr = fmt.Errorf("%w：%s", err, detail)
	}
	if recordErr := e.recordStartFailure(startedAt, startErr); recordErr != nil {
		return fmt.Errorf("%w（记录失败状态时出错：%v）", startErr, recordErr)
	}
	return startErr
}

func (e *systemdRestartExecutor) prepareStart(startedAt string) error {
	statusPath, logPath := e.paths()
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		return err
	}
	if err := os.Chmod(logPath, 0o644); err != nil {
		return err
	}
	return writeRestartStatus(statusPath, RestartStatusResponse{
		State:     RestartStateStarting,
		StartedAt: startedAt,
	})
}

func (e *systemdRestartExecutor) recordStartFailure(startedAt string, startErr error) error {
	statusPath, logPath := e.paths()
	message := fmt.Sprintf("启动后台服务重启单元失败：%v\n", startErr)
	if err := appendRestartLog(logPath, message); err != nil {
		return err
	}
	exitCode := 1
	return writeRestartStatus(statusPath, RestartStatusResponse{
		State:      RestartStateFailed,
		StartedAt:  startedAt,
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
		ExitCode:   &exitCode,
	})
}

func (e *systemdRestartExecutor) paths() (string, string) {
	statusPath := e.statusPath
	if statusPath == "" {
		statusPath = defaultRestartStatusPath
	}
	logPath := e.logPath
	if logPath == "" {
		logPath = defaultRestartLogPath
	}
	return statusPath, logPath
}

func writeRestartStatus(statusPath string, status RestartStatusResponse) error {
	payload, err := json.Marshal(status)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	nextPath := statusPath + ".next"
	if err := os.WriteFile(nextPath, payload, 0o644); err != nil {
		return err
	}
	if err := os.Chmod(nextPath, 0o644); err != nil {
		_ = os.Remove(nextPath)
		return err
	}
	if err := os.Rename(nextPath, statusPath); err != nil {
		_ = os.Remove(nextPath)
		return err
	}
	return nil
}

func appendRestartLog(logPath, message string) error {
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(message)
	return err
}

func (e *systemdRestartExecutor) Status(ctx context.Context) (RestartStatusResponse, error) {
	if err := ctx.Err(); err != nil {
		return RestartStatusResponse{}, err
	}
	statusPath, logPath := e.paths()
	payload, err := os.ReadFile(statusPath)
	if errorsIsNotExist(err) {
		return RestartStatusResponse{State: RestartStateIdle}, nil
	}
	if err != nil {
		return RestartStatusResponse{}, err
	}

	var status RestartStatusResponse
	if err := json.Unmarshal(payload, &status); err != nil {
		return RestartStatusResponse{}, fmt.Errorf("解析重启状态失败：%w", err)
	}
	if !validRestartState(status.State) {
		return RestartStatusResponse{}, fmt.Errorf("未知的重启状态：%s", status.State)
	}
	if status.State == RestartStateStarting {
		status, err = e.reconcileStartingStatus(ctx, status)
		if err != nil {
			return RestartStatusResponse{}, err
		}
	}
	if status.State == RestartStateFailed {
		output, logErr := readFileTail(logPath, maximumRestartLogBytes)
		if logErr != nil {
			status.Output = fmt.Sprintf("读取重启日志失败：%v", logErr)
		} else {
			status.Output = string(output)
		}
	}
	return status, nil
}

func (e *systemdRestartExecutor) reconcileStartingStatus(ctx context.Context, status RestartStatusResponse) (RestartStatusResponse, error) {
	runner := e.runCommand
	if runner == nil {
		runner = runCombinedOutput
	}
	output, err := runner(
		ctx,
		"systemctl",
		"show",
		restartUnitName,
		"--property=ActiveState",
		"--property=Result",
		"--property=ExecMainStatus",
		"--property=Job",
		"--no-pager",
	)
	if err != nil {
		return RestartStatusResponse{}, fmt.Errorf("读取后台服务重启单元状态失败：%w", err)
	}

	properties := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		key, value, found := strings.Cut(line, "=")
		if found {
			properties[key] = value
		}
	}
	if properties["Job"] != "" {
		return status, nil
	}
	activeState := properties["ActiveState"]
	if activeState != "failed" && activeState != "inactive" {
		return status, nil
	}

	statusPath, logPath := e.paths()
	latestStatus, err := readRestartStatus(statusPath)
	if err != nil {
		return RestartStatusResponse{}, err
	}
	if latestStatus.State != RestartStateStarting || latestStatus.StartedAt != status.StartedAt {
		return latestStatus, nil
	}
	status = latestStatus

	exitCode := 1
	if parsed, parseErr := strconv.Atoi(properties["ExecMainStatus"]); parseErr == nil && parsed != 0 {
		exitCode = parsed
	}
	status.State = RestartStateFailed
	status.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	status.ExitCode = &exitCode
	reason := "后台服务重启单元异步启动失败"
	if activeState == "inactive" {
		reason = "后台服务重启单元已结束但未写入最终状态"
	}
	message := fmt.Sprintf("%s：\n%s", reason, strings.TrimSpace(string(output)))
	if err := appendRestartLog(logPath, message+"\n"); err != nil {
		return RestartStatusResponse{}, err
	}
	if err := writeRestartStatus(statusPath, status); err != nil {
		return RestartStatusResponse{}, err
	}
	return status, nil
}

func readRestartStatus(statusPath string) (RestartStatusResponse, error) {
	payload, err := os.ReadFile(statusPath)
	if err != nil {
		return RestartStatusResponse{}, err
	}
	var status RestartStatusResponse
	if err := json.Unmarshal(payload, &status); err != nil {
		return RestartStatusResponse{}, fmt.Errorf("解析重启状态失败：%w", err)
	}
	if !validRestartState(status.State) {
		return RestartStatusResponse{}, fmt.Errorf("未知的重启状态：%s", status.State)
	}
	return status, nil
}

func validRestartState(state RestartState) bool {
	switch state {
	case RestartStateIdle, RestartStateStarting, RestartStateRunning, RestartStateSucceeded, RestartStateFailed:
		return true
	default:
		return false
	}
}
