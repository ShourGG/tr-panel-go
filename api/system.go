package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"terraria-panel/config"
	"terraria-panel/models"
	"terraria-panel/utils"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	cachedCPU             float64
	cachedMemory          float64
	cachedUploadSpeed     uint64
	cachedDownloadSpeed   uint64
	lastUpdateTime        time.Time
	resourceMutex         sync.RWMutex
	cacheExpiration       = 5 * time.Second
	isUpdating            bool
	lastCPUIdle           uint64
	lastCPUTotal          uint64
	lastNetworkRXBytes    uint64
	lastNetworkTXBytes    uint64
	lastNetworkSampleTime time.Time
	panelStartedAt        = time.Now().UTC()
	cachedPublicIP        string
	lastPublicIPLookupAt  time.Time
	publicIPMutex         sync.RWMutex
	publicIPSuccessTTL    = 10 * time.Minute
	publicIPFailureTTL    = 1 * time.Minute
)

const (
	updateChannelStable = "stable"
	updateChannelDev    = "dev"
	githubRepoReleases  = "https://github.com/ShourGG/tr-panel-go/releases"
)

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName    string               `json:"tag_name"`
	Name       string               `json:"name"`
	HTMLURL    string               `json:"html_url"`
	Draft      bool                 `json:"draft"`
	Prerelease bool                 `json:"prerelease"`
	Assets     []githubReleaseAsset `json:"assets"`
}

func getInstalledTModLoaderTerrariaVersion() string {
	version, ok := getInstalledGameVersion("tmodloader")
	if !ok || strings.TrimSpace(version) == "" {
		return ""
	}

	if release, found := resolveTModLoaderReleaseOption(version); found {
		if terrariaVersion := strings.TrimSpace(release.TerrariaVersion); terrariaVersion != "" {
			return terrariaVersion
		}
	}

	return ""
}

func InitSystemMonitoring() {
	updateSystemResources()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			updateSystemResources()
		}
	}()
}

func updateSystemResources() {
	resourceMutex.Lock()
	if isUpdating {
		resourceMutex.Unlock()
		return
	}
	isUpdating = true
	resourceMutex.Unlock()

	defer func() {
		resourceMutex.Lock()
		isUpdating = false
		resourceMutex.Unlock()
	}()

	cpuUsage := calculateCPUUsageIncremental()
	if lastUpdateTime.IsZero() && cpuUsage == 0 {
		time.Sleep(200 * time.Millisecond)
		cpuUsage = calculateCPUUsageIncremental()
	}

	memUsage := calculateMemoryUsage()
	uploadSpeed, downloadSpeed := calculateNetworkSpeeds()

	resourceMutex.Lock()
	cachedCPU = cpuUsage
	cachedMemory = memUsage
	cachedUploadSpeed = uploadSpeed
	cachedDownloadSpeed = downloadSpeed
	lastUpdateTime = time.Now()
	resourceMutex.Unlock()
}

func ensureSystemResourcesFresh() {
	resourceMutex.RLock()
	needsUpdate := lastUpdateTime.IsZero() || time.Since(lastUpdateTime) > cacheExpiration
	resourceMutex.RUnlock()
	if needsUpdate {
		updateSystemResources()
	}
}

func calculateCPUUsageIncremental() float64 {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cachedCPU
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return cachedCPU
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 {
		return cachedCPU
	}

	idle, _ := strconv.ParseUint(fields[4], 10, 64)
	total := uint64(0)
	for i := 1; i < len(fields); i++ {
		val, _ := strconv.ParseUint(fields[i], 10, 64)
		total += val
	}

	if lastCPUTotal == 0 {
		lastCPUIdle = idle
		lastCPUTotal = total
		return 0
	}

	idleDelta := float64(idle - lastCPUIdle)
	totalDelta := float64(total - lastCPUTotal)

	lastCPUIdle = idle
	lastCPUTotal = total

	if totalDelta == 0 {
		return cachedCPU
	}

	usage := (1.0 - idleDelta/totalDelta) * 100.0
	if usage < 0 {
		return 0
	}
	if usage > 100 {
		return 100
	}
	return usage
}

