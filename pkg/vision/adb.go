package vision

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ConnectResult describes the outcome of an auto-connect attempt.
type ConnectResult struct {
	Connected bool
	Address   string // "ip:port" if connected
	DeviceIP  string // discovered local IP
	Ports     []int  // discovered ADB listening ports
	Message   string
}

// AutoConnect tries to find and connect to the Android device over wireless ADB
// without any user input. It:
//  1. Checks if already connected → returns immediately
//  2. Reads the device's own WiFi IP from network interfaces (pure Go)
//  3. Discovers ADB listening ports from /proc/net/tcp and /proc/net/tcp6
//  4. Tries adb connect for each IP:port combination
//  5. Returns a ConnectResult with precise instructions if nothing worked
func AutoConnect(ctx context.Context) (*ConnectResult, error) {
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// 1. Already connected?
	if isADBReady(connectCtx) {
		addr := currentADBDevice(connectCtx)
		return &ConnectResult{Connected: true, Address: addr, Message: "already connected"}, nil
	}

	// 2. Get device WiFi IPs
	ips := localIPv4s()
	deviceIP := ""
	if len(ips) > 0 {
		deviceIP = ips[0]
	}

	// 3. Discover ADB listening ports from /proc
	ports := discoverADBPorts()

	if len(ports) == 0 {
		msg := "Wireless Debugging is not enabled — no ADB ports found.\n\n"
		msg += "Enable it:\n"
		msg += "  Settings → Developer Options → Wireless Debugging → toggle ON\n\n"
		if deviceIP != "" {
			msg += fmt.Sprintf("Your device IP: %s\n", deviceIP)
			msg += "Then run: adb pair " + deviceIP + ":<PAIRING-PORT>\n"
			msg += "  (pairing port shown in the Wireless Debugging screen)\n"
			msg += "Then run: adb connect " + deviceIP + ":<DEBUG-PORT>\n"
			msg += "  (debug port also shown in Wireless Debugging screen)"
		}
		return &ConnectResult{
			Connected: false,
			DeviceIP:  deviceIP,
			Ports:     nil,
			Message:   msg,
		}, nil
	}

	// 4. Try adb connect for every IP:port pair
	for _, ip := range ips {
		for _, port := range ports {
			addr := fmt.Sprintf("%s:%d", ip, port)
			if tryADBConnect(connectCtx, addr) {
				return &ConnectResult{
					Connected: true,
					Address:   addr,
					DeviceIP:  deviceIP,
					Ports:     ports,
					Message:   "auto-connected to " + addr,
				}, nil
			}
		}
	}

	// 5. Ports found but connect failed — build precise instructions
	portList := make([]string, len(ports))
	for i, p := range ports {
		portList[i] = strconv.Itoa(p)
	}

	msg := fmt.Sprintf("Found ADB port(s) %s but connection was refused.\n\n",
		strings.Join(portList, ", "))

	if deviceIP != "" {
		msg += "This usually means you need to pair first (one-time only):\n\n"
		msg += fmt.Sprintf("  1. On phone: Settings → Developer Options → Wireless Debugging\n")
		msg += fmt.Sprintf("     → tap \"Pair device with pairing code\"\n")
		msg += fmt.Sprintf("  2. Run: adb pair %s:<PAIRING-PORT>\n", deviceIP)
		msg += fmt.Sprintf("     (use the pairing port shown on screen, NOT %s)\n",
			strings.Join(portList, "/"))
		msg += fmt.Sprintf("  3. Run: adb connect %s:%s\n", deviceIP, portList[0])
		msg += "\nAfter pairing once, reconnection is automatic."
	}

	return &ConnectResult{
		Connected: false,
		DeviceIP:  deviceIP,
		Ports:     ports,
		Message:   msg,
	}, nil
}

// isADBReady returns true if at least one authorized ADB device is attached.
func isADBReady(ctx context.Context) bool {
	out, err := exec.CommandContext(ctx, adbBin, "devices").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n")[1:] {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, "\tdevice") || strings.HasSuffix(line, " device") {
			return true
		}
	}
	return false
}

// currentADBDevice returns the address of the first connected ADB device,
// or an empty string if none.
func currentADBDevice(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, adbBin, "devices").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n")[1:] {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, "\tdevice") || strings.HasSuffix(line, " device") {
			return strings.Fields(line)[0]
		}
	}
	return ""
}

// tryADBConnect runs "adb connect addr" and returns true if the output
// contains "connected" (and not "failed" or "refused").
func tryADBConnect(ctx context.Context, addr string) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, adbBin, "connect", addr).Output()
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(out))
	return strings.Contains(lower, "connected") &&
		!strings.Contains(lower, "failed") &&
		!strings.Contains(lower, "refused") &&
		!strings.Contains(lower, "cannot")
}

// localIPv4s returns non-loopback IPv4 addresses for all active network
// interfaces, preferring wlan0 (WiFi) over mobile data interfaces.
func localIPv4s() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var wlan, other []string

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			ipStr := ip.String()
			if strings.HasPrefix(iface.Name, "wlan") {
				wlan = append(wlan, ipStr)
			} else {
				other = append(other, ipStr)
			}
		}
	}

	return append(wlan, other...)
}

// discoverADBPorts reads /proc/net/tcp and /proc/net/tcp6 to find TCP ports
// in LISTEN state that fall in the ADB wireless debugging range (>= 30000).
// This works without root on Android/Termux.
func discoverADBPorts() []int {
	seen := make(map[int]bool)
	var ports []int

	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		scanner.Scan() // skip header line
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 4 {
				continue
			}
			// fields[3] is state: "0A" = TCP_LISTEN
			if strings.ToUpper(fields[3]) != "0A" {
				continue
			}
			// fields[1] is local_address: "XXXXXXXX:PPPP" or ipv6 variant
			parts := strings.Split(fields[1], ":")
			if len(parts) < 2 {
				continue
			}
			portHex := parts[len(parts)-1]
			port64, err := strconv.ParseInt(portHex, 16, 32)
			if err != nil {
				continue
			}
			port := int(port64)
			if port >= 30000 && !seen[port] {
				seen[port] = true
				ports = append(ports, port)
			}
		}
		f.Close()
	}

	return ports
}

// SetupStatus returns a structured summary of the current ADB state
// including device IP and any discovered ports, for display to the user.
func SetupStatus(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var sb strings.Builder

	// ADB devices
	out, err := exec.CommandContext(ctx, adbBin, "devices", "-l").Output()
	if err == nil {
		sb.WriteString("ADB devices:\n")
		sb.WriteString(strings.TrimSpace(string(out)))
		sb.WriteString("\n\n")
	}

	// Local IP
	ips := localIPv4s()
	if len(ips) > 0 {
		sb.WriteString("Device IP: " + strings.Join(ips, ", ") + "\n")
	} else {
		sb.WriteString("Device IP: (not found — is WiFi connected?)\n")
	}

	// Listening ports
	ports := discoverADBPorts()
	if len(ports) > 0 {
		portStrs := make([]string, len(ports))
		for i, p := range ports {
			portStrs[i] = strconv.Itoa(p)
		}
		sb.WriteString("ADB listening port(s): " + strings.Join(portStrs, ", ") + "\n")
	} else {
		sb.WriteString("ADB listening ports: none (Wireless Debugging not enabled)\n")
	}

	return strings.TrimSpace(sb.String())
}
