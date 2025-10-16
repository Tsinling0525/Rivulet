package main

import (
    "context"
    "flag"
    "fmt"
    "io"
    "bytes"
    "encoding/json"
    "net/http"
    "os"
    "os/exec"
    "os/signal"
    "path/filepath"
    "strconv"
    "strings"
    "syscall"
    "time"

	"github.com/Tsinling0525/rivulet/cmd/api/server"
	_ "github.com/Tsinling0525/rivulet/nodes/echo"
	_ "github.com/Tsinling0525/rivulet/nodes/files"
	_ "github.com/Tsinling0525/rivulet/nodes/http"
	_ "github.com/Tsinling0525/rivulet/nodes/logic"
	_ "github.com/Tsinling0525/rivulet/nodes/merge"
	_ "github.com/Tsinling0525/rivulet/nodes/fs"
	_ "github.com/Tsinling0525/rivulet/nodes/ollama"
)

func runServer() error {
	r := server.NewRouter()
	port := os.Getenv("RIV_API_PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("🚀 Starting Rivulet API Server on :%s\n", port)
	srv := &http.Server{Addr: ":" + port, Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("server error: %v\n", err)
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func main() {
    if len(os.Args) < 2 {
        _ = runServer()
        return
    }
    sub := os.Args[1]
    switch sub {
    case "server":
        _ = runServer()
    case "start":
        if err := startDaemon(); err != nil {
            fmt.Println("start error:", err)
            os.Exit(1)
        }
    case "stop":
        if err := stopDaemon(); err != nil {
            fmt.Println("stop error:", err)
            os.Exit(1)
        }
    case "status":
        if err := statusDaemon(); err != nil {
            fmt.Println("status error:", err)
            os.Exit(1)
        }
    case "run":
        fs := flag.NewFlagSet("run", flag.ExitOnError)
        file := fs.String("file", "", "Path to n8n workflow JSON")
        _ = fs.Parse(os.Args[2:])
        if *file == "" {
            fmt.Println("--file is required")
            os.Exit(2)
        }
        if err := runFlowFromFile(*file); err != nil {
            fmt.Println("error:", err)
            os.Exit(1)
        }
    case "inst":
        if len(os.Args) < 3 {
            fmt.Println("Usage: rivulet inst <create|ps|stop|logs|enqueue> [args]")
            os.Exit(2)
        }
        sub2 := os.Args[2]
        switch sub2 {
        case "create":
            fs := flag.NewFlagSet("inst create", flag.ExitOnError)
            wf := fs.String("workflow", "", "Path to workflow JSON")
            _ = fs.Parse(os.Args[3:])
            if *wf == "" { fmt.Println("--workflow is required"); os.Exit(2) }
            if err := instCreate(*wf); err != nil { fmt.Println("error:", err); os.Exit(1) }
        case "ps":
            if err := instPS(); err != nil { fmt.Println("error:", err); os.Exit(1) }
        case "stop":
            fs := flag.NewFlagSet("inst stop", flag.ExitOnError)
            id := fs.String("id", "", "Instance ID")
            _ = fs.Parse(os.Args[3:])
            if *id == "" { fmt.Println("--id is required"); os.Exit(2) }
            if err := instStop(*id); err != nil { fmt.Println("error:", err); os.Exit(1) }
        case "logs":
            fs := flag.NewFlagSet("inst logs", flag.ExitOnError)
            id := fs.String("id", "", "Instance ID")
            _ = fs.Parse(os.Args[3:])
            if *id == "" { fmt.Println("--id is required"); os.Exit(2) }
            if err := instLogs(*id); err != nil { fmt.Println("error:", err); os.Exit(1) }
        case "enqueue":
            fs := flag.NewFlagSet("inst enqueue", flag.ExitOnError)
            id := fs.String("id", "", "Instance ID")
            data := fs.String("data", "", "Path to JSON file with {\"data\": ...} or full n8n request")
            _ = fs.Parse(os.Args[3:])
            if *id == "" || *data == "" { fmt.Println("--id and --data are required"); os.Exit(2) }
            if err := instEnqueue(*id, *data); err != nil { fmt.Println("error:", err); os.Exit(1) }
        default:
            fmt.Println("Usage: rivulet inst <create|ps|stop|logs|enqueue> [args]")
            os.Exit(2)
        }
    default:
        fmt.Println("Usage:")
        fmt.Println("  rivulet server             # start API server (foreground)")
        fmt.Println("  rivulet start              # start background daemon")
        fmt.Println("  rivulet stop               # stop background daemon")
        fmt.Println("  rivulet status             # show daemon status")
        fmt.Println("  rivulet run --file path    # run workflow JSON once")
        fmt.Println("  rivulet inst ...           # manage workflow instances")
    }
}

// --- Daemon helpers ---

func rivHomeDir() (string, error) {
    if v := os.Getenv("RIV_HOME"); v != "" {
        return v, nil
    }
    h, err := os.UserHomeDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(h, ".rivulet"), nil
}

func ensureDir(path string) error {
    if fi, err := os.Stat(path); err == nil {
        if fi.IsDir() {
            return nil
        }
        return fmt.Errorf("%s exists and is not a directory", path)
    } else if os.IsNotExist(err) {
        return os.MkdirAll(path, 0o755)
    } else {
        return err
    }
}

func pidFilePath() (string, error) {
    base, err := rivHomeDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(base, "rivulet.pid"), nil
}

func logFilePath() (string, error) {
    base, err := rivHomeDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(base, "rivulet.log"), nil
}

func readPID() (int, error) {
    p, err := pidFilePath()
    if err != nil {
        return 0, err
    }
    b, err := os.ReadFile(p)
    if err != nil {
        return 0, err
    }
    s := string(b)
    pid, err := strconv.Atoi(strings.TrimSpace(s))
    if err != nil {
        return 0, err
    }
    return pid, nil
}

func writePID(pid int) error {
    p, err := pidFilePath()
    if err != nil {
        return err
    }
    return os.WriteFile(p, []byte(strconv.Itoa(pid)), 0o644)
}

func removePIDFile() {
    if p, err := pidFilePath(); err == nil {
        _ = os.Remove(p)
    }
}

func isRunning(pid int) bool {
    // Signal 0 checks existence
    if pid <= 0 {
        return false
    }
    err := syscall.Kill(pid, 0)
    if err == nil {
        return true
    }
    // EPERM means we can’t signal but it exists
    return err == syscall.EPERM
}

func startDaemon() error {
    home, err := rivHomeDir()
    if err != nil {
        return err
    }
    if err := ensureDir(home); err != nil {
        return err
    }
    pidPath, _ := pidFilePath()
    if b, err := os.ReadFile(pidPath); err == nil {
        if pid, perr := strconv.Atoi(strings.TrimSpace(string(b))); perr == nil && isRunning(pid) {
            return fmt.Errorf("rivulet already running (pid %d)", pid)
        }
    }

    logPath, _ := logFilePath()
    lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
    if err != nil {
        return err
    }
    // best-effort header
    _, _ = io.WriteString(lf, time.Now().Format(time.RFC3339)+" starting rivulet daemon\n")

    bin, err := os.Executable()
    if err != nil {
        return err
    }
    cmd := exec.Command(bin, "server")
    cmd.Stdout = lf
    cmd.Stderr = lf
    cmd.Stdin = nil
    cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
    if err := cmd.Start(); err != nil {
        _ = lf.Close()
        return err
    }
    if err := writePID(cmd.Process.Pid); err != nil {
        _ = lf.Close()
        return err
    }
    // Don't wait; process continues in background
    _ = lf.Close()
    fmt.Printf("Rivulet started in background (pid %d). Logs: %s\n", cmd.Process.Pid, logPath)
    return nil
}

func stopDaemon() error {
    pid, err := readPID()
    if err != nil {
        return fmt.Errorf("cannot read pid file: %w", err)
    }
    if !isRunning(pid) {
        removePIDFile()
        fmt.Println("Rivulet is not running")
        return nil
    }
    if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
        return err
    }
    // Wait up to 5s for shutdown
    deadline := time.Now().Add(5 * time.Second)
    for time.Now().Before(deadline) {
        if !isRunning(pid) {
            removePIDFile()
            fmt.Println("Rivulet stopped")
            return nil
        }
        time.Sleep(150 * time.Millisecond)
    }
    // Force kill
    _ = syscall.Kill(pid, syscall.SIGKILL)
    removePIDFile()
    fmt.Println("Rivulet force-stopped")
    return nil
}

func statusDaemon() error {
    pid, err := readPID()
    if err != nil {
        fmt.Println("Rivulet not running (no pid file)")
        return nil
    }
    if isRunning(pid) {
        logPath, _ := logFilePath()
        fmt.Printf("Rivulet running (pid %d). Logs: %s\n", pid, logPath)
    } else {
        fmt.Printf("Rivulet not running (stale pid %d)\n", pid)
        removePIDFile()
    }
    return nil
}

// --- Instance CLI helpers (call local API) ---

func apiBase() string {
    port := os.Getenv("RIV_API_PORT")
    if port == "" { port = "8080" }
    return "http://127.0.0.1:" + port
}

func httpJSON(method, path string, payload any) (map[string]any, error) {
    var body *bytes.Reader
    if payload != nil {
        b, err := json.Marshal(payload)
        if err != nil { return nil, err }
        body = bytes.NewReader(b)
    } else {
        body = bytes.NewReader([]byte{})
    }
    req, _ := http.NewRequest(method, apiBase()+path, body)
    req.Header.Set("Content-Type", "application/json")
    resp, err := http.DefaultClient.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()
    var out map[string]any
    dec := json.NewDecoder(resp.Body)
    if err := dec.Decode(&out); err != nil { return nil, err }
    if ok, _ := out["success"].(bool); !ok {
        if msg, _ := out["error"].(string); msg != "" { return nil, fmt.Errorf(msg) }
        return nil, fmt.Errorf("request failed")
    }
    if data, _ := out["data"].(map[string]any); data != nil { return data, nil }
    return out, nil
}

func instCreate(path string) error {
    data, err := httpJSON("POST", "/instances", map[string]string{"workflow_path": path})
    if err != nil { return err }
    fmt.Printf("created instance: %s (state=%v)\n", data["id"], data["state"])
    return nil
}

func instPS() error {
    data, err := httpJSON("GET", "/instances", nil)
    if err != nil { return err }
    insts, _ := data["instances"].([]any)
    for _, it := range insts {
        m := it.(map[string]any)
        fmt.Printf("%s\t%s\t%s\t%s\n", m["id"], m["state"], m["name"], m["workflow_path"])
    }
    return nil
}

func instStop(id string) error {
    _, err := httpJSON("POST", "/instances/"+id+"/stop", map[string]any{})
    return err
}

func instLogs(id string) error {
    data, err := httpJSON("GET", "/instances/"+id+"/logs", nil)
    if err != nil { return err }
    logs, _ := data["logs"].([]any)
    for _, l := range logs { fmt.Println(l) }
    return nil
}

func instEnqueue(id, path string) error {
    b, err := os.ReadFile(path)
    if err != nil { return err }
    var tmp map[string]any
    if err := json.Unmarshal(b, &tmp); err != nil { return err }
    payload := any(tmp)
    if d, ok := tmp["data"]; ok {
        payload = map[string]any{"data": d}
    }
    _, err = httpJSON("POST", "/instances/"+id+"/enqueue", payload)
    return err
}