func readMemInfo() (uint64, uint64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	var memTotal uint64
	var memAvailable uint64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}

		switch fields[0] {
		case "MemTotal:":
			memTotal, _ = strconv.ParseUint(fields[1], 10, 64)
		case "MemAvailable:":
			memAvailable, _ = strconv.ParseUint(fields[1], 10, 64)
		}
	}

	return memTotal * 1024, memAvailable * 1024
}

func calculateMemoryUsage() float64 {
	memTotal, memAvailable := readMemInfo()
	if memTotal == 0 {
		return 0
	}

	usage := (1.0 - float64(memAvailable)/float64(memTotal)) * 100.0
	if usage < 0 {
		return 0
	}
	if usage > 100 {
		return 100
	}
	return usage
}

func readNetworkTotals() (uint64, uint64) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	var rxBytes uint64
	var txBytes uint64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}

		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		rxBytes += rx
		txBytes += tx
	}

	return rxBytes, txBytes
}

func calculateNetworkSpeeds() (uint64, uint64) {
	now := time.Now()
	rxBytes, txBytes := readNetworkTotals()

	if lastNetworkSampleTime.IsZero() {
		lastNetworkRXBytes = rxBytes
		lastNetworkTXBytes = txBytes
		lastNetworkSampleTime = now
		return 0, 0
	}

	elapsed := now.Sub(lastNetworkSampleTime).Seconds()
	if elapsed <= 0 {
		return cachedUploadSpeed, cachedDownloadSpeed
	}

	var downloadSpeed uint64
	var uploadSpeed uint64

	if rxBytes >= lastNetworkRXBytes {
		downloadSpeed = uint64(float64(rxBytes-lastNetworkRXBytes) / elapsed)
	}
	if txBytes >= lastNetworkTXBytes {
		uploadSpeed = uint64(float64(txBytes-lastNetworkTXBytes) / elapsed)
	}

	lastNetworkRXBytes = rxBytes
	lastNetworkTXBytes = txBytes
	lastNetworkSampleTime = now

	return uploadSpeed, downloadSpeed
}

func getCPUUsage() float64 {
	ensureSystemResourcesFresh()
	resourceMutex.RLock()
	defer resourceMutex.RUnlock()
	return cachedCPU
}

func getMemoryUsage() float64 {
	ensureSystemResourcesFresh()
	resourceMutex.RLock()
	defer resourceMutex.RUnlock()
	return cachedMemory
}

func getNetworkSpeeds() (uint64, uint64) {
	ensureSystemResourcesFresh()
	resourceMutex.RLock()
	defer resourceMutex.RUnlock()
	return cachedUploadSpeed, cachedDownloadSpeed
}

func getOSInfo() string {
	osInfo := runtime.GOOS
	if runtime.GOOS != "linux" {
		return osInfo
	}

	file, err := os.Open("/etc/os-release")
	if err != nil {
		return osInfo
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		}
	}

	return osInfo
}

func getCPUModel() string {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return runtime.GOARCH
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}

	return runtime.GOARCH
}

func getSystemUptimeSeconds() int {
	file, err := os.Open("/proc/uptime")
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) == 0 {
		return 0
	}

	uptime, _ := strconv.ParseFloat(fields[0], 64)
	return int(uptime)
}

func getHostnameAndIPs() (string, []string) {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "-"
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return hostname, []string{}
	}

	localIPs := []string{}
	seen := make(map[string]struct{})

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}

			ipText := ip.String()
			if _, exists := seen[ipText]; exists {
				continue
			}
			seen[ipText] = struct{}{}
			localIPs = append(localIPs, ipText)
		}
	}

	return hostname, localIPs
}

func getConfiguredPublicIP() string {
	if ip := strings.TrimSpace(os.Getenv("SERVER_IP")); ip != "" {
		return ip
	}
	if ip := getPublicIPFromInterfaces(); ip != "" {
		return ip
	}
	return getPublicIPFromCache()
}

func isRoutablePublicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() || ip.IsUnspecified() {
		return false
	}
	return true
}

func getPublicIPFromInterfaces() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	ipv6Fallback := ""
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}

			ip := ipNet.IP
			if !isRoutablePublicIP(ip) {
				continue
			}

			if ipv4 := ip.To4(); ipv4 != nil {
				return ipv4.String()
			}

			if ipv6Fallback == "" {
				ipv6Fallback = ip.String()
			}
		}
	}

	return ipv6Fallback
}

