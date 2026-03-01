package tools

import (
	"archive/zip"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/vision"
)

// VisionInstallTool manages the Gorkbot Vision companion Android app.
// The companion app provides MediaProjection-based screen capture over
// a local HTTP service on 127.0.0.1:7777 — no ADB, no root required.
//
// Build chain (no Gradle, no full Android SDK required):
//   pkg install aapt2 d8 apksigner   (Termux — small, fast)
//   android-34.jar downloaded once from Google's SDK CDN (~70 MB)
//   javac → d8 → aapt2 link → apksigner
type VisionInstallTool struct{ BaseTool }

func NewVisionInstallTool() *VisionInstallTool {
	return &VisionInstallTool{BaseTool{
		name: "vision_install",
		description: "Install, start, or check the Gorkbot Vision companion app. " +
			"The companion uses Android's MediaProjection API for screen capture " +
			"over localhost — no ADB or root required. " +
			"Actions: status, install (build+install), launch (start service), stop.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *VisionInstallTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "status | install | launch | stop",
				"enum":        []string{"status", "install", "launch", "stop"},
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *VisionInstallTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action := "status"
	if a, ok := params["action"].(string); ok && a != "" {
		action = a
	}

	switch action {
	case "install":
		return t.install(ctx)
	case "launch":
		return t.launch(ctx)
	case "stop":
		return t.stop(ctx)
	default:
		return t.status(ctx)
	}
}

// ─── status ──────────────────────────────────────────────────────────────────

func (t *VisionInstallTool) status(ctx context.Context) (*ToolResult, error) {
	running := vision.CompanionRunning(ctx)

	apkInstalled := isCompanionInstalled(ctx)

	projectDir := companionProjectDir()
	_, srcErr := os.Stat(filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml"))
	srcExists := srcErr == nil

	_, jarErr := os.Stat(filepath.Join(projectDir, "android-34.jar"))
	jarCached := jarErr == nil

	var sb strings.Builder
	sb.WriteString("## Gorkbot Vision Companion Status\n\n")
	sb.WriteString(fmt.Sprintf("Service running:   %s\n", boolIcon(running)))
	sb.WriteString(fmt.Sprintf("APK installed:     %s\n", boolIcon(apkInstalled)))
	sb.WriteString(fmt.Sprintf("Source generated:  %s  (%s)\n", boolIcon(srcExists), projectDir))
	sb.WriteString(fmt.Sprintf("android-34.jar:    %s  (build cache)\n", boolIcon(jarCached)))

	if running {
		sb.WriteString("\n✓ **Vision tools are ready.** Screen capture active on 127.0.0.1:7777.\n")
	} else if apkInstalled {
		sb.WriteString("\n⚠ App installed but service not running.\n")
		sb.WriteString("Use: `vision_install` with action=launch\n")
	} else {
		sb.WriteString("\n✗ Companion not installed. Use: `vision_install` with action=install\n")
	}

	return &ToolResult{
		Success: running,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"running":       running,
			"apk_installed": apkInstalled,
			"source_exists": srcExists,
			"jar_cached":    jarCached,
			"project_dir":   projectDir,
		},
	}, nil
}

// ─── install ─────────────────────────────────────────────────────────────────

func (t *VisionInstallTool) install(ctx context.Context) (*ToolResult, error) {
	var sb strings.Builder
	sb.WriteString("## Installing Gorkbot Vision Companion\n\n")

	// Step 1: Generate project source
	sb.WriteString("**Step 1/5:** Generating Android project source...\n")
	projectDir, err := generateCompanionProject()
	if err != nil {
		return errorResult("generate project: " + err.Error()), nil
	}
	sb.WriteString(fmt.Sprintf("  ✓ Source written to %s\n\n", projectDir))

	// Step 2: Install Termux build tools (aapt2, d8, apksigner)
	sb.WriteString("**Step 2/5:** Checking/installing Termux build tools...\n")
	if toolMsg, err := installTermuxBuildTools(ctx); err != nil {
		sb.WriteString("  ✗ " + err.Error() + "\n")
		sb.WriteString("  Try manually: pkg install aapt2 d8 apksigner\n")
		return &ToolResult{Success: false, Output: sb.String(), Error: err.Error()}, nil
	} else {
		sb.WriteString("  ✓ " + toolMsg + "\n\n")
	}

	// Step 3: Download android-34.jar (one-time, ~70 MB)
	sb.WriteString("**Step 3/5:** Fetching Android API stubs (android-34.jar)...\n")
	sb.WriteString("  First run downloads ~70 MB from Google's SDK CDN; cached after that.\n")
	androidJar, err := ensureAndroidJar(ctx, projectDir)
	if err != nil {
		sb.WriteString("  ✗ " + err.Error() + "\n")
		return &ToolResult{Success: false, Output: sb.String(), Error: err.Error()}, nil
	}
	sb.WriteString("  ✓ android-34.jar ready\n\n")

	// Step 4: Build APK (compile → DEX → package → sign)
	sb.WriteString("**Step 4/5:** Building APK (javac → d8 → aapt2 → apksigner)...\n")
	apkPath, err := buildAPKManual(ctx, projectDir, androidJar)
	if err != nil {
		sb.WriteString("  ✗ Build failed: " + err.Error() + "\n")
		return &ToolResult{Success: false, Output: sb.String(), Error: err.Error()}, nil
	}
	sb.WriteString(fmt.Sprintf("  ✓ APK built: %s\n\n", apkPath))

	// Step 5: Trigger Android install dialog
	sb.WriteString("**Step 5/5:** Opening Android install dialog...\n")
	installOut, err := installAPK(ctx, apkPath)
	if err != nil {
		sb.WriteString("  ✗ Install trigger failed: " + err.Error() + "\n")
		sb.WriteString(fmt.Sprintf("  Manual install: open Files app → navigate to %s → tap it\n", apkPath))
	} else {
		sb.WriteString("  ✓ " + installOut + "\n")
	}

	sb.WriteString("\n---\n")
	sb.WriteString("**After the install dialog completes**, run:\n")
	sb.WriteString("  `vision_install` with action=launch\n\n")
	sb.WriteString("That launches PermissionActivity which shows Android's one-time\n")
	sb.WriteString("\"Allow Gorkbot Vision to capture your screen?\" dialog. Tap **Start now**.\n")
	sb.WriteString("Vision tools will then work permanently.\n")

	return &ToolResult{
		Success: true,
		Output:  sb.String(),
		Data:    map[string]interface{}{"apk_path": apkPath, "project_dir": projectDir},
	}, nil
}

// ─── launch ──────────────────────────────────────────────────────────────────

func (t *VisionInstallTool) launch(ctx context.Context) (*ToolResult, error) {
	if vision.CompanionRunning(ctx) {
		return &ToolResult{
			Success: true,
			Output:  "## Gorkbot Vision\n\n✓ Already running on 127.0.0.1:7777",
		}, nil
	}

	out, err := runShell(ctx, 30*time.Second,
		"am start -n ai.velarium.gorkbot/.PermissionActivity")
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to launch: %v\n%s", err, out),
			Output: "## Launch Failed\n\nCompanion app may not be installed.\n" +
				"Run `vision_install` with action=install first.\n",
		}, nil
	}

	time.Sleep(3 * time.Second)

	var sb strings.Builder
	sb.WriteString("## Gorkbot Vision\n\n")
	sb.WriteString("PermissionActivity launched. **Tap \"Start now\"** on the dialog.\n\n")

	if vision.CompanionRunning(ctx) {
		sb.WriteString("✓ Service is running on 127.0.0.1:7777 — vision tools ready.\n")
		return &ToolResult{Success: true, Output: sb.String()}, nil
	}

	sb.WriteString("Service starting... (waiting for user to tap the permission dialog)\n")
	sb.WriteString("Run `vision_install` with action=status to check once you've tapped it.\n")
	return &ToolResult{Success: true, Output: sb.String()}, nil
}

