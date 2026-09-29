package deploy

import (
	"context"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

func managedMongoTestConfig() *config.ProjectConfig {
	cfg := &config.ProjectConfig{Name: "myapp"}
	cfg.MongoDB = config.MongoConfig{Enabled: true, Managed: true}
	return cfg
}

const savedMongoURI = "mongodb://myapp:oldpass@myapp-mongodb:27017/myapp?authSource=admin&replicaSet=rs0"

func mongoCRedentialsPattern() string {
	return "cat /opt/frankendeploy/apps/myapp/shared/.mongo_credentials"
}

func TestDeployMongoDBService_ReusesRunningContainerAndCredentials(t *testing.T) {
	mock := dbMock(map[string]ssh.ExecResult{
		mongoCRedentialsPattern():         {Stdout: savedMongoURI + "\n", ExitCode: 0},
		"docker ps -aq":                   {Stdout: "abc123\n", ExitCode: 0},
		"docker ps -q":                    {Stdout: "abc123\n", ExitCode: 0},
		"rs.status().members[0].stateStr": {Stdout: "PRIMARY\n", ExitCode: 0},
	})

	uri, err := DeployMongoDBService(context.Background(), mock, managedMongoTestConfig(), "/opt/frankendeploy/apps/myapp", nil)
	if err != nil {
		t.Fatalf("DeployMongoDBService: %v", err)
	}
	if uri != savedMongoURI {
		t.Errorf("expected saved URI to be reused, got %q", uri)
	}
	for _, cmd := range mock.Commands {
		if strings.Contains(cmd, "docker run") {
			t.Errorf("must not recreate a running container with saved credentials: %s", cmd)
		}
	}
}

func TestDeployMongoDBService_StartsStoppedContainerAndReusesCredentials(t *testing.T) {
	mock := dbMock(map[string]ssh.ExecResult{
		mongoCRedentialsPattern():         {Stdout: savedMongoURI + "\n", ExitCode: 0},
		"docker ps -aq":                   {Stdout: "abc123\n", ExitCode: 0},
		"docker ps -q":                    {Stdout: "", ExitCode: 0}, // not running
		"rs.status().members[0].stateStr": {Stdout: "PRIMARY\n", ExitCode: 0},
	})

	uri, err := DeployMongoDBService(context.Background(), mock, managedMongoTestConfig(), "/opt/frankendeploy/apps/myapp", nil)
	if err != nil {
		t.Fatalf("DeployMongoDBService: %v", err)
	}
	if uri != savedMongoURI {
		t.Errorf("expected saved URI, got %q", uri)
	}
	var started bool
	for _, cmd := range mock.Commands {
		if strings.Contains(cmd, "docker start myapp-mongodb") {
			started = true
		}
		if strings.Contains(cmd, "docker run") {
			t.Errorf("must docker start, not recreate: %s", cmd)
		}
	}
	if !started {
		t.Error("expected the stopped mongodb container to be started")
	}
}

func TestDeployMongoDBService_VolumeWithoutCredentialsFailsExplicitly(t *testing.T) {
	mock := dbMock(map[string]ssh.ExecResult{
		mongoCRedentialsPattern(): {ExitCode: 1},
		"docker ps -aq":           {Stdout: "", ExitCode: 0},
		"docker volume ls":        {Stdout: "myapp-mongodb-data\n", ExitCode: 0},
	})

	_, err := DeployMongoDBService(context.Background(), mock, managedMongoTestConfig(), "/opt/frankendeploy/apps/myapp", nil)
	if err == nil {
		t.Fatal("a data volume without credentials must fail explicitly (regenerated password would be ignored by the datadir)")
	}
	if !strings.Contains(err.Error(), "docker volume rm") {
		t.Errorf("the error must explain how to recover, got: %v", err)
	}
	for _, cmd := range mock.Commands {
		if strings.Contains(cmd, "docker run") {
			t.Errorf("must not create a container over an orphaned volume: %s", cmd)
		}
	}
}

func TestDeployMongoDBService_FreshSetupCreatesAndInits(t *testing.T) {
	mock := dbMock(map[string]ssh.ExecResult{
		mongoCRedentialsPattern():         {ExitCode: 1},
		"docker ps -aq":                   {Stdout: "", ExitCode: 0},
		"docker volume ls":                {Stdout: "", ExitCode: 0},
		"rs.status().members[0].stateStr": {Stdout: "PRIMARY\n", ExitCode: 0},
	})

	uri, err := DeployMongoDBService(context.Background(), mock, managedMongoTestConfig(), "/opt/frankendeploy/apps/myapp", nil)
	if err != nil {
		t.Fatalf("DeployMongoDBService: %v", err)
	}
	if !strings.HasPrefix(uri, "mongodb://myapp:") {
		t.Errorf("expected a fresh MONGODB_URI, got %q", uri)
	}
	if !strings.Contains(uri, "replicaSet=rs0") {
		t.Errorf("expected replicaSet=rs0 in the URI, got %q", uri)
	}

	var ran, initiated, saved bool
	for _, cmd := range mock.Commands {
		if strings.Contains(cmd, "docker run") && strings.Contains(cmd, "mongodb-community-server") && strings.Contains(cmd, "--replSet rs0") {
			ran = true
		}
		if strings.Contains(cmd, "rs.initiate({_id:\"rs0\", members:[{_id:0, host:\"myapp-mongodb:27017\"}]})") {
			initiated = true
		}
		if strings.Contains(cmd, ".mongo_credentials") && strings.HasPrefix(cmd, "echo ") {
			saved = true
		}
	}
	if !ran {
		t.Error("expected a docker run with the community-server image and --replSet rs0")
	}
	if !initiated {
		t.Error("expected rs.initiate() with an explicit member host")
	}
	if !saved {
		t.Error("expected credentials to be saved (before/around init so a failed init is retried next deploy)")
	}
}

func TestDeployMongoDBService_FailedInitPersistsCredentialsForRetry(t *testing.T) {
	old := mongoAttempts
	mongoAttempts = 2
	defer func() { mongoAttempts = old }()

	// The node accepts auth but never elects a PRIMARY within the (short)
	// retry budget: the deploy errors, yet the credentials are already
	// persisted so the next deploy resumes via the reuse branch.
	mock := dbMock(map[string]ssh.ExecResult{
		mongoCRedentialsPattern(): {ExitCode: 1},
		"docker ps -aq":           {Stdout: "", ExitCode: 0},
		"docker volume ls":        {Stdout: "", ExitCode: 0},
		"rs.initiate":             {ExitCode: 0},
	})

	_, err := DeployMongoDBService(context.Background(), mock, managedMongoTestConfig(), "/opt/frankendeploy/apps/myapp", nil)
	if err == nil {
		t.Fatal("expected a PRIMARY timeout when the set never elects a primary")
	}

	var saved bool
	for _, cmd := range mock.Commands {
		if strings.Contains(cmd, ".mongo_credentials") && strings.HasPrefix(cmd, "echo ") {
			saved = true
		}
	}
	if !saved {
		t.Error("expected credentials to be persisted even when the init failed, so the next deploy retries it")
	}
}