func getPublicIPFromCache() string {
	publicIPMutex.RLock()
	cached := cachedPublicIP
	lastLookup := lastPublicIPLookupAt
	publicIPMutex.RUnlock()

	age := time.Since(lastLookup)
	if cached != "" && !lastLookup.IsZero() && age < publicIPSuccessTTL {
		return cached
	}
	if cached == "" && !lastLookup.IsZero() && age < publicIPFailureTTL {
		return "-"
	}

	publicIPMutex.Lock()
	defer publicIPMutex.Unlock()

	age = time.Since(lastPublicIPLookupAt)
	if cachedPublicIP != "" && !lastPublicIPLookupAt.IsZero() && age < publicIPSuccessTTL {
		return cachedPublicIP
	}
	if cachedPublicIP == "" && !lastPublicIPLookupAt.IsZero() && age < publicIPFailureTTL {
		return "-"
	}

	if detected := detectPublicIP(); detected != "" {
		cachedPublicIP = detected
		lastPublicIPLookupAt = time.Now()
		return detected
	}

	lastPublicIPLookupAt = time.Now()
	if cachedPublicIP != "" {
		return cachedPublicIP
	}

	return "-"
}

func detectPublicIP() string {
	endpoints := []string{
		"https://api.ipify.org",
		"https://api64.ipify.org",
		"https://ifconfig.me/ip",
		"https://ifconfig.io/ip",
		"https://ip.sb",
		"https://ipinfo.io/ip",
		"https://checkip.amazonaws.com",
		"https://ipv4.icanhazip.com",
	}

	client := &http.Client{Timeout: 2 * time.Second}
	for _, endpoint := range endpoints {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "terraria-panel/1.0")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 128))
		resp.Body.Close()
		if readErr != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		ipText := strings.TrimSpace(string(body))
		if net.ParseIP(ipText) != nil {
			return ipText
		}
	}

	return ""
}

func getDiskInfo() ([]gin.H, float64) {
	cmd := exec.Command("df", "-B1", "-PT")
	output, err := cmd.Output()
	if err != nil {
		return []gin.H{}, 0
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) <= 1 {
		return []gin.H{}, 0
	}

	skipFsTypes := map[string]bool{
		"tmpfs": true, "devtmpfs": true, "squashfs": true,
		"efivarfs": true, "overlay": true, "none": true,
	}

	disks := make([]gin.H, 0, len(lines)-1)
	rootUsage := 0.0

	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}

		fstype := fields[1]
		mount := fields[len(fields)-1]

		if skipFsTypes[fstype] {
			continue
		}
		if mount == "/snap" || strings.HasPrefix(mount, "/snap/") {
			continue
		}
		if mount == "/boot/efi" || mount == "/boot" {
			continue
		}

		total, err1 := strconv.ParseUint(fields[2], 10, 64)
		used, err2 := strconv.ParseUint(fields[3], 10, 64)
		avail, err3 := strconv.ParseUint(fields[4], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil || total == 0 {
			continue
		}

		if total < 1*1024*1024*1024 {
			continue
		}

		usedPercent := float64(used) / float64(total) * 100
		disks = append(disks, gin.H{
			"device":       fields[0],
			"fstype":       fstype,
			"mountpoint":   mount,
			"total":        total,
			"used":         used,
			"free":         avail,
			"usagePercent": usedPercent,
		})

		if mount == "/" {
			rootUsage = usedPercent
		}
	}

	if rootUsage == 0 && len(disks) > 0 {
		if percent, ok := disks[0]["usagePercent"].(float64); ok {
			rootUsage = percent
		}
	}

	return disks, rootUsage
}

func getPanelVersion() string {
	return "1.3.18-dev.3"
}

func normalizeUpdateChannel(channel string) string {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case updateChannelDev:
		return updateChannelDev
	default:
		return updateChannelStable
	}
}

func getConfiguredUpdateChannel() string {
	cfg := config.Load()
	return normalizeUpdateChannel(cfg.UpdateChannel)
}

func buildGitHubReleaseAPIURLs(apiPath string) []string {
	return []string{
		apiPath,
		"https://cors.isteed.cc/" + apiPath,
		"https://gh.noki.icu/" + apiPath,
	}
}

