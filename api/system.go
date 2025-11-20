package api
import (
	"bufio"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"terraria-panel/models"
	"time"
	"github.com/gin-gonic/gin"
)
var (
	cachedCPU       float64
	cachedMemory    float64
	lastUpdateTime  time.Time
	resourceMutex   sync.RWMutex
	cacheExpiration = 5 * time.Second
	isUpdating      bool
	lastCPUIdle  uint64
	lastCPUTotal uint64
)
func InitSystemMonitoring() {
	calculateCPUUsageIncremental()
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
	memUsage := calculateMemoryUsage()
	resourceMutex.Lock()
	cachedCPU = cpuUsage
	cachedMemory = memUsage
	lastUpdateTime = time.Now()
	resourceMutex.Unlock()
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
	line := scanner.Text()
	fields := strings.Fields(line)
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
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}
	return usage
}
func calculateCPUUsage() float64 {
	file1, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	scanner1 := bufio.NewScanner(file1)
	if !scanner1.Scan() {
		file1.Close()
		return 0
	}
	line1 := scanner1.Text()
	file1.Close()
	fields1 := strings.Fields(line1)
	if len(fields1) < 5 {
		return 0
	}
	idle1, _ := strconv.ParseUint(fields1[4], 10, 64)
	total1 := uint64(0)
	for i := 1; i < len(fields1); i++ {
		val, _ := strconv.ParseUint(fields1[i], 10, 64)
		total1 += val
	}
	time.Sleep(1 * time.Second)
	file2, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	scanner2 := bufio.NewScanner(file2)
	if !scanner2.Scan() {
		file2.Close()
		return 0
	}
	line2 := scanner2.Text()
	file2.Close()
	fields2 := strings.Fields(line2)
	if len(fields2) < 5 {
		return 0
	}
	idle2, _ := strconv.ParseUint(fields2[4], 10, 64)
	total2 := uint64(0)
	for i := 1; i < len(fields2); i++ {
		val, _ := strconv.ParseUint(fields2[i], 10, 64)
		total2 += val
	}
	idleDelta := float64(idle2 - idle1)
	totalDelta := float64(total2 - total1)
	if totalDelta == 0 {
		return 0
	}
	usage := (1.0 - idleDelta/totalDelta) * 100.0
	return usage
}
func getCPUUsage() float64 {
	resourceMutex.RLock()
	defer resourceMutex.RUnlock()
	if time.Since(lastUpdateTime) > cacheExpiration {
		go updateSystemResources()
	}
	return cachedCPU
}
func calculateMemoryUsage() float64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer file.Close()
	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
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
	if memTotal == 0 {
		return 0
	}
	usage := (1.0 - float64(memAvailable)/float64(memTotal)) * 100.0
	return usage
}
func getMemoryUsage() float64 {
	resourceMutex.RLock()
	defer resourceMutex.RUnlock()
	if time.Since(lastUpdateTime) > cacheExpiration {
		go updateSystemResources()
	}
	return cachedMemory
}
func getOSInfo() string {
	osInfo := runtime.GOOS
	if runtime.GOOS == "linux" {
		file, err := os.Open("/etc/os-release")
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "PRETTY_NAME=") {
					osInfo = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
					break
				}
			}
		}
	}
	return osInfo
}
func GetSystemInfo(c *gin.Context) {
	cpuUsage := getCPUUsage()
	memUsage := getMemoryUsage()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	var totalMemory uint64
	file, err := os.Open("/proc/meminfo")
	if err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "MemTotal:" {
				totalMemory, _ = strconv.ParseUint(fields[1], 10, 64)
				break
			}
		}
		file.Close()
	}
	var uptime float64
	uptimeFile, err := os.Open("/proc/uptime")
	if err == nil {
		scanner := bufio.NewScanner(uptimeFile)
		if scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) > 0 {
				uptime, _ = strconv.ParseFloat(fields[0], 64)
			}
		}
		uptimeFile.Close()
	}
	osInfo := getOSInfo()
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"cpu":         cpuUsage,
		"memory":      memUsage,
		"os":          osInfo,
		"cpuCores":    runtime.NumCPU(),
		"totalMemory": totalMemory * 1024,
		"uptime":      int(uptime),
		"goroutine":   runtime.NumGoroutine(),
		"goMemory": gin.H{
			"alloc":      m.Alloc / 1024 / 1024,
			"totalAlloc": m.TotalAlloc / 1024 / 1024,
			"sys":        m.Sys / 1024 / 1024,
		},
	}))
}
func GetCPU(c *gin.Context) {
	cpuUsage := getCPUUsage()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"usage": cpuUsage,
		},
	})
}
func GetMemory(c *gin.Context) {
	memUsage := getMemoryUsage()
	var totalMemory uint64
	file, err := os.Open("/proc/meminfo")
	if err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "MemTotal:" {
				totalMemory, _ = strconv.ParseUint(fields[1], 10, 64)
				break
			}
		}
		file.Close()
	}
	usedMemory := uint64(float64(totalMemory) * memUsage / 100.0)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"usage": memUsage,
			"used":  usedMemory * 1024,
			"total": totalMemory * 1024,
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
