package utils
import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"github.com/creack/pty"
)
type Process struct {
	cmd        *exec.Cmd
	pid        int
	stdin      io.WriteCloser
	ptyFile    *os.File
	usePTY     bool
	serverType string
	roomID     int
	mu         sync.Mutex
	logWriter  io.Writer
	outputBuffer []string
	bufferMu     sync.RWMutex
	maxBufferLines int
}
var (
	processes = make(map[int]*Process)
	processMu sync.RWMutex
)
func StartProcess(roomID int, command string, args []string, workDir string, envVars map[string]string, logWriter io.Writer, serverType string) (*Process, error) {
	processMu.Lock()
	defer processMu.Unlock()
	if p, exists := processes[roomID]; exists {
		if p.IsRunning() {
			return nil, fmt.Errorf("进程已在运行中")
		}
	}
	cmd := exec.Command(command, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	if len(envVars) > 0 {
		cmd.Env = os.Environ()
		for k, v := range envVars {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 stdin 管道失败: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, err
	}
	p := &Process{
		cmd:        cmd,
		pid:        cmd.Process.Pid,
		stdin:      stdin,
		usePTY:     false,
		serverType: serverType,
		roomID:     roomID,
		logWriter:  logWriter,
	}
	go p.readOutput(stdout, "STDOUT")
	go p.readOutput(stderr, "STDERR")
	processes[roomID] = p
	return p, nil
}
func StartProcessWithPTY(roomID int, command string, args []string, workDir string, envVars map[string]string, logWriter io.Writer, serverType string) (*Process, error) {
	processMu.Lock()
	defer processMu.Unlock()
	if p, exists := processes[roomID]; exists {
		if p.IsRunning() {
			return nil, fmt.Errorf("进程已在运行中")
		}
	}
	cmd := exec.Command(command, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	if len(envVars) > 0 {
		cmd.Env = os.Environ()
		for k, v := range envVars {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	ptyFile, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("创建 PTY 失败: %v", err)
	}
	p := &Process{
		cmd:            cmd,
		pid:            cmd.Process.Pid,
		ptyFile:        ptyFile,
		usePTY:         true,
		serverType:     serverType,
		roomID:         roomID,
		logWriter:      logWriter,
		outputBuffer:   make([]string, 0, 1000),
		maxBufferLines: 1000,
	}
	go p.readPTYOutput()
	processes[roomID] = p
	log.Printf("[INFO] 进程启动成功 (PTY模式) - PID: %d, Room: %d, Type: %s", p.pid, roomID, serverType)
	return p, nil
}
func (p *Process) readPTYOutput() {
	buf := make([]byte, 1024)
	for {
		n, err := p.ptyFile.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("[ERROR] PTY 读取错误: %v", err)
			}
			break
		}
		if n > 0 {
			output := string(buf[:n])
			p.addToBuffer(output)
			if p.logWriter != nil {
				fmt.Fprint(p.logWriter, output)
				if f, ok := p.logWriter.(*os.File); ok {
					f.Sync()
				}
			}
			if p.roomID == 0 {
				filtered := filterANSIEscapeSequences(output)
				if filtered != "" {
					BroadcastPluginServerLog(filtered)
				}
			}
			if p.serverType == "tshock" && strings.Contains(output, "/setup") {
				p.captureAdminToken(output)
			}
		}
	}
}
var BroadcastPluginServerLog func(string) = func(msg string) {
}
func (p *Process) addToBuffer(output string) {
	p.bufferMu.Lock()
	defer p.bufferMu.Unlock()
	filtered := filterANSIEscapeSequences(output)
	if filtered == "" {
		return
	}
	p.outputBuffer = append(p.outputBuffer, filtered)
	if len(p.outputBuffer) > p.maxBufferLines {
		p.outputBuffer = p.outputBuffer[len(p.outputBuffer)-p.maxBufferLines:]
	}
}
func filterANSIEscapeSequences(output string) string {
	output = regexp.MustCompile(`\x1b\][^\x07\x1b\n]*(\x07|\x1b\\)?`).ReplaceAllString(output, "")
	output = regexp.MustCompile(`\x1b\[[0-9;]*[HJKsu]`).ReplaceAllString(output, "")
	output = regexp.MustCompile(`\x1b\[[0-9;]*[ABCDEFGSTf]`).ReplaceAllString(output, "")
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "")
	return output
}
func (p *Process) GetOutputBuffer() string {
	p.bufferMu.RLock()
	defer p.bufferMu.RUnlock()
	return strings.Join(p.outputBuffer, "")
}
func (p *Process) readOutput(reader io.Reader, prefix string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if p.logWriter != nil {
			fmt.Fprintf(p.logWriter, "[%s] %s\n", prefix, line)
		}
		if p.serverType == "tshock" && strings.Contains(line, "/setup") {
			p.captureAdminToken(line)
		}
	}
}
func (p *Process) IsRunning() bool {
	if p.cmd == nil || p.cmd.Process == nil {
		return false
	}
	err := p.cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}