func fetchChannelRelease(channel string) (*githubRelease, error) {
	apiPath := "https://api.github.com/repos/ShourGG/tr-panel-go/releases?per_page=20"
	apiClient := &http.Client{Timeout: 15 * time.Second}

	var releases []githubRelease
	var lastErr error
	for _, apiURL := range buildGitHubReleaseAPIURLs(apiPath) {
		log.Printf("[升级] 尝试获取版本列表: %s", apiURL)
		resp, err := apiClient.Get(apiURL)
		if err != nil {
			lastErr = err
			continue
		}

		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("unexpected status: %d", resp.StatusCode)
				return
			}

			var decoded []githubRelease
			if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
				lastErr = err
				return
			}

			releases = decoded
			lastErr = nil
		}()

		if lastErr == nil {
			break
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("无法连接 GitHub Releases API: %w", lastErr)
	}

	for _, release := range releases {
		if release.Draft {
			continue
		}

		if channel == updateChannelDev {
			if release.Prerelease {
				return &release, nil
			}
			continue
		}

		if !release.Prerelease {
			return &release, nil
		}
	}

	if channel == updateChannelDev {
		return nil, fmt.Errorf("当前没有可用的开发版预发布")
	}

	return nil, fmt.Errorf("当前没有可用的正式版发布")
}

func findReleaseAssetDownloadURL(release *githubRelease, assetName string) string {
	if release == nil {
		return ""
	}

	for _, asset := range release.Assets {
		if asset.Name == assetName {
			return asset.BrowserDownloadURL
		}
	}

	return ""
}

func trimReleaseVersion(tag string) string {
	return strings.TrimPrefix(strings.TrimSpace(tag), "v")
}

func GetUpdateInfo(c *gin.Context) {
	channel := getConfiguredUpdateChannel()
	currentVersion := getPanelVersion()

	response := gin.H{
		"channel":        channel,
		"currentVersion": currentVersion,
		"latestVersion":  currentVersion,
		"latestTag":      "",
		"hasUpdate":      false,
		"updateUrl":      githubRepoReleases,
		"downloadUrl":    "",
	}

	release, err := fetchChannelRelease(channel)
	if err != nil {
		response["error"] = err.Error()
		c.JSON(http.StatusOK, models.SuccessResponse(response))
		return
	}

	latestVersion := trimReleaseVersion(release.TagName)
	response["latestVersion"] = latestVersion
	response["latestTag"] = release.TagName
	response["updateUrl"] = release.HTMLURL
	response["downloadUrl"] = findReleaseAssetDownloadURL(release, "terraria-panel")
	response["hasUpdate"] = latestVersion != currentVersion
	c.JSON(http.StatusOK, models.SuccessResponse(response))
}

// SelfUpgrade handles POST /system/upgrade
func SelfUpgrade(c *gin.Context) {
	currentVersion := getPanelVersion()
	channel := getConfiguredUpdateChannel()
	release, err := fetchChannelRelease(channel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
		return
	}

	latestVersion := trimReleaseVersion(release.TagName)
	if latestVersion == currentVersion {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已是最新版本 " + currentVersion, "upgraded": false})
		return
	}

	// 2. Find binary asset
	downloadURL := findReleaseAssetDownloadURL(release, "terraria-panel")
	if downloadURL == "" {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("未找到可下载的二进制文件"))
		return
	}

	// 3. Get current binary path
	execPath, err := os.Executable()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("无法获取当前程序路径: "+err.Error()))
		return
	}
	execPath, _ = filepath.EvalSymlinks(execPath)
	backupPath := execPath + ".bak"
	tmpPath := execPath + ".new"

	// 4. Download new binary
	cfg := config.Load()
	downloadOpts := utils.GetDownloadConfig(cfg, downloadURL, tmpPath, nil)
	if err := utils.DownloadWithRetry(downloadOpts); err != nil {
		os.Remove(tmpPath)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("下载失败: "+err.Error()))
		return
	}
	tmpInfo, err := os.Stat(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("检查下载文件失败: "+err.Error()))
		return
	}
	if tmpInfo.Size() < 1024*1024 {
		os.Remove(tmpPath)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(fmt.Sprintf("下载不完整 (%d bytes)", tmpInfo.Size())))
		return
	}
	log.Printf("[升级] 下载完成，版本 %s，文件大小 %d bytes", release.TagName, tmpInfo.Size())

	// 5. Backup -> Replace
	os.Remove(backupPath)
	if err := os.Rename(execPath, backupPath); err != nil {
		os.Remove(tmpPath)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("备份当前版本失败: "+err.Error()))
		return
	}
	if err := os.Rename(tmpPath, execPath); err != nil {
		// Rollback
		os.Rename(backupPath, execPath)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("替换二进制失败: "+err.Error()))
		return
	}
	os.Chmod(execPath, 0755)

	log.Printf("[升级] 二进制已替换，版本 %s -> %s，即将重启服务...", currentVersion, latestVersion)

	// 6. Respond first, then restart
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"upgraded":   true,
		"message":    fmt.Sprintf("升级成功 %s → %s，正在重启服务...", currentVersion, latestVersion),
		"oldVersion": currentVersion,
		"newVersion": latestVersion,
	})

	// Schedule restart after response is sent
	go func() {
		time.Sleep(1 * time.Second)
		log.Printf("[升级] 执行 systemctl restart tr-panel ...")
		exec.Command("systemctl", "restart", "tr-panel").Run()
	}()
}

