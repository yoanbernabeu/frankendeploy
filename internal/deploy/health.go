package deploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// HealthChecker performs health checks on deployed applications
type HealthChecker struct {
	client      ssh.Executor
	containerID string
	path        string
	port        string
	timeout     time.Duration
	retries     int
	interval    time.Duration
}

// NewHealthChecker creates a new health checker with sensible defaults from constants.
func NewHealthChecker(client ssh.Executor, containerID, path, port string) *HealthChecker {
	return &HealthChecker{
		client:      client,
		containerID: containerID,
		path:        path,
		port:        port,
		timeout:     constants.HealthCheckTimeout,
		retries:     constants.HealthCheckRetries,
		interval:    constants.HealthCheckInterval,
	}
}

// SetTimeout sets the overall timeout
func (h *HealthChecker) SetTimeout(timeout time.Duration) {
	h.timeout = timeout
}

// SetRetries sets the number of retries
func (h *HealthChecker) SetRetries(retries int) {
	h.retries = retries
}

// SetInterval sets the interval between retries
func (h *HealthChecker) SetInterval(interval time.Duration) {
	h.interval = interval
}

// HealthResult contains the result of a health check
type HealthResult struct {
	Healthy      bool
	StatusCode   int
	Message      string
	ResponseTime time.Duration
	Attempts     int
}

// Check performs the health check with retries
func (h *HealthChecker) Check(ctx context.Context) (*HealthResult, error) {
	result := &HealthResult{
		Healthy: false,
	}

	deadline := time.Now().Add(h.timeout)

	for attempt := 1; attempt <= h.retries; attempt++ {
		result.Attempts = attempt

		if time.Now().After(deadline) {
			result.Message = "health check timeout"
			return result, nil
		}

		// Check if container is running. A transient SSH error counts as a
		// failed attempt (the connection may recover), never as a panic.
		containerCheck, err := h.client.Exec(ctx, fmt.Sprintf(
			"docker inspect %s --format '{{.State.Status}}'", h.containerID))
		if err != nil || containerCheck == nil {
			result.Message = fmt.Sprintf("container status check failed: %v", err)
			if err := h.sleepBetweenAttempts(ctx, attempt, result); err != nil {
				return result, err
			}
			continue
		}

		containerStatus := strings.TrimSpace(containerCheck.Stdout)
		if containerStatus != "running" {
			result.Message = fmt.Sprintf("container not running (status: %s)", containerStatus)
			if err := h.sleepBetweenAttempts(ctx, attempt, result); err != nil {
				return result, err
			}
			continue
		}

		// Check HTTP endpoint
		start := time.Now()
		healthCmd := fmt.Sprintf(
			"docker exec %s curl -sf -w '%%{http_code}' -o /dev/null http://localhost:%s%s",
			h.containerID, h.port, h.path)

		httpCheck, err := h.client.Exec(ctx, healthCmd)
		result.ResponseTime = time.Since(start)

		if err == nil && httpCheck != nil && httpCheck.ExitCode == 0 {
			result.Healthy = true
			result.StatusCode = 200 // default if parsing fails
			if httpCheck.Stdout != "" {
				var code int
				if n, scanErr := fmt.Sscanf(strings.TrimSpace(httpCheck.Stdout), "%d", &code); n == 1 && scanErr == nil {
					result.StatusCode = code
				}
			}
			result.Message = "healthy"
			return result, nil
		}

		// Parse status code from output
		result.StatusCode = 0
		if httpCheck != nil && httpCheck.Stdout != "" {
			if n, scanErr := fmt.Sscanf(httpCheck.Stdout, "%d", &result.StatusCode); n == 0 || scanErr != nil {
				result.StatusCode = 0
			}
		}

		result.Message = fmt.Sprintf("HTTP check failed (status: %d)", result.StatusCode)
		if err := h.sleepBetweenAttempts(ctx, attempt, result); err != nil {
			return result, err
		}
	}

	return result, nil
}

// sleepBetweenAttempts waits for the retry interval, honoring context
// cancellation, and skips the pointless sleep after the last attempt.
func (h *HealthChecker) sleepBetweenAttempts(ctx context.Context, attempt int, result *HealthResult) error {
	if attempt >= h.retries {
		return nil
	}
	select {
	case <-ctx.Done():
		result.Message = "health check cancelled"
		return ctx.Err()
	case <-time.After(h.interval):
		return nil
	}
}

// ContainerLogs retrieves recent container logs (stdout and stderr merged)
func ContainerLogs(ctx context.Context, client ssh.Executor, containerID string, lines int) (string, error) {
	result, err := client.Exec(ctx, fmt.Sprintf(
		"docker logs %s --tail %d 2>&1", containerID, lines))

	if err != nil {
		return "", err
	}
	if result == nil {
		return "", fmt.Errorf("no output from docker logs")
	}

	return result.Stdout, nil
}