// ─── stop ────────────────────────────────────────────────────────────────────

func (t *VisionInstallTool) stop(ctx context.Context) (*ToolResult, error) {
	if !vision.CompanionRunning(ctx) {
		return &ToolResult{Success: true, Output: "Companion is not running."}, nil
	}
	if err := vision.StopCompanion(ctx); err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}
	return &ToolResult{Success: true, Output: "✓ Companion stopped."}, nil
}

// ─── build helpers ───────────────────────────────────────────────────────────

// installTermuxBuildTools installs aapt2, d8, and apksigner via pkg if missing.
func installTermuxBuildTools(ctx context.Context) (string, error) {
	needed := []string{"aapt2", "d8", "apksigner"}
	var missing []string
	for _, t := range needed {
		if _, err := exec.LookPath(t); err != nil {
			missing = append(missing, t)
		}
	}
	if len(missing) == 0 {
		return "aapt2, d8, apksigner — all present", nil
	}

	args := append([]string{"install", "-y"}, missing...)
	cmd := exec.CommandContext(ctx, "pkg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pkg install %v failed: %w\n%s", missing, err, out)
	}

	// Re-check after install
	var stillMissing []string
	for _, t := range missing {
		if _, err := exec.LookPath(t); err != nil {
			stillMissing = append(stillMissing, t)
		}
	}
	if len(stillMissing) > 0 {
		return "", fmt.Errorf("still missing after install: %v", stillMissing)
	}
	return fmt.Sprintf("installed: %s", strings.Join(missing, ", ")), nil
}