func getServerVersionInfo(serverType string) (string, string) {
	switch serverType {
	case "vanilla":
		if version, ok := getInstalledGameVersion("vanilla"); ok {
			return version, version
		}
		return "-", "-"
	case "tmodloader":
		if version, ok := getInstalledGameVersion("tmodloader"); ok {
			if terrariaVersion := getInstalledTModLoaderTerrariaVersion(); terrariaVersion != "" {
				return version, terrariaVersion
			}
			return version, "由当前 tModLoader 版本决定"
		}
		return "-", "-"
	case "tshock":
		if version, ok := getInstalledGameVersion("tshock6"); ok {
			return version, "1.4.5.6"
		}
		if version, ok := getInstalledGameVersion("tshock5"); ok {
			return version, "1.4.4.9"
		}
		return "-", "-"
	default:
		return "-", "-"
	}
}

func getServerTypeDisplayName(serverType string) string {
	switch serverType {
	case "vanilla":
		return "原版"
	case "tmodloader":
		return "tModLoader"
	case "tshock":
		return "TShock"
	default:
		return serverType
	}
}

func getServerRuntimeInfo() gin.H {
	result := gin.H{
		"hasRunningServer": false,
		"runningRoomCount": 0,
		"serverUptime":     0,
		"serverVersion":    "-",
		"terrariaVersion":  "-",
		"serverType":       "-",
		"gamePort":         0,
		"roomName":         "-",
		"roomId":           0,
		"runningRooms":     []gin.H{},
	}

	if roomStorage == nil {
		return result
	}

	rooms, err := roomStorage.GetAll()
	if err != nil {
		return result
	}

	runningRooms := 0
	var selectedRoomID int
	var selectedRoomName string
	var selectedServerType string
	var selectedGamePort int
	var selectedServerUptime int
	var selectedServerVersion string
	var selectedTerrariaVersion string
	runningRoomItems := make([]gin.H, 0)

	for i := range rooms {
		room := &rooms[i]
		if syncRoomRuntimeState(room) {
			runningRooms++
			serverVersion, terrariaVersion := getServerVersionInfo(room.ServerType)
			uptimeSeconds := 0
			if room.StartTime != nil {
				seconds := int(time.Since(*room.StartTime).Seconds())
				if seconds > 0 {
					uptimeSeconds = seconds
				}
			}

			runningRoomItems = append(runningRoomItems, gin.H{
				"id":               room.ID,
				"name":             room.Name,
				"serverType":       room.ServerType,
				"serverTypeText":   getServerTypeDisplayName(room.ServerType),
				"gamePort":         room.Port,
				"uptime":           uptimeSeconds,
				"serverVersion":    serverVersion,
				"terrariaVersion":  terrariaVersion,
				"configuredWorld":  room.WorldFile,
				"maxPlayers":       room.MaxPlayers,
				"configuredStatus": room.Status,
			})

			if selectedRoomID == 0 {
				selectedRoomID = room.ID
				selectedRoomName = room.Name
				selectedServerType = room.ServerType
				selectedGamePort = room.Port
				selectedServerUptime = uptimeSeconds
				selectedServerVersion = serverVersion
				selectedTerrariaVersion = terrariaVersion
			}
		}
	}

	if runningRooms == 0 {
		return result
	}

	result["hasRunningServer"] = true
	result["runningRoomCount"] = runningRooms
	result["serverUptime"] = selectedServerUptime
	result["serverVersion"] = selectedServerVersion
	result["terrariaVersion"] = selectedTerrariaVersion
	result["serverType"] = selectedServerType
	result["gamePort"] = selectedGamePort
	result["roomName"] = selectedRoomName
	result["roomId"] = selectedRoomID
	result["runningRooms"] = runningRoomItems

	return result
}

