package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

const (
	serverPort        = 4821
	defaultListenAddr = "127.0.0.1:4821"
	dataVersion       = 1
	defaultFileMode   = 0o644
	defaultDirMode    = 0o755
	dataDirEnvKey     = "SLASH_KEY_DATA_DIR"
	listenAddrEnvKey  = "SLASH_KEY_LISTEN_ADDR"
)

var vpnInterfacePrefixes = []string{"utun", "tun", "tap", "ppp", "wg"}

var vpnInterfaceAddrs = listVPNInterfaceAddrs

var (
	errUsage               = errors.New("usage error")
	errRuntime             = errors.New("runtime error")
	errProjectExists       = errors.New("project already exists")
	errProjectNotFound     = errors.New("project not found")
	errDaemonNotRunning    = errors.New("daemon not running")
	errCodexNotImplemented = errors.New("add --codex is not implemented yet")
)

type project struct {
	ID          string    `json:"id"`
	RootPath    string    `json:"rootPath"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

type registryFile struct {
	Projects []project `json:"projects"`
}

type pathEntry struct {
	ProjectID    string   `json:"projectId"`
	RelativePath string   `json:"relativePath"`
	Kind         string   `json:"kind"`
	Segments     []string `json:"segments"`
	Basename     string   `json:"basename"`
}

type indexFile struct {
	ProjectID string      `json:"projectId"`
	RootPath  string      `json:"rootPath"`
	BuiltAt   time.Time   `json:"builtAt"`
	Entries   []pathEntry `json:"entries"`
}

type daemonState struct {
	PID         int       `json:"pid"`
	Port        int       `json:"port"`
	ListenAddr  string    `json:"listenAddr"`
	Status      string    `json:"status"`
	StartedAt   time.Time `json:"startedAt"`
	DataVersion int       `json:"dataVersion"`
}

type daemonRuntime struct {
	registry []project
	indexes  []indexFile
}

type appPaths struct {
	dataDir     string
	registry    string
	daemonState string
	indexDir    string
	logDir      string
	logFile     string
}

func main() {
	code := run(os.Args[1:])
	os.Exit(code)
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return 1
	}

	paths, err := resolvePaths()
	if err != nil {
		return printError(err, 2)
	}

	switch args[0] {
	case "start":
		if err := cmdStart(paths, args[1:]); err != nil {
			return classifyError(err)
		}
		return 0
	case "stop":
		if err := cmdStop(paths); err != nil {
			return classifyError(err)
		}
		return 0
	case "status":
		if err := cmdStatus(paths, os.Stdout); err != nil {
			return classifyError(err)
		}
		return 0
	case "list":
		if err := cmdList(paths, os.Stdout); err != nil {
			return classifyError(err)
		}
		return 0
	case "add":
		if err := cmdAdd(paths, args[1:]); err != nil {
			return classifyError(err)
		}
		return 0
	case "delete":
		if err := cmdDelete(paths, args[1:]); err != nil {
			return classifyError(err)
		}
		return 0
	case "path":
		if err := cmdPath(paths, args[1:], os.Stdout); err != nil {
			return classifyError(err)
		}
		return 0
	case "serve":
		if err := cmdServe(paths); err != nil {
			return printError(err, 2)
		}
		return 0
	default:
		printUsage(os.Stderr)
		return 1
	}
}

func classifyError(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, errUsage) || errors.Is(err, errProjectExists) || errors.Is(err, errProjectNotFound) || errors.Is(err, errDaemonNotRunning) || errors.Is(err, errCodexNotImplemented) {
		return printError(err, 1)
	}
	return printError(err, 2)
}

func printError(err error, exitCode int) int {
	fmt.Fprintln(os.Stderr, err.Error())
	return exitCode
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  slash-key start [-e [ipAddr]]")
	fmt.Fprintln(w, "  slash-key stop")
	fmt.Fprintln(w, "  slash-key status")
	fmt.Fprintln(w, "  slash-key list")
	fmt.Fprintln(w, "  slash-key add <dirPath>")
	fmt.Fprintln(w, "  slash-key add --codex")
	fmt.Fprintln(w, "  slash-key delete <dirPath>")
	fmt.Fprintln(w, "  slash-key path [query]")
}

func resolvePaths() (appPaths, error) {
	dataDir := os.Getenv(dataDirEnvKey)
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return appPaths{}, fmt.Errorf("%w: resolve home: %v", errRuntime, err)
		}
		dataDir = filepath.Join(home, ".slash-key")
	}
	return appPaths{
		dataDir:     dataDir,
		registry:    filepath.Join(dataDir, "registry.json"),
		daemonState: filepath.Join(dataDir, "daemon.json"),
		indexDir:    filepath.Join(dataDir, "indexes"),
		logDir:      filepath.Join(dataDir, "logs"),
		logFile:     filepath.Join(dataDir, "logs", "daemon.log"),
	}, nil
}

func ensureDataDirs(paths appPaths) error {
	for _, path := range []string{paths.dataDir, paths.indexDir, paths.logDir} {
		if err := os.MkdirAll(path, defaultDirMode); err != nil {
			return fmt.Errorf("%w: mkdir %s: %v", errRuntime, path, err)
		}
	}
	return nil
}

func loadRegistry(paths appPaths) (registryFile, error) {
	var registry registryFile
	data, err := os.ReadFile(paths.registry)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return registryFile{}, nil
		}
		return registryFile{}, fmt.Errorf("%w: read registry: %v", errRuntime, err)
	}
	if len(data) == 0 {
		return registryFile{}, nil
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return registryFile{}, fmt.Errorf("%w: parse registry: %v", errRuntime, err)
	}
	return registry, nil
}

func saveRegistry(paths appPaths, registry registryFile) error {
	if err := ensureDataDirs(paths); err != nil {
		return err
	}
	return writeJSON(paths.registry, registry)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: marshal json: %v", errRuntime, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, defaultFileMode); err != nil {
		return fmt.Errorf("%w: write %s: %v", errRuntime, path, err)
	}
	return nil
}

func normalizeProjectPath(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("%w: path is required", errUsage)
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("%w: resolve path: %v", errRuntime, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: directory does not exist", errUsage)
		}
		return "", fmt.Errorf("%w: normalize path: %v", errRuntime, err)
	}
	info, err := os.Stat(real)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: directory does not exist", errUsage)
		}
		return "", fmt.Errorf("%w: stat path: %v", errRuntime, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: path is not a directory", errUsage)
	}
	return filepath.Clean(real), nil
}

func projectID(root string) string {
	var b strings.Builder
	b.WriteString("proj_")
	for _, part := range strings.Split(root, string(os.PathSeparator)) {
		if part == "" {
			continue
		}
		for _, r := range part {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			} else {
				b.WriteByte('_')
			}
		}
		b.WriteByte('_')
	}
	return strings.TrimRight(b.String(), "_")
}

func indexPath(paths appPaths, id string) string {
	return filepath.Join(paths.indexDir, id+".json")
}

func cmdAdd(paths appPaths, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("%w: slash-key add <dirPath> or slash-key add --codex", errUsage)
	}
	if args[0] == "--codex" {
		return errCodexNotImplemented
	}

	root, err := normalizeProjectPath(args[0])
	if err != nil {
		return err
	}
	registry, err := loadRegistry(paths)
	if err != nil {
		return err
	}
	for _, existing := range registry.Projects {
		if existing.RootPath == root {
			return fmt.Errorf("%w: %s", errProjectExists, root)
		}
	}

	project := project{
		ID:          projectID(root),
		RootPath:    root,
		DisplayName: filepath.Base(root),
		CreatedAt:   time.Now().UTC(),
	}
	index, err := buildIndex(project)
	if err != nil {
		return err
	}
	registry.Projects = append(registry.Projects, project)
	if err := saveRegistry(paths, registry); err != nil {
		return err
	}
	if err := ensureDataDirs(paths); err != nil {
		return err
	}
	if err := writeJSON(indexPath(paths, project.ID), index); err != nil {
		return err
	}
	if daemon, ok := runningDaemon(paths); ok {
		_ = daemonReload(daemon)
	}
	fmt.Println(root)
	return nil
}

func cmdDelete(paths appPaths, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("%w: slash-key delete <dirPath>", errUsage)
	}
	root, err := normalizeProjectPath(args[0])
	if err != nil {
		return err
	}
	registry, err := loadRegistry(paths)
	if err != nil {
		return err
	}
	filtered := make([]project, 0, len(registry.Projects))
	var removed *project
	for _, item := range registry.Projects {
		if item.RootPath == root {
			copyItem := item
			removed = &copyItem
			continue
		}
		filtered = append(filtered, item)
	}
	if removed == nil {
		return fmt.Errorf("%w: %s", errProjectNotFound, root)
	}
	registry.Projects = filtered
	if err := saveRegistry(paths, registry); err != nil {
		return err
	}
	if err := os.Remove(indexPath(paths, removed.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: remove index: %v", errRuntime, err)
	}
	if daemon, ok := runningDaemon(paths); ok {
		_ = daemonReload(daemon)
	}
	fmt.Println(root)
	return nil
}

func cmdList(paths appPaths, w io.Writer) error {
	registry, err := loadRegistry(paths)
	if err != nil {
		return err
	}
	for _, item := range registry.Projects {
		fmt.Fprintln(w, item.RootPath)
	}
	return nil
}

func cmdPath(paths appPaths, args []string, w io.Writer) error {
	query := ""
	if len(args) > 1 {
		return fmt.Errorf("%w: slash-key path [query]", errUsage)
	}
	if len(args) == 1 {
		query = args[0]
	}
	if daemon, ok := runningDaemon(paths); ok {
		results, err := daemonSearch(daemon, query)
		if err == nil {
			for _, result := range results {
				fmt.Fprintln(w, result)
			}
			return nil
		}
	}
	results, err := localSearch(paths, query)
	if err != nil {
		return err
	}
	for _, result := range results {
		fmt.Fprintln(w, result)
	}
	return nil
}

func cmdStatus(paths appPaths, w io.Writer) error {
	state, ok := runningDaemon(paths)
	if !ok {
		if err := pingDaemon(daemonState{Port: serverPort}); err == nil {
			projects, projectErr := daemonList(daemonState{Port: serverPort})
			if projectErr != nil {
				projects = nil
			}
			fmt.Fprintln(w, "running")
			fmt.Fprintf(w, "http://localhost:%d\n", serverPort)
			fmt.Fprintf(w, "projects: %d\n", len(projects))
			return nil
		}
		if _, err := loadDaemonState(paths); err == nil {
			fmt.Fprintln(w, "stale")
			return nil
		}
		fmt.Fprintln(w, "stopped")
		return nil
	}
	registry, err := loadRegistry(paths)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "running")
	fmt.Fprintf(w, "http://localhost:%d\n", state.Port)
	fmt.Fprintf(w, "projects: %d\n", len(registry.Projects))
	return nil
}

func cmdStart(paths appPaths, args []string) error {
	listenAddr, err := resolveStartListenAddr(args)
	if err != nil {
		return err
	}

	if err := ensureDataDirs(paths); err != nil {
		return err
	}
	if _, ok := runningDaemon(paths); ok {
		fmt.Println("slash-key server already running")
		fmt.Printf("local: http://localhost:%d\n", serverPort)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("%w: resolve executable: %v", errRuntime, err)
	}
	logFile, err := os.OpenFile(paths.logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, defaultFileMode)
	if err != nil {
		return fmt.Errorf("%w: open log file: %v", errRuntime, err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "serve")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(), dataDirEnvKey+"="+paths.dataDir)
	if listenAddr != "" {
		cmd.Env = append(cmd.Env, listenAddrEnvKey+"="+listenAddr)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: start daemon: %v", errRuntime, err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("%w: release daemon process: %v", errRuntime, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if state, ok := runningDaemon(paths); ok {
			if err := pingDaemon(state); err == nil {
				fmt.Println("slash-key server started")
				printStartupURLs(state.ListenAddr)
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("%w: daemon did not become healthy", errRuntime)
}

func cmdStop(paths appPaths) error {
	state, ok := runningDaemon(paths)
	if !ok {
		if err := daemonShutdown(daemonState{Port: serverPort}); err == nil {
			fmt.Println("slash-key server stopped")
			return nil
		}
		return errDaemonNotRunning
	}
	if err := daemonShutdown(state); err != nil && state.PID > 0 {
		process, findErr := os.FindProcess(state.PID)
		if findErr != nil {
			return fmt.Errorf("%w: find process: %v", errRuntime, findErr)
		}
		if signalErr := process.Signal(syscall.SIGTERM); signalErr != nil {
			return fmt.Errorf("%w: stop daemon: %v", errRuntime, err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := runningDaemon(paths); !ok && pingDaemon(daemonState{Port: serverPort}) != nil {
			fmt.Println("slash-key server stopped")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = os.Remove(paths.daemonState)
	fmt.Println("slash-key server stopped")
	return nil
}

func cmdServe(paths appPaths) error {
	if err := ensureDataDirs(paths); err != nil {
		return err
	}
	registry, indexes, err := loadRuntime(paths)
	if err != nil {
		return err
	}
	runtime := &daemonRuntime{registry: registry.Projects, indexes: indexes}

	listenAddr := os.Getenv(listenAddrEnvKey)
	if listenAddr == "" {
		listenAddr = defaultListenAddr
	}
	listenAddr, err = validateServeListenAddr(listenAddr)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("%w: bind %s: %v", errRuntime, listenAddr, err)
	}
	defer ln.Close()

	state := daemonState{
		PID:         os.Getpid(),
		Port:        serverPort,
		ListenAddr:  listenAddr,
		Status:      "running",
		StartedAt:   time.Now().UTC(),
		DataVersion: dataVersion,
	}
	if err := writeJSON(paths.daemonState, state); err != nil {
		return err
	}
	defer os.Remove(paths.daemonState)

	server := &http.Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONResponse(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		registry, indexes, err := loadRuntime(paths)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "RELOAD_FAILED", err.Error())
			return
		}
		runtime.registry = registry.Projects
		runtime.indexes = indexes
		writeJSONResponse(w, http.StatusOK, map[string]any{"status": "reloaded"})
	})
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]any{"status": "stopping"})
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
		}()
	})
	mux.HandleFunc("/list", func(w http.ResponseWriter, _ *http.Request) {
		paths := make([]string, 0, len(runtime.registry))
		for _, item := range runtime.registry {
			paths = append(paths, item.RootPath)
		}
		writeJSONResponse(w, http.StatusOK, paths)
	})
	mux.HandleFunc("/path", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			query = r.URL.Query().Get("query")
		}
		results := searchIndexes(runtime.indexes, query)
		writeJSONResponse(w, http.StatusOK, results)
	})

	server.Handler = mux
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-shutdownCh
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("%w: serve: %v", errRuntime, err)
	}
	return nil
}

func printStartupURLs(listenAddr string) {
	for _, line := range startupURLLines(listenAddr) {
		fmt.Println(line)
	}
}

func startupURLLines(listenAddr string) []string {
	lines := []string{fmt.Sprintf("local: http://localhost:%d", serverPort)}
	host, _, err := net.SplitHostPort(listenAddr)
	if err != nil || host == "" || isLoopbackHost(host) {
		return lines
	}
	lines = append(lines, fmt.Sprintf("network: http://%s:%d", host, serverPort))
	return lines
}

func resolveStartListenAddr(args []string) (string, error) {
	switch len(args) {
	case 0:
		return "", nil
	case 1:
		if args[0] != "-e" {
			return "", fmt.Errorf("%w: slash-key start [-e [ipAddr]]", errUsage)
		}
		return autoDetectVPNListenAddr()
	case 2:
		if args[0] != "-e" {
			return "", fmt.Errorf("%w: slash-key start [-e [ipAddr]]", errUsage)
		}
		return explicitListenAddr(args[1])
	default:
		return "", fmt.Errorf("%w: slash-key start [-e [ipAddr]]", errUsage)
	}
}

func explicitListenAddr(host string) (string, error) {
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("%w: invalid ip address %q", errUsage, host)
	}
	return validateServeListenAddr(net.JoinHostPort(ip.String(), fmt.Sprintf("%d", serverPort)))
}

func autoDetectVPNListenAddr() (string, error) {
	addrs, err := vpnInterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("%w: detect vpn address: %v", errRuntime, err)
	}
	for _, addr := range addrs {
		if validated, err := validateServeListenAddr(net.JoinHostPort(addr.String(), fmt.Sprintf("%d", serverPort))); err == nil {
			return validated, nil
		}
	}
	return "", fmt.Errorf("%w: no safe vpn address found for -e", errUsage)
}

func validateServeListenAddr(listenAddr string) (string, error) {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return "", fmt.Errorf("%w: invalid listen addr %q", errUsage, listenAddr)
	}
	if port != fmt.Sprintf("%d", serverPort) {
		return "", fmt.Errorf("%w: listen port must be %d", errUsage, serverPort)
	}
	if isWildcardHost(host) {
		return "", fmt.Errorf("%w: wildcard listen addr is not allowed", errUsage)
	}
	if isLoopbackHost(host) {
		return net.JoinHostPort(normalizeLoopbackHost(host), port), nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("%w: invalid listen host %q", errUsage, host)
	}
	if isLANIP(ip) {
		return "", fmt.Errorf("%w: lan listen addr is not allowed", errUsage)
	}
	if ok, err := isVPNInterfaceAddr(ip); err != nil {
		return "", fmt.Errorf("%w: inspect vpn interfaces: %v", errRuntime, err)
	} else if !ok {
		return "", fmt.Errorf("%w: listen addr must belong to a vpn interface", errUsage)
	}
	return net.JoinHostPort(ip.String(), port), nil
}

func isWildcardHost(host string) bool {
	return host == "0.0.0.0" || host == "::" || host == ""
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizeLoopbackHost(host string) string {
	if host == "localhost" {
		return "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() && ip.To4() != nil {
		return "127.0.0.1"
	}
	return host
}

func isLANIP(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	switch {
	case ip4[0] == 10:
		return true
	case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
		return true
	case ip4[0] == 192 && ip4[1] == 168:
		return true
	default:
		return false
	}
}

func isVPNInterfaceAddr(target net.IP) (bool, error) {
	addrs, err := vpnInterfaceAddrs()
	if err != nil {
		return false, err
	}
	for _, addr := range addrs {
		if addr.Equal(target) {
			return true, nil
		}
	}
	return false, nil
}

func listVPNInterfaceAddrs() ([]net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var addrs []net.IP
	for _, iface := range ifaces {
		if !isVPNInterfaceName(iface.Name) || iface.Flags&net.FlagUp == 0 {
			continue
		}
		interfaceAddrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range interfaceAddrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || isLANIP(ip) {
				continue
			}
			addrs = append(addrs, ip)
		}
	}
	return addrs, nil
}

func isVPNInterfaceName(name string) bool {
	for _, prefix := range vpnInterfacePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func machineIPv4() (string, error) {
	addrs, err := vpnInterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, ip := range addrs {
		ip4 := ip.To4()
		if ip4 == nil {
			continue
		}
		return ip4.String(), nil
	}

	interfaceAddrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, addr := range interfaceAddrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || isLANIP(ip) {
			continue
		}
		if ip.IsGlobalUnicast() {
			return ip.String(), nil
		}
	}
	return "", errors.New("no suitable network address found")
}

func machineIPv4OrEmpty() string {
	ip, err := machineIPv4()
	if err != nil {
		return ""
	}
	return ip
}

func runningDaemon(paths appPaths) (daemonState, bool) {
	state, err := loadDaemonState(paths)
	if err != nil {
		return daemonState{}, false
	}
	if err := pingDaemon(state); err != nil {
		_ = os.Remove(paths.daemonState)
		return daemonState{}, false
	}
	return state, true
}

func loadDaemonState(paths appPaths) (daemonState, error) {
	data, err := os.ReadFile(paths.daemonState)
	if err != nil {
		return daemonState{}, err
	}
	var state daemonState
	if err := json.Unmarshal(data, &state); err != nil {
		return daemonState{}, err
	}
	return state, nil
}

func daemonBaseURL(state daemonState) string {
	host := "127.0.0.1"
	if state.ListenAddr != "" {
		if listenHost, _, err := net.SplitHostPort(state.ListenAddr); err == nil && listenHost != "" {
			host = normalizeLoopbackHost(listenHost)
		}
	}
	return fmt.Sprintf("http://%s:%d", host, state.Port)
}

func pingDaemon(state daemonState) error {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(daemonBaseURL(state) + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health status %d", resp.StatusCode)
	}
	return nil
}

func daemonReload(state daemonState) error {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest(http.MethodPost, daemonBaseURL(state)+"/reload", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reload status %d", resp.StatusCode)
	}
	return nil
}

func daemonShutdown(state daemonState) error {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest(http.MethodPost, daemonBaseURL(state)+"/shutdown", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("shutdown status %d", resp.StatusCode)
	}
	return nil
}

func daemonSearch(state daemonState, query string) ([]string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	endpoint := daemonBaseURL(state) + "/path"
	if query != "" {
		endpoint += "?q=" + url.QueryEscape(query)
	}
	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search status %d", resp.StatusCode)
	}
	var results []string
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}
	return results, nil
}

func daemonList(state daemonState) ([]string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(daemonBaseURL(state) + "/list")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list status %d", resp.StatusCode)
	}
	var projects []string
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func loadRuntime(paths appPaths) (registryFile, []indexFile, error) {
	registry, err := loadRegistry(paths)
	if err != nil {
		return registryFile{}, nil, err
	}
	indexes := make([]indexFile, 0, len(registry.Projects))
	for _, project := range registry.Projects {
		index, err := loadIndex(paths, project.ID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				built, buildErr := buildIndex(project)
				if buildErr != nil {
					return registryFile{}, nil, buildErr
				}
				if err := writeJSON(indexPath(paths, project.ID), built); err != nil {
					return registryFile{}, nil, err
				}
				index = built
			} else {
				return registryFile{}, nil, err
			}
		}
		indexes = append(indexes, index)
	}
	return registry, indexes, nil
}

func loadIndex(paths appPaths, projectID string) (indexFile, error) {
	data, err := os.ReadFile(indexPath(paths, projectID))
	if err != nil {
		return indexFile{}, err
	}
	var index indexFile
	if err := json.Unmarshal(data, &index); err != nil {
		return indexFile{}, fmt.Errorf("%w: parse index: %v", errRuntime, err)
	}
	return index, nil
}

func localSearch(paths appPaths, query string) ([]string, error) {
	_, indexes, err := loadRuntime(paths)
	if err != nil {
		return nil, err
	}
	return searchIndexes(indexes, query), nil
}

type scoredResult struct {
	path     string
	score    int
	segments int
	length   int
}

func searchIndexes(indexes []indexFile, query string) []string {
	normalizedQuery := strings.ToLower(query)
	scored := make([]scoredResult, 0)
	for _, index := range indexes {
		for _, entry := range index.Entries {
			score, ok := matchEntry(entry, normalizedQuery)
			if !ok {
				continue
			}
			scored = append(scored, scoredResult{
				path:     entry.RelativePath,
				score:    score,
				segments: len(entry.Segments),
				length:   len(entry.RelativePath),
			})
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score < scored[j].score
		}
		if scored[i].segments != scored[j].segments {
			return scored[i].segments < scored[j].segments
		}
		if scored[i].length != scored[j].length {
			return scored[i].length < scored[j].length
		}
		return scored[i].path < scored[j].path
	})
	results := make([]string, 0, len(scored))
	for _, item := range scored {
		results = append(results, item.path)
	}
	return results
}

func matchEntry(entry pathEntry, query string) (int, bool) {
	if query == "" {
		return 0, true
	}
	basename := strings.ToLower(entry.Basename)
	if basename == query {
		return 0, true
	}
	if strings.HasPrefix(basename, query) {
		return 1, true
	}
	for _, segment := range entry.Segments {
		if strings.HasPrefix(strings.ToLower(segment), query) {
			return 2, true
		}
	}
	if strings.Contains(strings.ToLower(entry.RelativePath), query) {
		return 3, true
	}
	return 0, false
}

func buildIndex(project project) (indexFile, error) {
	entries, err := scanProject(project.RootPath)
	if err != nil {
		return indexFile{}, err
	}
	for i := range entries {
		entries[i].ProjectID = project.ID
	}
	return indexFile{
		ProjectID: project.ID,
		RootPath:  project.RootPath,
		BuiltAt:   time.Now().UTC(),
		Entries:   entries,
	}, nil
}

func scanProject(root string) ([]pathEntry, error) {
	allPaths := make([]string, 0, 256)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		base := filepath.Base(path)
		if isAlwaysIgnored(base) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		allPaths = append(allPaths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: scan project: %v", errRuntime, err)
	}
	ignored, err := gitIgnoredPaths(root, allPaths)
	if err != nil {
		return nil, err
	}
	entries := make([]pathEntry, 0, len(allPaths))
	for _, path := range allPaths {
		if ignored[path] {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("%w: stat entry: %v", errRuntime, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, fmt.Errorf("%w: relative path: %v", errRuntime, err)
		}
		rel = filepath.ToSlash(rel)
		entries = append(entries, pathEntry{
			RelativePath: "./" + rel,
			Kind:         entryKind(info),
			Segments:     strings.Split(rel, "/"),
			Basename:     filepath.Base(path),
		})
	}
	return entries, nil
}

func isAlwaysIgnored(base string) bool {
	switch base {
	case ".git", "node_modules", "dist", "build", ".next", "coverage":
		return true
	default:
		return false
	}
}

func entryKind(info os.FileInfo) string {
	if info.IsDir() {
		return "directory"
	}
	return "file"
}

func gitIgnoredPaths(root string, paths []string) (map[string]bool, error) {
	ignored := make(map[string]bool)
	if len(paths) == 0 {
		return ignored, nil
	}

	rootFS := osfs.New(root)
	patterns, err := gitignore.ReadPatterns(rootFS, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: read gitignore patterns: %v", errRuntime, err)
	}

	hostFS := osfs.New(string(os.PathSeparator))
	globalPatterns, err := gitignore.LoadGlobalPatterns(hostFS)
	if err == nil {
		patterns = append(globalPatterns, patterns...)
	}
	systemPatterns, err := gitignore.LoadSystemPatterns(hostFS)
	if err == nil {
		patterns = append(systemPatterns, patterns...)
	}
	matcher := gitignore.NewMatcher(patterns)

	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, fmt.Errorf("%w: relative path for ignore: %v", errRuntime, err)
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("%w: stat ignore candidate: %v", errRuntime, err)
		}
		if matcher.Match(parts, info.IsDir()) {
			ignored[filepath.Clean(path)] = true
		}
	}
	return ignored, nil
}

func writeJSONResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	writeJSONResponse(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