// ensureAndroidJar downloads android-34.jar from Google's SDK CDN and caches it
// inside the companion project directory. Subsequent calls return the cached path.
//
// Download path:
//  1. Parse https://dl.google.com/android/repository/repository2-3.xml
//     to find the current platform-34 zip URL.
//  2. Fall back to a set of historically-known URLs.
//  3. Download the platform zip (~70 MB), extract android.jar from it.
func ensureAndroidJar(ctx context.Context, projectDir string) (string, error) {
	jarPath := filepath.Join(projectDir, "android-34.jar")
	if info, err := os.Stat(jarPath); err == nil && info.Size() > 1024*1024 {
		return jarPath, nil // cached and non-empty
	}

	// Try manifest first, then fall back to known historical URLs.
	urls, _ := findPlatformZipURLs(ctx, 34)
	urls = append(urls,
		// Known historical URLs (try newest extension level first)
		"https://dl.google.com/android/repository/platform-34-ext10_r01.zip",
		"https://dl.google.com/android/repository/platform-34-ext7_r01.zip",
		"https://dl.google.com/android/repository/platform-34_r02.zip",
		"https://dl.google.com/android/repository/platform-34_r01.zip",
	)

	var lastErr error
	for _, zipURL := range urls {
		if err := downloadAndExtractAndroidJar(ctx, zipURL, jarPath); err == nil {
			return jarPath, nil
		} else {
			lastErr = err
		}
	}

	return "", fmt.Errorf("all download URLs failed for android-34.jar (last: %w)", lastErr)
}

// findPlatformZipURLs fetches the SDK repository manifest and returns the
// download URL(s) for the given API level platform package.
func findPlatformZipURLs(ctx context.Context, api int) ([]string, error) {
	const sdkManifest = "https://dl.google.com/android/repository/repository2-3.xml"

	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, sdkManifest, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Streaming XML parse — the manifest is large so we decode package by package.
	type complete struct {
		URL string `xml:"url"`
	}
	type archive struct {
		OS       string   `xml:"host-os"`
		Complete complete `xml:"complete"`
	}
	type archives struct {
		List []archive `xml:"archive"`
	}
	type remotePackage struct {
		Path     string   `xml:"path,attr"`
		Archives archives `xml:"archives"`
	}

	targetPath := fmt.Sprintf("platforms;android-%d", api)
	baseURL := "https://dl.google.com/android/repository/"

	var urls []string
	dec := xml.NewDecoder(resp.Body)
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "remotePackage" {
			continue
		}
		var pkg remotePackage
		if err := dec.DecodeElement(&pkg, &se); err != nil {
			continue
		}
		if pkg.Path != targetPath {
			continue
		}
		for _, arch := range pkg.Archives.List {
			if arch.Complete.URL == "" {
				continue
			}
			u := baseURL + arch.Complete.URL
			if arch.OS == "" || arch.OS == "linux" {
				urls = append([]string{u}, urls...) // prefer linux/universal
			} else {
				urls = append(urls, u)
			}
		}
		break
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("platform-android-%d not found in manifest", api)
	}
	return urls, nil
}

