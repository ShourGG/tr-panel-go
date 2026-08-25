package api

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateStagedTShockInstallationRequiresExpectedVersion(t *testing.T) {
	stageDir := t.TempDir()
	writeTShockCoreFiles(t, stageDir)
	if err := os.WriteFile(filepath.Join(stageDir, ".tshock_version"), []byte("5.2.4"), 0644); err != nil {
		t.Fatalf("write version marker: %v", err)
	}

	if runtime.GOOS != "linux" {
		if err := validateStagedTShockInstallation(stageDir, "5"); err != nil {
			t.Fatalf("validate matching staged TShock: %v", err)
		}
	}
	if err := validateStagedTShockInstallation(stageDir, "6"); err == nil {
		t.Fatal("expected staged TShock validation to reject a mismatched major version")
	}
}

func writeTShockCoreFiles(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "ServerPlugins"), 0755); err != nil {
		t.Fatalf("create TShock plugin directory: %v", err)
	}
	for relativePath, contents := range map[string][]byte{
		"TShock.Server":                     []byte("server"),
		"ServerPlugins/TShockAPI.dll":       []byte("tshock api"),
		"ServerPlugins/TShockAPI.deps.json": []byte(`{"runtimeTarget":{}}`),
	} {
		if err := os.WriteFile(filepath.Join(dir, relativePath), contents, 0755); err != nil {
			t.Fatalf("write TShock core file %s: %v", relativePath, err)
		}
	}
}

func TestValidateStagedTShockInstallationAcceptsBothOfficialCoreLayouts(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("staged validation also checks the host .NET runtime on Linux")
	}
	for _, testCase := range []struct {
		major  string
		marker string
	}{
		{major: "5", marker: "5.2.4"},
		{major: "6", marker: "6.1.0"},
	} {
		t.Run("TShock"+testCase.major, func(t *testing.T) {
			stageDir := t.TempDir()
			writeTShockCoreFiles(t, stageDir)
			if err := os.WriteFile(filepath.Join(stageDir, ".tshock_version"), []byte(testCase.marker), 0644); err != nil {
				t.Fatalf("write version marker: %v", err)
			}
			if err := validateStagedTShockInstallation(stageDir, testCase.major); err != nil {
				t.Fatalf("validate TShock %s stage: %v", testCase.major, err)
			}
		})
	}
}

func TestCopyMissingTShockCoreFilesPreservesRoomDataAndPlugins(t *testing.T) {
	root := t.TempDir()
	sharedDir := filepath.Join(root, "shared")
	roomDir := filepath.Join(root, "room")
	writeTShockCoreFiles(t, sharedDir)
	if err := os.MkdirAll(filepath.Join(roomDir, "ServerPlugins"), 0755); err != nil {
		t.Fatalf("create room plugin directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roomDir, "TShock.Server"), []byte("room server"), 0755); err != nil {
		t.Fatalf("write room executable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roomDir, "ServerPlugins", "UserPlugin.dll"), []byte("user plugin"), 0644); err != nil {
		t.Fatalf("write user plugin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roomDir, "tshock.sqlite"), []byte("room database"), 0644); err != nil {
		t.Fatalf("write room database: %v", err)
	}

	if err := copyMissingTShockCoreFiles(sharedDir, roomDir); err != nil {
		t.Fatalf("copy missing TShock core files: %v", err)
	}
	for _, relativePath := range tshockRequiredCoreFiles {
		if !isUsableTShockFile(filepath.Join(roomDir, relativePath)) {
			t.Fatalf("room core file was not repaired: %s", relativePath)
		}
	}
	for relativePath, want := range map[string]string{
		"TShock.Server":                "room server",
		"ServerPlugins/UserPlugin.dll": "user plugin",
		"tshock.sqlite":                "room database",
	} {
		contents, err := os.ReadFile(filepath.Join(roomDir, relativePath))
		if err != nil || string(contents) != want {
			t.Fatalf("room data %s changed: contents=%q err=%v", relativePath, string(contents), err)
		}
	}
}

func TestReplaceTShockInstallationDirectoryReplacesOnlyAfterPreparedStage(t *testing.T) {
	root := t.TempDir()
	targetDir := filepath.Join(root, "tshock")
	stageDir := filepath.Join(root, "stage")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := os.MkdirAll(stageDir, 0755); err != nil {
		t.Fatalf("create stage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "old-core"), []byte("old"), 0644); err != nil {
		t.Fatalf("write old file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stageDir, "TShock.Server"), []byte("new"), 0755); err != nil {
		t.Fatalf("write staged file: %v", err)
	}
	if err := replaceTShockInstallationDirectory(stageDir, targetDir); err != nil {
		t.Fatalf("replace TShock directory: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(targetDir, "TShock.Server")); err != nil || string(got) != "new" {
		t.Fatalf("replacement core = %q, err=%v", string(got), err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "old-core")); !os.IsNotExist(err) {
		t.Fatalf("old program file survived replacement, stat err=%v", err)
	}
	if _, err := os.Stat(stageDir); !os.IsNotExist(err) {
		t.Fatalf("stage directory should have been renamed away, stat err=%v", err)
	}
}

func TestCopyTShockUserDataPreservesPluginsAndDatabaseOnly(t *testing.T) {
	root := t.TempDir()
	existingDir := filepath.Join(root, "existing")
	stageDir := filepath.Join(root, "stage")
	for _, dir := range []string{
		filepath.Join(existingDir, "ServerPlugins"),
		filepath.Join(existingDir, "tshock"),
		stageDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("create directory %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(existingDir, "ServerPlugins", "Useful.dll"), []byte("plugin"), 0644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, "tshock", "tshock.sqlite"), []byte("database"), 0644); err != nil {
		t.Fatalf("write database: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, "TShock.Server"), []byte("old program"), 0755); err != nil {
		t.Fatalf("write old program: %v", err)
	}
	if err := copyTShockUserData(existingDir, stageDir); err != nil {
		t.Fatalf("copy user data: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stageDir, "ServerPlugins", "Useful.dll")); err != nil {
		t.Fatalf("plugin was not preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stageDir, "tshock", "tshock.sqlite")); err != nil {
		t.Fatalf("database was not preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stageDir, "TShock.Server")); !os.IsNotExist(err) {
		t.Fatalf("old program must not be copied into stage, stat err=%v", err)
	}
}