func GetSystemInfo(c *gin.Context) {
	cpuUsage := getCPUUsage()
	memUsage := getMemoryUsage()
	memTotal, memAvailable := readMemInfo()
	systemUptime := getSystemUptimeSeconds()
	hostname, localIPs := getHostnameAndIPs()
	uploadSpeed, downloadSpeed := getNetworkSpeeds()
	disks, diskUsage := getDiskInfo()
	serverInfo := getServerRuntimeInfo()
	cfg := config.Load()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"cpu":              cpuUsage,
		"memory":           memUsage,
		"disk":             diskUsage,
		"os":               getOSInfo(),
		"arch":             runtime.GOARCH,
		"cpuModel":         getCPUModel(),
		"cpuCores":         runtime.NumCPU(),
		"totalMemory":      memTotal,
		"freeMemory":       memAvailable,
		"uptime":           systemUptime,
		"systemUptime":     systemUptime,
		"hostname":         hostname,
		"localIPs":         localIPs,
		"publicIP":         getConfiguredPublicIP(),
		"uploadSpeed":      uploadSpeed,
		"downloadSpeed":    downloadSpeed,
		"disks":            disks,
		"panelPort":        cfg.Port,
		"panelVersion":     getPanelVersion(),
		"hasRunningServer": serverInfo["hasRunningServer"],
		"runningRoomCount": serverInfo["runningRoomCount"],
		"serverUptime":     serverInfo["serverUptime"],
		"serverVersion":    serverInfo["serverVersion"],
		"terrariaVersion":  serverInfo["terrariaVersion"],
		"serverType":       serverInfo["serverType"],
		"gamePort":         serverInfo["gamePort"],
		"roomName":         serverInfo["roomName"],
		"roomId":           serverInfo["roomId"],
		"runningRooms":     serverInfo["runningRooms"],
		"goroutine":        runtime.NumGoroutine(),
		"goVersion":        runtime.Version(),
		"goMemory": gin.H{
			"alloc":        float64(m.Alloc) / 1024 / 1024,
			"heapAlloc":    float64(m.HeapAlloc) / 1024 / 1024,
			"totalAlloc":   float64(m.TotalAlloc) / 1024 / 1024,
			"sys":          float64(m.Sys) / 1024 / 1024,
			"numGC":        m.NumGC,
			"numGoroutine": runtime.NumGoroutine(),
		},
	}))
}

func GetCPU(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"usage": getCPUUsage(),
		},
	})
}

func GetMemory(c *gin.Context) {
	memUsage := getMemoryUsage()
	memTotal, memAvailable := readMemInfo()
	usedMemory := uint64(0)
	if memTotal >= memAvailable {
		usedMemory = memTotal - memAvailable
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"usage": memUsage,
			"used":  usedMemory,
			"free":  memAvailable,
			"total": memTotal,
		},
	})
}

func GetSystemInfoDetail(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"cpuCores":  runtime.NumCPU(),
		"goroutine": runtime.NumGoroutine(),
		"memory": gin.H{
			"alloc":      m.Alloc / 1024 / 1024,
			"totalAlloc": m.TotalAlloc / 1024 / 1024,
			"sys":        m.Sys / 1024 / 1024,
		},
	}))
}

func GetPanelStatus(c *gin.Context) {
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"startedAt":     panelStartedAt.Format(time.RFC3339Nano),
		"startedAtUnix": panelStartedAt.Unix(),
		"now":           time.Now().UTC().Format(time.RFC3339Nano),
	}))
}