// downloadAndExtractAndroidJar downloads a platform zip and extracts android.jar.
func downloadAndExtractAndroidJar(ctx context.Context, zipURL, jarPath string) error {
	dlCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, zipURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, zipURL)
	}

	// Write platform zip to a temp file (needed for zip.OpenReader)
	tmpZip := jarPath + ".tmp.zip"
	defer os.Remove(tmpZip)

	f, err := os.Create(tmpZip)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("download %s: %w", zipURL, err)
	}
	f.Close()

	// Extract android.jar from the platform zip
	zr, err := zip.OpenReader(tmpZip)
	if err != nil {
		return fmt.Errorf("open zip %s: %w", zipURL, err)
	}
	defer zr.Close()

	for _, zf := range zr.File {
		// android.jar lives at "android-{api}/android.jar" inside the zip
		if !strings.HasSuffix(zf.Name, "/android.jar") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(jarPath)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if copyErr != nil {
			os.Remove(jarPath)
			return copyErr
		}
		return nil // success
	}

	return fmt.Errorf("android.jar not found inside %s", zipURL)
}

// buildAPKManual compiles and packages the companion APK without Gradle.
//
//	javac  →  d8  →  aapt2 link  →  (add classes.dex)  →  apksigner
func buildAPKManual(ctx context.Context, projectDir, androidJar string) (string, error) {
	buildDir := filepath.Join(projectDir, "build-manual")
	classesDir := filepath.Join(buildDir, "classes")
	dexDir := filepath.Join(buildDir, "dex")
	for _, d := range []string{classesDir, dexDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return "", err
		}
	}

	javaDir := filepath.Join(projectDir, "app", "src", "main", "java", "ai", "velarium", "gorkbot")
	manifest := filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml")

	// 1. Compile Java source → .class files
	compileCmd := fmt.Sprintf(
		"javac -source 8 -target 8 -bootclasspath %s -d %s %s/*.java",
		androidJar, classesDir, javaDir)
	if out, err := runShell(ctx, 3*time.Minute, compileCmd); err != nil {
		return "", fmt.Errorf("javac: %w\n%s", err, out)
	}

	// 2. Convert .class → Dalvik bytecode (classes.dex)
	dexCmd := fmt.Sprintf(
		"d8 --min-api 29 --output %s %s/ai/velarium/gorkbot/*.class",
		dexDir, classesDir)
	if out, err := runShell(ctx, 3*time.Minute, dexCmd); err != nil {
		return "", fmt.Errorf("d8: %w\n%s", err, out)
	}

	// 3. Package manifest → unsigned APK (no custom resources)
	apkUnsigned := filepath.Join(buildDir, "app-unsigned.apk")
	aapt2Cmd := fmt.Sprintf(
		"aapt2 link --manifest %s -I %s -o %s --min-sdk-version 29 --target-sdk-version 34 --version-code 2 --version-name 1.0.2",
		manifest, androidJar, apkUnsigned)
	if out, err := runShell(ctx, 2*time.Minute, aapt2Cmd); err != nil {
		return "", fmt.Errorf("aapt2 link: %w\n%s", err, out)
	}

	// 4. Add classes.dex into the APK zip
	zipCmd := fmt.Sprintf("cd %s && zip -j %s classes.dex", dexDir, apkUnsigned)
	if out, err := runShell(ctx, 30*time.Second, zipCmd); err != nil {
		return "", fmt.Errorf("adding DEX to APK: %w\n%s", err, out)
	}

	// 5. Generate debug keystore if not cached
	keystorePath := filepath.Join(buildDir, "debug.jks")
	if _, err := os.Stat(keystorePath); err != nil {
		keytoolCmd := fmt.Sprintf(
			"keytool -genkeypair -keystore %s -alias debug -keyalg RSA -keysize 2048 -validity 10000"+
				" -dname 'CN=Debug,O=Debug,C=US' -storepass gorkbotkey -keypass gorkbotkey -noprompt",
			keystorePath)
		if out, err := runShell(ctx, time.Minute, keytoolCmd); err != nil {
			return "", fmt.Errorf("keytool: %w\n%s", err, out)
		}
	}

	// 6. Sign the APK
	apkSigned := filepath.Join(buildDir, "app-signed.apk")
	signCmd := fmt.Sprintf(
		"apksigner sign --ks %s --ks-pass pass:gorkbotkey --key-pass pass:gorkbotkey --out %s %s",
		keystorePath, apkSigned, apkUnsigned)
	if out, err := runShell(ctx, time.Minute, signCmd); err != nil {
		return "", fmt.Errorf("apksigner: %w\n%s", err, out)
	}

	// 7. Copy to ~/storage/downloads for easy access
	destDir := filepath.Join(os.Getenv("HOME"), "storage", "downloads")
	if _, statErr := os.Stat(destDir); statErr == nil {
		dest := filepath.Join(destDir, "gorkbot-vision.apk")
		if data, err := os.ReadFile(apkSigned); err == nil {
			if err := os.WriteFile(dest, data, 0644); err == nil {
				return dest, nil
			}
		}
	}

	return apkSigned, nil
}

