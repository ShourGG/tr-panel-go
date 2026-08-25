package api

import "testing"

func TestModProgressStageTracksDownloadLifecycle(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		message  string
		previous string
		want     string
	}{
		{"steam preparation", "downloading", "正在准备 SteamCMD 和 Workshop 组件...", "", "preparing"},
		{"workshop download", "downloading", "正在下载 42%", "preparing", "downloading"},
		{"locate mod", "installing", "正在查找模组文件...", "downloading", "locating"},
		{"install mod", "installing", "正在安装模组文件...", "locating", "installing"},
		{"completed", "completed", "下载完成", "installing", "completed"},
		{"failed", "failed", "下载失败: SteamCMD exited", "downloading", "failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := modProgressStage(test.status, test.message, test.previous); got != test.want {
				t.Fatalf("modProgressStage()=%q, want %q", got, test.want)
			}
		})
	}
}

func TestWorkshopIDValidationAcceptsNumericIDsAndRejectsPathLikeValues(t *testing.T) {
	for _, workshopID := range []string{"2619954303", "123456"} {
		if !isValidWorkshopID(workshopID) {
			t.Fatalf("valid Workshop ID %q was rejected", workshopID)
		}
	}
	for _, workshopID := range []string{"", "12345", "2619954303/../../etc", "https://example.test/item"} {
		if isValidWorkshopID(workshopID) {
			t.Fatalf("invalid Workshop ID %q was accepted", workshopID)
		}
	}
}