func (p *Process) GetPID() int {
	return p.pid
}
func (p *Process) SendCommand(command string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.usePTY {
		if p.ptyFile == nil {
			return fmt.Errorf("PTY is not available")
		}
		if !strings.HasSuffix(command, "\n") {
			command += "\n"
		}
		_, err := p.ptyFile.Write([]byte(command))
		if err != nil {
			log.Printf("[ERROR] PTY 写入失败: %v", err)
			return fmt.Errorf("PTY 写入失败: %v", err)
		}
		log.Printf("[INFO] 命令已通过 PTY 发送: %s", strings.TrimSpace(command))
		return nil
	}
	if p.stdin == nil {
		return fmt.Errorf("stdin is not available")
	}
	_, err := p.stdin.Write([]byte(command))
	return err
}
func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return fmt.Errorf("进程未运行")
	}
	pid := p.cmd.Process.Pid
	fmt.Printf("[INFO] ========== 开始优雅关闭进程 ==========\n")
	fmt.Printf("[INFO] 进程 PID: %d\n", pid)
	fmt.Printf("[INFO] 服务器类型: %s\n", p.serverType)
	if p.serverType == "tshock" {
		fmt.Println("[INFO] TShock 服务器使用 SIGTERM 信号优雅关闭")
		fmt.Println("[INFO] TShock 会自动保存世界并优雅退出")
		if p.usePTY && p.ptyFile != nil {
			p.ptyFile.Close()
		} else if p.stdin != nil {
			p.stdin.Close()
		}
		if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			fmt.Printf("[ERROR] 发送 SIGTERM 信号失败: %v\n", err)
			fmt.Printf("[WARN] 尝试强制关闭进程...\n")
			return p.cmd.Process.Kill()
		}
		fmt.Println("[INFO] SIGTERM 信号已发送，等待服务器优雅退出（10秒）...")
		done := make(chan error, 1)
		go func() {
			done <- p.cmd.Wait()
		}()
		select {
		case waitErr := <-done:
			if waitErr != nil {
				fmt.Printf("[WARN] Wait() 返回错误: %v\n", waitErr)
			}
			for i := 0; i < 3; i++ {
				if err := p.cmd.Process.Signal(syscall.Signal(0)); err != nil {
					fmt.Printf("[SUCCESS] ✅ TShock 服务器已优雅退出 (PID: %d)\n", pid)
					return nil
				}
				if i < 2 {
					fmt.Printf("[WARN] 进程 %d 仍在运行，等待 1 秒后重试... (%d/3)\n", pid, i+1)
					time.Sleep(1 * time.Second)
				}
			}
			fmt.Printf("[ERROR] ❌ 进程 %d 仍在运行，Wait() 返回但进程未退出！\n", pid)
			fmt.Printf("[WARN] 强制终止进程 (SIGKILL)...\n")
			if err := p.cmd.Process.Kill(); err != nil {
				fmt.Printf("[ERROR] SIGKILL 失败: %v\n", err)
				return err
			}
			time.Sleep(1 * time.Second)
			if err := p.cmd.Process.Signal(syscall.Signal(0)); err == nil {
				return fmt.Errorf("进程 %d 无法终止", pid)
			}
			fmt.Printf("[SUCCESS] ✅ 进程 %d 已强制终止\n", pid)
			return nil
		case <-time.After(10 * time.Second):
			if err := p.cmd.Process.Signal(syscall.Signal(0)); err == nil {
				fmt.Printf("[WARN] 进程 %d 未在10秒内退出，强制关闭 (SIGKILL)\n", pid)
				if err := p.cmd.Process.Kill(); err != nil {
					fmt.Printf("[ERROR] SIGKILL 失败: %v\n", err)
					return err
				}
				time.Sleep(1 * time.Second)
				if err := p.cmd.Process.Signal(syscall.Signal(0)); err == nil {
					return fmt.Errorf("进程 %d 无法终止", pid)
				}
				fmt.Printf("[SUCCESS] ✅ 进程 %d 已强制终止\n", pid)
				return nil
			}
			fmt.Printf("[INFO] 进程 %d 已退出\n", pid)
			return nil
		}
	}
	if p.stdin != nil {
		fmt.Printf("[INFO] stdin 管道状态: 可用\n")
		fmt.Println("[INFO] ========== 第1步：发送 save 命令 ==========")
		saveCmd := "save\n"
		n, err := p.stdin.Write([]byte(saveCmd))
		if err != nil {
			fmt.Printf("[ERROR] 发送 save 命令失败: %v\n", err)
			fmt.Printf("[ERROR] stdin 管道可能已关闭或损坏\n")
		} else {
			fmt.Printf("[SUCCESS] save 命令已发送 (写入 %d 字节)\n", n)
			fmt.Println("[INFO] 等待 2 秒，让服务器保存世界...")
			time.Sleep(2 * time.Second)
			fmt.Println("[INFO] 等待完成")
		}
		exitCommand := "exit"
		fmt.Printf("[INFO] ========== 第2步：发送 %s 命令 ==========\n", exitCommand)
		exitCmd := exitCommand + "\n"
		n, err = p.stdin.Write([]byte(exitCmd))
		if err != nil {
			fmt.Printf("[ERROR] 发送 %s 命令失败: %v\n", exitCommand, err)
			fmt.Printf("[ERROR] stdin 管道可能已关闭或损坏\n")
		} else {
			fmt.Printf("[SUCCESS] %s 命令已发送 (写入 %d 字节)\n", exitCommand, n)
		}
		fmt.Println("[INFO] ========== 第3步：关闭 stdin 管道 ==========")
		if err := p.stdin.Close(); err != nil {
			fmt.Printf("[WARN] 关闭 stdin 管道失败: %v\n", err)
		} else {
			fmt.Println("[SUCCESS] stdin 管道已关闭")
		}
		fmt.Println("[INFO] 等待服务器优雅退出（5秒）...")
		done := make(chan error, 1)
		go func() {
			done <- p.cmd.Wait()
		}()
		select {
		case <-done:
			fmt.Printf("[INFO] 进程 %d 已通过控制台命令正常退出\n", pid)
			return nil
		case <-time.After(5 * time.Second):
			if err := p.cmd.Process.Signal(syscall.Signal(0)); err == nil {
				fmt.Printf("[WARN] 进程 %d 未响应 exit 命令，强制关闭 (SIGKILL)\n", pid)
				return p.cmd.Process.Kill()
			}
			fmt.Printf("[INFO] 进程 %d 已退出\n", pid)
			return nil
		}
	}
	fmt.Printf("[WARN] 没有 stdin 管道，使用信号关闭进程 %d (SIGTERM)...\n", pid)
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		fmt.Printf("[WARN] SIGTERM失败，强制杀死进程: %v\n", err)
		return p.cmd.Process.Kill()
	}
	done := make(chan error, 1)
	go func() {
		done <- p.cmd.Wait()
	}()
	select {
	case <-done:
		fmt.Printf("[INFO] 进程 %d 已正常退出\n", pid)
		return nil
	case <-time.After(5 * time.Second):
		if err := p.cmd.Process.Signal(syscall.Signal(0)); err == nil {
			fmt.Printf("[WARN] 进程 %d 未退出，强制关闭 (SIGKILL)\n", pid)
			return p.cmd.Process.Kill()
		}
		return nil
	}
}
func GetProcess(roomID int) (*Process, bool) {
	processMu.RLock()
	defer processMu.RUnlock()
	p, exists := processes[roomID]
	return p, exists
}
func GetPluginServerOutputBuffer() string {
	p, exists := GetProcess(0)
	if !exists || p == nil {
		return ""
	}
	return p.GetOutputBuffer()
}
func StopProcess(roomID int) error {
	processMu.Lock()
	defer processMu.Unlock()
	p, exists := processes[roomID]
	if !exists {
		return fmt.Errorf("进程不存在")
	}
	if err := p.Stop(); err != nil {
		return err
	}
	delete(processes, roomID)
	return nil
}
func (p *Process) captureAdminToken(line string) {
	re := regexp.MustCompile(`/setup\s+(\d+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		token := "/setup " + matches[1]
		log.Printf("[INFO] 捕获到 TShock 管理员令牌: %s (房间 ID: %d)", token, p.roomID)
		if p.logWriter != nil {
			fmt.Fprintf(p.logWriter, "[ADMIN_TOKEN] %s\n", token)
		}
	}
}