// ─── other helpers ────────────────────────────────────────────────────────────

func companionProjectDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "gorkbot-companion")
}

func generateCompanionProject() (string, error) {
	dir := companionProjectDir()
	javaDir := filepath.Join(dir, "app", "src", "main", "java", "ai", "velarium", "gorkbot")
	resDir := filepath.Join(dir, "app", "src", "main", "res")

	for _, d := range []string{javaDir, resDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return "", err
		}
	}

	files := map[string]string{
		filepath.Join(dir, "settings.gradle"):                           vision.CompanionSettingsGradle(),
		filepath.Join(dir, "build.gradle"):                              vision.CompanionRootGradle(),
		filepath.Join(dir, "app", "build.gradle"):                       vision.CompanionAppGradle(),
		filepath.Join(dir, "app", "src", "main", "AndroidManifest.xml"): vision.CompanionManifest(),
		filepath.Join(javaDir, "PermissionActivity.java"):               vision.CompanionPermissionActivity(),
		filepath.Join(javaDir, "ScreenService.java"):                    vision.CompanionScreenService(),
		filepath.Join(javaDir, "BootReceiver.java"):                     vision.CompanionBootReceiver(),
		filepath.Join(dir, ".gitignore"):                                vision.CompanionGitignore(),
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("write %s: %w", filepath.Base(path), err)
		}
	}

	return dir, nil
}

func installAPK(ctx context.Context, apkPath string) (string, error) {
	installCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Prefer termux-open (handles content URI correctly)
	if termuxOpen, err := exec.LookPath("termux-open"); err == nil {
		cmd := exec.CommandContext(installCtx, termuxOpen, apkPath)
		if out, err := cmd.CombinedOutput(); err == nil {
			_ = out
			return "Android install dialog opened via termux-open", nil
		}
	}

	// Fallback: am start with VIEW intent
	amCmd := fmt.Sprintf(
		"am start -a android.intent.action.VIEW -d 'file://%s' -t application/vnd.android.package-archive",
		apkPath)
	out, err := runShell(ctx, 10*time.Second, amCmd)
	if err != nil {
		return "", fmt.Errorf("am start: %w — %s", err, out)
	}
	return "Android install dialog opened — tap Install to complete", nil
}

func isCompanionInstalled(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := runShell(ctx, 5*time.Second, "pm list packages ai.velarium.gorkbot")
	return err == nil && strings.Contains(out, "ai.velarium.gorkbot")
}

func runShell(ctx context.Context, timeout time.Duration, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func boolIcon(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func errorResult(msg string) *ToolResult {
	return &ToolResult{Success: false, Error: msg, Output: "## Error\n\n" + msg}
}
